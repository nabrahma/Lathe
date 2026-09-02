// Package pdfengine implements every PDF task in pure Go via pdfcpu.
//
// It needs no external binary and no downloaded component, so PDF work is
// available the moment Lathe is installed. That is the reason the PDF tasks
// are the ones on the home screen by default.
package pdfengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/usererr"
)

// Engine performs PDF tasks.
type Engine struct{}

// New returns the PDF engine.
func New() *Engine { return &Engine{} }

// ID identifies the engine in the task registry.
func (e *Engine) ID() string { return "pdfcpu" }

// Available is always true: the engine is compiled in.
func (e *Engine) Available() bool { return true }

// Execute dispatches to the handler for the requested task.
func (e *Engine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch req.Task.ID {
	case "pdf.compress":
		return compress(req, progress)
	case "pdf.merge":
		return merge(req, progress)
	case "pdf.split":
		return split(req, progress)
	case "pdf.rotate":
		return rotate(req, progress)
	case "pdf.delete-pages":
		return deletePages(req, progress)
	case "pdf.reorder":
		return reorder(req, progress)
	case "pdf.watermark":
		return watermark(req, progress)
	case "pdf.protect":
		return protect(req, progress)
	case "pdf.unlock":
		return unlock(req, progress)
	case "pdf.to-images":
		return toImages(req, progress)
	case "pdf.from-images":
		return fromImages(req, progress)
	default:
		return nil, fmt.Errorf("pdf engine cannot handle task %q", req.Task.ID)
	}
}

// conf builds a pdfcpu configuration. pdfcpu writes a config directory on
// first use; Lathe disables that so nothing appears in the user's home folder
// without being asked.
func conf(password string) *model.Configuration {
	c := model.NewDefaultConfiguration()
	c.ValidationMode = model.ValidationRelaxed
	if password != "" {
		c.UserPW = password
		c.OwnerPW = password
	}
	return c
}

func init() {
	// A file converter has no business creating a dotfile directory on first
	// run; pdfcpu defaults to doing so.
	api.DisableConfigDir()
}

// pageCount reads the page count, translating pdfcpu's password errors into
// something the interface can act on.
func pageCount(path, password string) (int, error) {
	n, err := api.PageCountFile(path)
	if err != nil {
		if password == "" {
			return 0, err
		}
		// PageCountFile takes no configuration, so an encrypted file needs the
		// full read path.
		ctxt, readErr := api.ReadContextFile(path)
		if readErr != nil {
			return 0, readErr
		}
		return ctxt.PageCount, nil
	}
	return n, nil
}

func compress(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Compressing"))

	in := req.Inputs[0]
	out := req.Workspace.Path(outputName(in, "-compressed"))
	password := req.Options.String("password", "")

	c := conf(password)
	// pdfcpu's optimiser removes redundant objects and shared resources. The
	// quality setting decides how aggressively embedded images are rewritten.
	switch req.Options.String("quality", "medium") {
	case "low":
		c.OptimizeDuplicateContentStreams = true
	case "high":
		c.OptimizeDuplicateContentStreams = false
	default:
		c.OptimizeDuplicateContentStreams = true
	}

	if err := api.OptimizeFile(in, out, c); err != nil {
		return nil, err
	}

	// Optimisation is lossless, so on an already-optimised file the result can
	// be larger. Returning the smaller of the two is what the user asked for.
	inInfo, err1 := os.Stat(in)
	outInfo, err2 := os.Stat(out)
	notes := []string{}
	if err1 == nil && err2 == nil && outInfo.Size() >= inInfo.Size() {
		notes = append(notes, "This PDF was already about as small as it can get without losing quality.")
	}
	return &engine.Response{Outputs: []string{out}, Notes: notes}, nil
}

