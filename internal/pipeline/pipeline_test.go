package pipeline_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/detect"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
)

// fakeEngine stands in for a real converter so the pipeline's own behaviour
// can be tested without depending on any external tool.
type fakeEngine struct {
	id        string
	available bool
	outputs   int
	// behaviour hooks, all optional
	fail       error
	emptyFile  bool
	noOutput   bool
	writeInput bool
	block      time.Duration
	stages     []string
}

func (f *fakeEngine) ID() string {
	if f.id == "" {
		return "fake"
	}
	return f.id
}

func (f *fakeEngine) Available() bool { return f.available }

func (f *fakeEngine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	for i, s := range f.stages {
		progress(engine.Progress{Fraction: float64(i+1) / float64(len(f.stages)), Stage: s})
	}

	if f.writeInput {
		// The thing the integrity test exists to catch.
		if err := os.WriteFile(req.Inputs[0], []byte("clobbered"), 0o644); err != nil {
			return nil, err
		}
	}
	if f.block > 0 {
		select {
		case <-time.After(f.block):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail != nil {
		return nil, f.fail
	}
	if f.noOutput {
		return &engine.Response{}, nil
	}

	n := f.outputs
	if n == 0 {
		n = 1
	}
	var outs []string
	for i := 0; i < n; i++ {
		name := "result.pdf"
		if i > 0 {
			name = "result-" + string(rune('a'+i)) + ".pdf"
		}
		p := req.Workspace.Path(name)
		body := []byte("%PDF-1.7\nfake output\n%%EOF\n")
		if f.emptyFile {
			body = nil
		}
		if err := os.WriteFile(p, body, 0o644); err != nil {
			return nil, err
		}
		outs = append(outs, p)
	}
	return &engine.Response{Outputs: outs, Notes: []string{"produced by a test engine"}}, nil
}

func fakeTask() task.Task {
	return task.Task{
		ID: "test.convert", Name: "Test convert", Verb: "Convert", Engine: "fake",
		Accepts: []detect.Category{detect.CategoryPDF}, MinInputs: 1, MaxInputs: 2,
	}
}

func newRunner(t *testing.T, e *fakeEngine) *pipeline.Runner {
	t.Helper()
	return pipeline.New(engine.NewRegistry(e), nil)
}

func TestRunProducesAnOutputInTheChosenFolder(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	outDir := t.TempDir()

	res, err := newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: outDir,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(res.Outputs))
	}
	if filepath.Dir(res.Outputs[0]) != outDir {
		t.Errorf("output landed in %s, want %s", filepath.Dir(res.Outputs[0]), outDir)
	}
	if res.InputBytes == 0 || res.OutputBytes == 0 {
		t.Error("size figures are needed for the result card")
	}
}

// The most important test in the project: an input file is never modified.
func TestInputFilesAreNeverModified(t *testing.T) {
	dir := t.TempDir()
	in := writePDF(t, dir, "sacred.pdf")
	before := hashOf(t, in)

	for _, tc := range []struct {
		name string
		eng  *fakeEngine
	}{
		{"success", &fakeEngine{available: true}},
		{"engine failure", &fakeEngine{available: true, fail: errors.New("engine exploded")}},
		{"empty output", &fakeEngine{available: true, emptyFile: true}},
		{"no output", &fakeEngine{available: true, noOutput: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = newRunner(t, tc.eng).Run(context.Background(), pipeline.Request{
				Task: fakeTask(), Inputs: []string{in}, OutputDir: t.TempDir(),
			})
			if after := hashOf(t, in); after != before {
				t.Fatal("the input file was modified")
			}
		})
	}
}

