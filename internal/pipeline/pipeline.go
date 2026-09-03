// Package pipeline runs one job from validated input to published output.
//
// The stages are fixed and always run in order: validate, ensure components,
// plan, execute, verify, write. Verification is not optional: a truncated
// engine output has a plausible size and is broken, and catching that here is
// the difference between an honest error and a corrupt file in someone's
// Documents folder.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nabrahma/lathe/internal/detect"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
)

// Stage names the phase a job is in, for the progress label.
type Stage string

// The pipeline stages, in order.
const (
	StageValidate Stage = "validating"
	StagePrepare  Stage = "preparing"
	StageExecute  Stage = "converting"
	StageVerify   Stage = "checking the result"
	StageWrite    Stage = "saving"
)

// TierChecker reports whether the components a task needs are installed. The
// pipeline only asks; downloading is the caller's decision, because it needs
// the user's consent.
type TierChecker interface {
	TierAvailable(t task.Tier) bool
	TierName(t task.Tier) string
	TierDownloadMB(t task.Tier) int
}

// Request is one job to run.
type Request struct {
	Task      task.Task
	Inputs    []string
	Options   engine.Options
	OutputDir string
	// Progress is called as the job advances. It may be nil.
	Progress func(engine.Progress)
}

// Result is what a completed job produced.
type Result struct {
	// Outputs are absolute paths in the user's chosen folder.
	Outputs []string
	Notes   []string
	// InputBytes and OutputBytes drive the "14.2 MB to 1.8 MB" line on the
	// result card.
	InputBytes  int64
	OutputBytes int64
}

// Runner executes jobs.
type Runner struct {
	engines *engine.Registry
	tiers   TierChecker
}

// New returns a Runner. tiers may be nil, in which case every tier is treated
// as available, which is what the tests and the headless CLI want.
func New(engines *engine.Registry, tiers TierChecker) *Runner {
	return &Runner{engines: engines, tiers: tiers}
}

// Run executes a job end to end. Every error it returns is already translated
// for display.
func (r *Runner) Run(ctx context.Context, req Request) (*Result, error) {
	report := req.Progress
	if report == nil {
		report = func(engine.Progress) {}
	}

	report(engine.Indeterminate(string(StageValidate)))
	inputs, err := r.validate(req)
	if err != nil {
		return nil, err
	}

	report(engine.Indeterminate(string(StagePrepare)))
	eng, err := r.prepare(req)
	if err != nil {
		return nil, err
	}

	ws, err := fsatomic.TempWorkspace(strings.ReplaceAll(req.Task.ID, ".", "-"))
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeNotWritable,
			"Lathe couldn't create a temporary working folder. Check that your disk isn't full.",
			usererr.ActionFreeSpace, usererr.ActionRetry)
	}
	defer func() { _ = ws.Close() }()

	report(engine.Indeterminate(string(StageExecute)))
	resp, err := eng.Execute(ctx, engine.Request{
		Task:      req.Task,
		Inputs:    req.Inputs,
		Options:   req.Options,
		Workspace: ws,
	}, report)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, usererr.New(usererr.CodeCancelled, "The job was cancelled.")
		}
		return nil, usererr.Translate(err)
	}
	if resp == nil || len(resp.Outputs) == 0 {
		return nil, usererr.New(usererr.CodeOutputInvalid,
			"The conversion finished but produced no file. Your original is untouched.",
			usererr.ActionRetry, usererr.ActionCopyDetails)
	}

	report(engine.Indeterminate(string(StageVerify)))
	if err := verify(resp.Outputs); err != nil {
		return nil, err
	}

	report(engine.Indeterminate(string(StageWrite)))
	result, err := publish(resp, req.OutputDir)
	if err != nil {
		return nil, err
	}
	for _, in := range inputs {
		result.InputBytes += in.SizeBytes
	}
	return result, nil
}

