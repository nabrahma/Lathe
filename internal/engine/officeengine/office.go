// Package officeengine converts documents by driving LibreOffice headless.
//
// LibreOffice is the only realistic way to convert Office formats faithfully,
// and it is also the most awkward tool Lathe drives: it keeps a per-user
// profile lock, so two conversions sharing a profile deadlock, and it exits
// zero whether or not it produced anything.
package officeengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	lexec "github.com/nabrahma/lathe/internal/exec"
	"github.com/nabrahma/lathe/internal/usererr"
)

const (
	componentID    = "libreoffice"
	convertTimeout = 10 * time.Minute
)

// Engine performs document conversions.
type Engine struct {
	deps   deps.Manager
	runner lexec.Runner
}

// New returns the document engine backed by the given component manager.
func New(m deps.Manager) *Engine {
	return &Engine{deps: m, runner: lexec.New()}
}

// ID identifies the engine in the task registry.
func (e *Engine) ID() string { return "libreoffice" }

// Available reports whether LibreOffice is installed and runs.
func (e *Engine) Available() bool {
	return e.deps != nil && e.deps.Available(componentID)
}

// Execute converts each input to the requested format.
func (e *Engine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	bin, err := e.deps.BinaryPath(componentID, "soffice")
	if err != nil {
		hint := ""
		for _, c := range e.deps.Components() {
			if c.ID == componentID {
				hint = c.Hint()
			}
		}
		return nil, usererr.New(usererr.CodeComponentMissing,
			strings.TrimSpace("Converting Office documents needs LibreOffice. "+hint),
			usererr.ActionCopyDetails)
	}

	format, err := targetFormat(req)
	if err != nil {
		return nil, err
	}

	outDir, err := req.Workspace.Sub("converted")
	if err != nil {
		return nil, err
	}

	// A private user profile per job. Sharing the default profile means a
	// second conversion blocks on a lock held by the first, and a crashed run
	// leaves a stale lock that hangs every later job.
	profile, err := req.Workspace.Sub("lo-profile")
	if err != nil {
		return nil, err
	}

	var outputs []string
	for i, in := range req.Inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress(engine.Progress{
			Fraction: float64(i) / float64(len(req.Inputs)),
			Stage:    fmt.Sprintf("Converting %s", filepath.Base(in)),
		})

		args := []string{
			"--headless", "--norestore", "--invisible",
			"--nolockcheck", "--nodefault", "--nofirststartwizard",
			"-env:UserInstallation=" + fileURL(profile),
		}
		// A PDF has to be opened through the Draw import filter; without it
		// LibreOffice refuses the document rather than converting it.
		if inFilter := importFilter(in); inFilter != "" {
			args = append(args, "--infilter="+inFilter)
		}
		args = append(args, "--convert-to", format, "--outdir", outDir, in)

		res, err := e.runner.Run(ctx, bin, args, lexec.Options{Timeout: convertTimeout})
		if err != nil {
			return nil, translate(err, in)
		}

		// LibreOffice exits zero even when it converted nothing, so success is
		// judged by whether a file appeared, not by the exit code.
		produced, err := findOutput(outDir, in, format, outputs)
		if err != nil {
			detail := strings.TrimSpace(string(res.Stderr) + string(res.Stdout))
			return nil, usererr.Wrap(fmt.Errorf("%s", detail), usererr.CodeCorruptInput,
				fmt.Sprintf("%s couldn't be converted. It may be damaged, or use a feature LibreOffice doesn't recognise.",
					filepath.Base(in)),
				usererr.ActionChooseFile, usererr.ActionCopyDetails)
		}
		outputs = append(outputs, produced)
	}

	progress(engine.Progress{Fraction: 1, Stage: "Finishing"})
	return &engine.Response{Outputs: outputs, Notes: notesFor(req, format)}, nil
}

