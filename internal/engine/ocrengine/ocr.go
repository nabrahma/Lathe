// Package ocrengine reads text out of images and PDFs.
//
// OCR is what separates Lathe from the self-hosted converters, so it gets a
// real preprocessing chain (see preprocess.go) rather than a naive Tesseract
// call on whatever the user dropped in.
package ocrengine

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	lexec "github.com/nabrahma/lathe/internal/exec"
	"github.com/nabrahma/lathe/internal/usererr"
)

const (
	componentID     = "tesseract"
	perPageTimeout  = 5 * time.Minute
	minUsefulLength = 8
)

// Engine performs OCR and text extraction.
type Engine struct {
	deps   deps.Manager
	runner lexec.Runner
}

// New returns the OCR engine backed by the given component manager.
func New(m deps.Manager) *Engine {
	return &Engine{deps: m, runner: lexec.New()}
}

// ID identifies the engine in the task registry.
func (e *Engine) ID() string { return "tesseract" }

// Available is true when text can be extracted at all. Native PDF text
// extraction is pure Go and always works, so the engine is never wholly
// unavailable; individual tasks check for Tesseract themselves.
func (e *Engine) Available() bool { return true }

// Execute dispatches to the handler for the requested task.
func (e *Engine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	switch req.Task.ID {
	case "text.from-image":
		return e.fromImages(ctx, req, progress)
	case "text.from-pdf":
		return e.fromPDF(ctx, req, progress)
	case "text.searchable-pdf":
		return e.searchablePDF(ctx, req, progress)
	case "text.image-to-document":
		return e.fromImages(ctx, req, progress)
	default:
		return nil, fmt.Errorf("OCR engine cannot handle task %q", req.Task.ID)
	}
}

// tesseractPath resolves the binary, with a message that says what to install
// rather than reporting a missing file.
func (e *Engine) tesseractPath() (string, error) {
	bin, err := e.deps.BinaryPath(componentID, "tesseract")
	if err != nil {
		hint := ""
		for _, c := range e.deps.Components() {
			if c.ID == componentID {
				hint = c.Hint()
			}
		}
		return "", usererr.New(usererr.CodeComponentMissing,
			strings.TrimSpace("Reading text from images needs Tesseract. "+hint),
			usererr.ActionCopyDetails)
	}
	return bin, nil
}

func (e *Engine) fromImages(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	bin, err := e.tesseractPath()
	if err != nil {
		return nil, err
	}

	var (
		outputs []string
		notes   []string
	)

	for i, in := range req.Inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress(engine.Progress{
			Fraction: float64(i) / float64(len(req.Inputs)),
			Stage:    fmt.Sprintf("Reading %s", filepath.Base(in)),
		})

		source := in
		if req.Options.Bool("enhance", true) {
			prepared, prepErr := e.prepare(in, req)
			if prepErr != nil {
				return nil, prepErr
			}
			source = prepared
		}

		text, err := e.readText(ctx, bin, source, req.Options)
		if err != nil {
			return nil, err
		}
		if note := qualityNote(in, text); note != "" && !contains(notes, note) {
			notes = append(notes, note)
		}

		base := strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
		// The OCR stage always writes plain text; converting that to .docx or
		// .pdf is a later step in the plan, done by the document engine.
		out := req.Workspace.UniqueName(base + ".txt")
		if err := os.WriteFile(out, []byte(text), 0o644); err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
	}

	progress(engine.Progress{Fraction: 1, Stage: "Finishing"})
	return &engine.Response{Outputs: outputs, Notes: notes}, nil
}