// validate checks everything that can be checked before any work is done, so a
// predictable failure arrives immediately rather than after five minutes.
func (r *Runner) validate(req Request) ([]detect.FileType, error) {
	t := req.Task

	if len(req.Inputs) < t.MinInputs {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			fmt.Sprintf("%s needs at least %s.", t.Name, plural(t.MinInputs, "file")),
			usererr.ActionChooseFile)
	}
	if t.MaxInputs > 0 && len(req.Inputs) > t.MaxInputs {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			fmt.Sprintf("%s takes at most %s at a time.", t.Name, plural(t.MaxInputs, "file")),
			usererr.ActionChooseFile)
	}

	types := make([]detect.FileType, 0, len(req.Inputs))
	for _, in := range req.Inputs {
		name := filepath.Base(in)

		ft, err := detect.Detect(in)
		switch {
		case errors.Is(err, detect.ErrEmptyFile):
			return nil, usererr.New(usererr.CodeEmptyInput,
				fmt.Sprintf("%s is empty, so there is nothing to convert.", name),
				usererr.ActionChooseFile).WithFile(name)
		case err != nil:
			return nil, usererr.Wrap(err, usererr.CodeUnsupportedInput,
				fmt.Sprintf("%s couldn't be read.", name),
				usererr.ActionChooseFile).WithFile(name)
		}

		if !t.AcceptsCategory(ft.Category) {
			msg := fmt.Sprintf("%s can't be used with %s.", name, t.Name)
			if ft.MismatchesName(name) {
				msg = fmt.Sprintf("%s is actually a %s file despite its name, so it can't be used with %s.",
					name, strings.ToUpper(ft.Extension), t.Name)
			}
			return nil, usererr.New(usererr.CodeUnsupportedInput, msg, usererr.ActionChooseFile).WithFile(name)
		}

		if ft.Encrypted && !req.Options.Has("password") {
			return nil, usererr.New(usererr.CodePasswordRequired,
				fmt.Sprintf("%s is password-protected. Enter the password to continue.", name),
				usererr.ActionEnterPassword).WithFile(name)
		}
		types = append(types, ft)
	}

	if err := fsatomic.CheckWritable(req.OutputDir); err != nil {
		return nil, usererr.Wrap(err, usererr.CodeNotWritable,
			"Lathe can't save to that folder. Choose a different one.",
			usererr.ActionChangeOption)
	}
	return types, nil
}

// prepare resolves the engine and confirms its components are installed.
func (r *Runner) prepare(req Request) (engine.Engine, error) {
	if r.tiers != nil && !r.tiers.TierAvailable(req.Task.RequiredTier) {
		name := r.tiers.TierName(req.Task.RequiredTier)
		size := r.tiers.TierDownloadMB(req.Task.RequiredTier)
		return nil, usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s needs %s, a one-time %d MB download. It works offline afterwards.",
				req.Task.Name, name, size),
			usererr.ActionDownload)
	}

	eng, err := r.engines.Get(req.Task.Engine)
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeComponentMissing,
			fmt.Sprintf("%s isn't available in this build of Lathe.", req.Task.Name),
			usererr.ActionCopyDetails)
	}
	if !eng.Available() {
		return nil, usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s needs a component that isn't installed yet.", req.Task.Name),
			usererr.ActionDownload)
	}
	return eng, nil
}

// verify confirms each output is a real, non-trivial file of a recognised
// type. A truncated result is the failure mode this exists to catch.
func verify(outputs []string) error {
	for _, out := range outputs {
		info, err := os.Stat(out)
		if err != nil {
			return usererr.Wrap(err, usererr.CodeOutputInvalid,
				"The conversion finished but its result is missing. Your original is untouched.",
				usererr.ActionRetry, usererr.ActionCopyDetails)
		}
		if info.Size() == 0 {
			return usererr.New(usererr.CodeOutputInvalid,
				"The conversion produced an empty file. Your original is untouched.",
				usererr.ActionRetry, usererr.ActionCopyDetails)
		}
		if _, err := detect.Detect(out); err != nil {
			return usererr.Wrap(err, usererr.CodeOutputInvalid,
				"The result didn't come out as a valid file. Your original is untouched.",
				usererr.ActionRetry, usererr.ActionCopyDetails)
		}
	}
	return nil
}

// publish moves verified results into the user's folder, never overwriting.
func publish(resp *engine.Response, outputDir string) (*Result, error) {
	result := &Result{Notes: resp.Notes, Outputs: make([]string, 0, len(resp.Outputs))}

	for _, src := range resp.Outputs {
		base := filepath.Base(src)
		ext := filepath.Ext(base)
		dst := fsatomic.UniquePath(outputDir, strings.TrimSuffix(base, ext), ext)

		if err := fsatomic.Publish(src, dst, 0); err != nil {
			// Clean up anything already published so a partial batch does not
			// litter the user's folder.
			for _, done := range result.Outputs {
				_ = os.Remove(done)
			}
			return nil, usererr.Translate(err)
		}
		result.Outputs = append(result.Outputs, dst)
		if info, err := os.Stat(dst); err == nil {
			result.OutputBytes += info.Size()
		}
	}
	return result, nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