// exportFilters names the LibreOffice filter for each format Lathe offers.
// The bare extension is ambiguous for several of these, and LibreOffice then
// reports "no export filter found" rather than guessing.
var exportFilters = map[string]string{
	"docx": "docx:MS Word 2007 XML",
	"doc":  "doc:MS Word 97",
	"odt":  "odt:writer8",
	"rtf":  "rtf:Rich Text Format",
	"txt":  "txt:Text (encoded):UTF8",
	"xlsx": "xlsx:Calc MS Excel 2007 XML",
	"ods":  "ods:calc8",
	"csv":  "csv:Text - txt - csv (StarCalc)",
	"pptx": "pptx:Impress MS PowerPoint 2007 XML",
	"odp":  "odp:impress8",
	"pdf":  "pdf",
}

// targetFormat resolves the LibreOffice filter for the task.
func targetFormat(req engine.Request) (string, error) {
	if custom := strings.TrimSpace(req.Options.String("target", "")); custom != "" {
		// The Advanced escape hatch: any filter LibreOffice knows. Arguments
		// reach the process as a slice, so nothing here needs shell escaping;
		// the only real constraint is that it is a single plausible token.
		if len(custom) > 120 || strings.ContainsAny(custom, "\n\r\x00") {
			return "", usererr.New(usererr.CodeInvalidOptions,
				"That format name isn't valid.", usererr.ActionChangeOption)
		}
		if filter, known := exportFilters[strings.ToLower(custom)]; known {
			return filter, nil
		}
		return custom, nil
	}

	format := "pdf"
	if req.Task.ID != "document.to-pdf" {
		format = req.Options.String("format", "pdf")
	}
	if filter, known := exportFilters[strings.ToLower(format)]; known {
		return filter, nil
	}
	return format, nil
}

// importFilter is needed only for inputs LibreOffice will not open by
// sniffing alone.
func importFilter(in string) string {
	if strings.EqualFold(filepath.Ext(in), ".pdf") {
		return "writer_pdf_import"
	}
	return ""
}

// findOutput locates the file LibreOffice wrote. It derives the expected name
// rather than taking the newest file, so a batch cannot claim a sibling's
// output.
func findOutput(dir, in, format string, alreadyClaimed []string) (string, error) {
	ext := format
	if i := strings.Index(ext, ":"); i > 0 {
		// Filters can be written as "csv:Text - txt - csv (StarCalc)".
		ext = ext[:i]
	}

	base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(filepath.Base(in)))
	candidate := filepath.Join(dir, base+"."+ext)
	if info, err := os.Stat(candidate); err == nil && info.Size() > 0 && !claimed(candidate, alreadyClaimed) {
		return candidate, nil
	}

	// Fall back to any unclaimed file with the right extension: LibreOffice
	// occasionally sanitises the stem.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(strings.TrimPrefix(filepath.Ext(entry.Name()), "."), ext) {
			continue
		}
		p := filepath.Join(dir, entry.Name())
		if info, err := entry.Info(); err == nil && info.Size() > 0 && !claimed(p, alreadyClaimed) {
			return p, nil
		}
	}
	return "", fmt.Errorf("no %s output was produced", ext)
}

func claimed(path string, claims []string) bool {
	for _, c := range claims {
		if c == path {
			return true
		}
	}
	return false
}

// notesFor states a fidelity caveat before the user discovers it themselves.
func notesFor(req engine.Request, format string) []string {
	if req.Task.ID == "document.pdf-to-word" {
		return []string{
			"Converting a PDF back into an editable document is best-effort. " +
				"Simple pages come through well; complex layouts with columns and floating figures often do not.",
		}
	}
	if format == "txt" {
		return []string{"Saving as plain text keeps the words but drops all formatting, images and tables."}
	}
	return nil
}

func translate(err error, in string) error {
	var exit *lexec.ExitError
	if ok := asExit(err, &exit); ok {
		lower := strings.ToLower(exit.Stderr)
		if strings.Contains(lower, "source file could not be loaded") {
			return usererr.Wrap(err, usererr.CodeCorruptInput,
				fmt.Sprintf("%s couldn't be opened. It may be damaged, or made by software LibreOffice doesn't recognise.",
					filepath.Base(in)),
				usererr.ActionChooseFile, usererr.ActionCopyDetails)
		}
	}
	return err
}

// fileURL builds the file:// URL LibreOffice requires for its profile path.
// A Windows path needs forward slashes and a leading slash before the drive.
func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return "file://" + slashed
}

var _ engine.Engine = (*Engine)(nil)