// fromPDF prefers the PDF's own text layer and falls back to OCR only for
// pages that have none, which is both faster and more accurate.
func (e *Engine) fromPDF(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	in := req.Inputs[0]
	progress(engine.Indeterminate("Looking for text in the PDF"))

	textDir, err := req.Workspace.Sub("text")
	if err != nil {
		return nil, err
	}

	conf := pdfConf(req.Options.String("password", ""))
	native := ""
	if err := api.ExtractContentFile(in, textDir, nil, conf); err == nil {
		native = readAll(textDir)
	}

	if strings.TrimSpace(native) != "" || !req.Options.Bool("ocr", true) {
		out := req.Workspace.UniqueName(nameFor(in, ".txt"))
		if err := os.WriteFile(out, []byte(native), 0o644); err != nil {
			return nil, err
		}
		if strings.TrimSpace(native) == "" {
			return nil, usererr.New(usererr.CodeNoTextFound,
				"This PDF has no text layer, so it is probably a scan. Turn on \"Read scanned pages too\" to have Lathe read it.",
				usererr.ActionChangeOption)
		}
		return &engine.Response{Outputs: []string{out}}, nil
	}

	// No text layer: it is a scan, so OCR the page images.
	bin, err := e.tesseractPath()
	if err != nil {
		return nil, err
	}

	pages, err := e.pageImages(req, in, conf)
	if err != nil {
		return nil, err
	}

	var parts []string
	for i, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress(engine.Progress{
			Fraction: float64(i) / float64(len(pages)),
			Stage:    fmt.Sprintf("Reading page %d of %d", i+1, len(pages)),
		})

		source := page
		if req.Options.Bool("enhance", true) {
			if prepared, prepErr := e.prepare(page, req); prepErr == nil {
				source = prepared
			}
		}
		text, err := e.readText(ctx, bin, source, req.Options)
		if err != nil {
			return nil, err
		}
		parts = append(parts, text)
	}

	out := req.Workspace.UniqueName(nameFor(in, ".txt"))
	if err := os.WriteFile(out, []byte(strings.Join(parts, "\n\n")), 0o644); err != nil {
		return nil, err
	}
	return &engine.Response{
		Outputs: []string{out},
		Notes:   []string{"This PDF was a scan, so the text was read from the page images."},
	}, nil
}

// searchablePDF rebuilds the document with an invisible text layer over the
// original page images: it looks identical but can be searched and copied.
func (e *Engine) searchablePDF(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	bin, err := e.tesseractPath()
	if err != nil {
		return nil, err
	}

	in := req.Inputs[0]
	conf := pdfConf(req.Options.String("password", ""))

	pages, err := e.pageImages(req, in, conf)
	if err != nil {
		return nil, err
	}

	outDir, err := req.Workspace.Sub("searchable")
	if err != nil {
		return nil, err
	}

	var built []string
	for i, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		progress(engine.Progress{
			Fraction: float64(i) / float64(len(pages)),
			Stage:    fmt.Sprintf("Reading page %d of %d", i+1, len(pages)),
		})

		// The text layer is derived from the enhanced image, but the page
		// itself keeps the original picture, so the result looks unchanged.
		source := page
		if req.Options.Bool("enhance", true) {
			if prepared, prepErr := e.prepare(page, req); prepErr == nil {
				source = prepared
			}
		}

		stem := filepath.Join(outDir, fmt.Sprintf("page-%04d", i+1))
		args := append(tessArgs(source, stem, req.Options), "pdf")
		if _, err := e.runner.Run(ctx, bin, args, lexec.Options{Timeout: perPageTimeout}); err != nil {
			return nil, translateTesseract(err)
		}
		built = append(built, stem+".pdf")
	}

	if len(built) == 0 {
		return nil, usererr.New(usererr.CodeNoTextFound,
			"No pages could be read from this PDF.", usererr.ActionChooseFile)
	}

	out := req.Workspace.UniqueName(nameFor(in, "-searchable.pdf"))
	if len(built) == 1 {
		if err := os.Rename(built[0], out); err != nil {
			return nil, err
		}
	} else if err := api.MergeCreateFile(built, out, false, pdfConf("")); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