func merge(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Merging %d files", len(req.Inputs))})

	out := req.Workspace.Path("merged.pdf")
	if err := api.MergeCreateFile(req.Inputs, out, false, conf(req.Options.String("password", ""))); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func split(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	in := req.Inputs[0]
	password := req.Options.String("password", "")
	c := conf(password)

	total, err := pageCount(in, password)
	if err != nil {
		return nil, err
	}

	dir, err := req.Workspace.Sub("split")
	if err != nil {
		return nil, err
	}

	switch req.Options.String("mode", "pages") {
	case "range":
		spec := req.Options.String("pages", "")
		pages, err := engine.ParsePages(spec, total)
		if err != nil {
			return nil, usererr.Wrap(err, usererr.CodeInvalidOptions, capitalise(err.Error())+".",
				usererr.ActionChangeOption)
		}
		progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Extracting %d pages", len(pages))})

		out := filepath.Join(dir, outputName(in, "-pages"))
		if err := api.CollectFile(in, out, pages.Strings(), c); err != nil {
			return nil, err
		}
		return &engine.Response{Outputs: []string{out}}, nil

	default:
		span := req.Options.Int("span", 1)
		if req.Options.String("mode", "pages") == "pages" {
			span = 1
		}
		if span < 1 {
			span = 1
		}
		progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Splitting %d pages", total)})

		if err := api.SplitFile(in, dir, span, c); err != nil {
			return nil, err
		}
		return &engine.Response{Outputs: collectFrom(dir, ".pdf")}, nil
	}
}

func rotate(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Rotating pages"))

	in := req.Inputs[0]
	password := req.Options.String("password", "")
	total, err := pageCount(in, password)
	if err != nil {
		return nil, err
	}

	pages, err := engine.ParsePages(req.Options.String("pages", ""), total)
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeInvalidOptions, capitalise(err.Error())+".",
			usererr.ActionChangeOption)
	}

	angle := req.Options.Int("angle", 90)
	if angle%90 != 0 {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Pages can only be turned by 90, 180 or 270 degrees.", usererr.ActionChangeOption)
	}

	out := req.Workspace.Path(outputName(in, "-rotated"))
	if err := api.RotateFile(in, out, angle, pages.Strings(), conf(password)); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func deletePages(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	in := req.Inputs[0]
	password := req.Options.String("password", "")

	total, err := pageCount(in, password)
	if err != nil {
		return nil, err
	}

	spec := req.Options.String("pages", "")
	if strings.TrimSpace(spec) == "" {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Choose which pages to remove.", usererr.ActionChangeOption)
	}
	pages, err := engine.ParsePages(spec, total)
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeInvalidOptions, capitalise(err.Error())+".",
			usererr.ActionChangeOption)
	}
	if len(pages) >= total {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"That would remove every page, leaving an empty document.", usererr.ActionChangeOption)
	}

	progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Removing %d of %d pages", len(pages), total)})

	out := req.Workspace.Path(outputName(in, "-edited"))
	if err := api.RemovePagesFile(in, out, pages.Strings(), conf(password)); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func reorder(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Rearranging pages"))

	in := req.Inputs[0]
	password := req.Options.String("password", "")

	total, err := pageCount(in, password)
	if err != nil {
		return nil, err
	}

	order, err := parseOrder(req.Options.String("order", ""), total)
	if err != nil {
		return nil, err
	}

	out := req.Workspace.Path(outputName(in, "-reordered"))
	// Collect assembles a new document in the order given, which is exactly
	// what a drag-to-reorder gesture produces.
	if err := api.CollectFile(in, out, order, conf(password)); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func watermark(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Adding watermark"))

	in := req.Inputs[0]
	text := strings.TrimSpace(req.Options.String("text", "DRAFT"))
	if text == "" {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Enter the text to stamp across the pages.", usererr.ActionChangeOption)
	}

	opacity := req.Options.Float("opacity", 0.3)
	if opacity < 0.05 {
		opacity = 0.05
	}
	if opacity > 1 {
		opacity = 1
	}

	rotation := 0
	switch req.Options.String("position", "center") {
	case "diagonal":
		rotation = 45
	case "bottom":
		rotation = 0
	}

	// pdfcpu takes its watermark settings as a description string. The keys
	// must be spelled out: "op" is ambiguous between opacity and offset, and
	// "sc" between scalefactor and scriptname.
	desc := fmt.Sprintf("fontname:Helvetica, points:48, opacity:%.2f, rotation:%d, scalefactor:0.7 abs, position:%s, fillcolor:#808080",
		opacity, rotation, anchorFor(req.Options.String("position", "center")))

	out := req.Workspace.Path(outputName(in, "-watermarked"))
	if err := api.AddTextWatermarksFile(in, out, nil, true, text, desc,
		conf(req.Options.String("password", ""))); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func protect(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Adding password"))

	password := req.Options.String("password", "")
	if len(password) < 4 {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Choose a password of at least four characters.", usererr.ActionChangeOption)
	}

	in := req.Inputs[0]
	out := req.Workspace.Path(outputName(in, "-protected"))

	c := model.NewAESConfiguration(password, password, 256)
	c.ValidationMode = model.ValidationRelaxed
	if err := api.EncryptFile(in, out, c); err != nil {
		return nil, err
	}
	return &engine.Response{
		Outputs: []string{out},
		Notes:   []string{"Keep this password somewhere safe. Without it the file cannot be opened, by anyone."},
	}, nil
}

func unlock(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Indeterminate("Removing password"))

	password := req.Options.String("password", "")
	if password == "" {
		// Lathe removes a password the user already has. It never attempts to
		// crack or bypass one, and says so rather than leaving it ambiguous.
		return nil, usererr.New(usererr.CodePasswordRequired,
			"Enter the password that opens this PDF. Lathe can only remove a password you already know.",
			usererr.ActionEnterPassword)
	}

	in := req.Inputs[0]
	out := req.Workspace.Path(outputName(in, "-unlocked"))
	if err := api.DecryptFile(in, out, conf(password)); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func toImages(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	in := req.Inputs[0]
	password := req.Options.String("password", "")

	total, err := pageCount(in, password)
	if err != nil {
		return nil, err
	}
	pages, err := engine.ParsePages(req.Options.String("pages", ""), total)
	if err != nil {
		return nil, usererr.Wrap(err, usererr.CodeInvalidOptions, capitalise(err.Error())+".",
			usererr.ActionChangeOption)
	}

	progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Exporting %d pages", len(pages))})

	dir, err := req.Workspace.Sub("images")
	if err != nil {
		return nil, err
	}
	if err := api.ExtractImagesFile(in, dir, pages.Strings(), conf(password)); err != nil {
		return nil, err
	}

	outputs := collectFrom(dir, ".png", ".jpg", ".jpeg", ".tif", ".tiff")
	if len(outputs) == 0 {
		// Pages drawn as vector content have no embedded image to extract.
		return nil, usererr.New(usererr.CodeUnsupportedInput,
			"These pages are drawn as text and shapes rather than scanned pictures, so there are no images to pull out.",
			usererr.ActionChangeOption)
	}
	return &engine.Response{Outputs: outputs}, nil
}

func fromImages(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Placing %d images", len(req.Inputs))})

	imp := api.DefaultImportConfig()
	switch size := req.Options.String("pageSize", "A4"); size {
	case "fit":
		// One page per image, sized to the image itself.
		imp.PageSize = "A4"
		imp.Pos = types.Full
	default:
		imp.PageSize = size
		if dim, ok := types.PaperSize[size]; ok {
			imp.PageDim = dim
		}
		imp.UserDim = true
	}

	if req.Options.String("orientation", "auto") == "landscape" && imp.PageDim != nil {
		imp.PageDim = &types.Dim{Width: imp.PageDim.Height, Height: imp.PageDim.Width}
	}

	out := req.Workspace.Path("images.pdf")
	if err := api.ImportImagesFile(req.Inputs, out, imp, conf("")); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

// parseOrder reads a comma-separated page order such as "3, 1, 2". Unlike a
// page range it is order-significant and may repeat a page.
func parseOrder(spec string, total int) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Set the page order first, for example 3, 1, 2.", usererr.ActionChangeOption)
	}

	var order []string
	for _, part := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ' ' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pages, err := engine.ParsePages(part, total)
		if err != nil {
			return nil, usererr.Wrap(err, usererr.CodeInvalidOptions, capitalise(err.Error())+".",
				usererr.ActionChangeOption)
		}
		order = append(order, pages.Strings()...)
	}
	if len(order) == 0 {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"No pages were listed in the order.", usererr.ActionChangeOption)
	}
	return order, nil
}

// collectFrom lists files an operation wrote into a directory, sorted so
// multi-page output arrives in page order.
func collectFrom(dir string, exts ...string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, want := range exts {
			if ext == want {
				out = append(out, filepath.Join(dir, e.Name()))
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// outputName derives a result filename from the input, so a user recognises
// what they got back.
func outputName(in, suffix string) string {
	base := filepath.Base(in)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return base + suffix + ".pdf"
}

func anchorFor(position string) string {
	switch position {
	case "bottom":
		return "bc"
	default:
		return "c"
	}
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

var _ engine.Engine = (*Engine)(nil)