func TestVerifyRejectsAnEmptyOutputRatherThanPublishingIt(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	outDir := t.TempDir()

	_, err := newRunner(t, &fakeEngine{available: true, emptyFile: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: outDir,
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeOutputInvalid {
		t.Fatalf("got %v, want a CodeOutputInvalid user error", err)
	}
	assertEmptyDir(t, outDir)
}

func TestAnEngineProducingNothingIsAnHonestError(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	_, err := newRunner(t, &fakeEngine{available: true, noOutput: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: t.TempDir(),
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeOutputInvalid {
		t.Fatalf("got %v, want a CodeOutputInvalid user error", err)
	}
}

func TestValidationRejectsTheWrongKindOfFile(t *testing.T) {
	dir := t.TempDir()
	notPDF := filepath.Join(dir, "photo.pdf")
	if err := os.WriteFile(notPDF, []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{notPDF}, OutputDir: t.TempDir(),
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeUnsupportedInput {
		t.Fatalf("got %v, want CodeUnsupportedInput", err)
	}
	// The message should name the real type, not just refuse.
	if !strings.Contains(ue.Message, "actually") {
		t.Errorf("message should explain the mismatch, got %q", ue.Message)
	}
}

func TestValidationRejectsAnEmptyFileWithItsOwnMessage(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{empty}, OutputDir: t.TempDir(),
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeEmptyInput {
		t.Fatalf("got %v, want CodeEmptyInput", err)
	}
}

func TestValidationAsksForAPasswordBeforeRunningAnything(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked.pdf")
	body := "%PDF-1.7\n1 0 obj\n<< >>\nendobj\ntrailer\n<< /Encrypt 5 0 R >>\n%%EOF\n"
	if err := os.WriteFile(locked, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{locked}, OutputDir: t.TempDir(),
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodePasswordRequired {
		t.Fatalf("got %v, want CodePasswordRequired", err)
	}
	if len(ue.Actions) == 0 || ue.Actions[0] != usererr.ActionEnterPassword {
		t.Error("a password error must offer a password field as its next action")
	}
}

func TestValidationEnforcesInputCounts(t *testing.T) {
	dir := t.TempDir()
	a, b, c := writePDF(t, dir, "a.pdf"), writePDF(t, dir, "b.pdf"), writePDF(t, dir, "c.pdf")

	r := newRunner(t, &fakeEngine{available: true})
	_, err := r.Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{a, b, c}, OutputDir: t.TempDir(),
	})
	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeInvalidOptions {
		t.Fatalf("three inputs into a two-input task: got %v", err)
	}

	_, err = r.Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: nil, OutputDir: t.TempDir(),
	})
	if !errors.As(err, &ue) || ue.Code != usererr.CodeInvalidOptions {
		t.Fatalf("no inputs: got %v", err)
	}
}

func TestAnUnavailableEngineAsksForADownload(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	_, err := newRunner(t, &fakeEngine{available: false}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: t.TempDir(),
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeComponentMissing {
		t.Fatalf("got %v, want CodeComponentMissing", err)
	}
}

func TestCancellationLeavesNoPartialFiles(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	outDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := newRunner(t, &fakeEngine{available: true, block: 5 * time.Second}).Run(ctx, pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: outDir,
	})

	var ue *usererr.Error
	if !errors.As(err, &ue) || ue.Code != usererr.CodeCancelled {
		t.Fatalf("got %v, want CodeCancelled", err)
	}
	assertEmptyDir(t, outDir)
}

func TestOutputsNeverOverwriteAnExistingFile(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	outDir := t.TempDir()
	existing := filepath.Join(outDir, "result.pdf")
	if err := os.WriteFile(existing, []byte("the user's earlier work"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
		Task: fakeTask(), Inputs: []string{in}, OutputDir: outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(res.Outputs[0]) != "result (1).pdf" {
		t.Errorf("output named %q, want result (1).pdf", filepath.Base(res.Outputs[0]))
	}

	kept, err := os.ReadFile(existing)
	if err != nil || string(kept) != "the user's earlier work" {
		t.Errorf("the existing file was disturbed: %q, %v", kept, err)
	}
}

func TestTheWorkspaceIsRemovedAfterEveryRun(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	before := countTempWorkspaces(t)

	for i := 0; i < 3; i++ {
		_, _ = newRunner(t, &fakeEngine{available: true}).Run(context.Background(), pipeline.Request{
			Task: fakeTask(), Inputs: []string{in}, OutputDir: t.TempDir(),
		})
	}
	if after := countTempWorkspaces(t); after > before {
		t.Errorf("workspaces leaked: %d before, %d after", before, after)
	}
}

func TestEngineProgressReachesTheCaller(t *testing.T) {
	in := writePDF(t, t.TempDir(), "input.pdf")
	var stages []string

	_, err := newRunner(t, &fakeEngine{available: true, stages: []string{"page 1 of 2", "page 2 of 2"}}).
		Run(context.Background(), pipeline.Request{
			Task: fakeTask(), Inputs: []string{in}, OutputDir: t.TempDir(),
			Progress: func(p engine.Progress) { stages = append(stages, p.Stage) },
		})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"page 1 of 2", "page 2 of 2"} {
		found := false
		for _, s := range stages {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("stage %q never reached the caller; got %q", want, stages)
		}
	}
}

func writePDF(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := "%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< >>\n%%EOF\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func hashOf(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("output folder should be empty, contains %v", names)
	}
}

func countTempWorkspaces(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(os.TempDir(), "lathe"))
	if err != nil {
		return 0
	}
	return len(entries)
}