// pageImages extracts the page pictures from a scanned PDF.
func (e *Engine) pageImages(req engine.Request, in string, conf *pdfConfiguration) ([]string, error) {
	dir, err := req.Workspace.Sub("pages")
	if err != nil {
		return nil, err
	}
	if err := api.ExtractImagesFile(in, dir, nil, conf); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var pages []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".tif", ".tiff":
			pages = append(pages, filepath.Join(dir, entry.Name()))
		}
	}
	if len(pages) == 0 {
		return nil, usererr.New(usererr.CodeUnsupportedInput,
			"This PDF already contains text rather than scanned pictures, so there is nothing to read.",
			usererr.ActionChangeOption)
	}
	sortPages(pages)
	return pages, nil
}

// prepare writes the enhanced copy of an image into the workspace. The input
// is only ever read.
func (e *Engine) prepare(in string, req engine.Request) (string, error) {
	f, err := os.Open(in)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	src, _, err := image.Decode(f)
	if err != nil {
		return "", usererr.Wrap(err, usererr.CodeCorruptInput,
			fmt.Sprintf("%s couldn't be read as an image.", filepath.Base(in)),
			usererr.ActionChooseFile)
	}

	dir, err := req.Workspace.Sub("enhanced")
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, strconv.FormatInt(time.Now().UnixNano(), 36)+".png")

	dst, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer func() { _ = dst.Close() }()

	if err := png.Encode(dst, enhance(src)); err != nil {
		return "", err
	}
	return out, dst.Sync()
}

// readText runs Tesseract and returns its plain-text output.
func (e *Engine) readText(ctx context.Context, bin, in string, opts engine.Options) (string, error) {
	res, err := e.runner.Run(ctx, bin, tessArgs(in, "stdout", opts), lexec.Options{Timeout: perPageTimeout})
	if err != nil {
		return "", translateTesseract(err)
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// tessArgs builds the Tesseract command line. Multi-language recognition in one
// pass — "-l eng+hin" — matters because real documents mix scripts.
func tessArgs(in, out string, opts engine.Options) []string {
	args := []string{in, out}

	if lang := strings.TrimSpace(opts.String("lang", "eng")); lang != "" {
		args = append(args, "-l", lang)
	}
	if psm := pageSegMode(opts.String("psm", "auto")); psm != "" {
		args = append(args, "--psm", psm)
	}
	return args
}

// pageSegMode maps the plain wording on the task screen to Tesseract's numeric
// page segmentation modes.
func pageSegMode(choice string) string {
	switch choice {
	case "block":
		return "6"
	case "line":
		return "7"
	case "word":
		return "8"
	case "sparse":
		return "11"
	case "auto":
		return "3"
	default:
		return ""
	}
}

// qualityNote tells the user when the input, not the engine, is the limit.
// Guiding someone to a better photo is more useful than silently returning
// nonsense.
func qualityNote(path, text string) string {
	if len(strings.TrimSpace(text)) >= minUsefulLength {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	cfg, _, err := image.DecodeConfig(f)
	if err == nil && (cfg.Width < 1000 || cfg.Height < 1000) {
		return "This image is fairly low resolution, so little text was found. " +
			"Taking the photo straight on, in good light and closer up usually helps."
	}
	return "Very little text was found in this image."
}

func translateTesseract(err error) error {
	var exit *lexec.ExitError
	if ok := asExit(err, &exit); ok && strings.Contains(strings.ToLower(exit.Stderr), "empty page") {
		return usererr.New(usererr.CodeNoTextFound,
			"No text was found in this image. If the text is small or blurry, try a clearer photo.",
			usererr.ActionChooseFile, usererr.ActionChangeOption)
	}
	return err
}

func nameFor(in, suffix string) string {
	return strings.TrimSuffix(filepath.Base(in), filepath.Ext(in)) + suffix
}

func readAll(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sortPages(names)

	var b strings.Builder
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

var _ engine.Engine = (*Engine)(nil)
