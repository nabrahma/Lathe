// Package imageengine performs image tasks in pure Go.
//
// Choosing pure Go over libvips keeps CGO_ENABLED=0, which keeps
// cross-compilation trivial and removes a shared library from every installer.
// The formats it covers (JPEG, PNG, GIF, BMP, TIFF and WebP) are the
// overwhelming majority of real input. HEIC and AVIF have no pure-Go decoder
// worth depending on and are handled by the media component instead.
package imageengine

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"github.com/disintegration/imaging"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp" // decode-only support, registered for image.Decode

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/usererr"
)

// Engine performs image tasks.
type Engine struct{}

// New returns the image engine.
func New() *Engine { return &Engine{} }

// ID identifies the engine in the task registry.
func (e *Engine) ID() string { return "image" }

// Available is always true: the engine is compiled in.
func (e *Engine) Available() bool { return true }

// Formats Lathe can write. Decoding covers these plus WebP.
var writable = map[string]bool{
	"jpg": true, "jpeg": true, "png": true, "webp": true,
	"tiff": true, "tif": true, "bmp": true, "gif": true,
}

// Execute dispatches to the handler for the requested task.
func (e *Engine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
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
			Stage:    fmt.Sprintf("Processing %s", filepath.Base(in)),
		})

		out, note, err := e.one(req, in)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, out)
		if note != "" && !contains(notes, note) {
			notes = append(notes, note)
		}
	}

	progress(engine.Progress{Fraction: 1, Stage: "Finishing"})
	return &engine.Response{Outputs: outputs, Notes: notes}, nil
}

func (e *Engine) one(req engine.Request, in string) (out, note string, err error) {
	src, err := load(in)
	if err != nil {
		return "", "", err
	}

	var (
		img    = src
		format = strings.ToLower(req.Options.String("format", ""))
	)

	switch req.Task.ID {
	case "image.convert":
		// The format is the point of this task.
	case "image.compress":
		if max := req.Options.Int("maxWidth", 0); max > 0 && img.Bounds().Dx() > max {
			img = imaging.Resize(img, max, 0, imaging.Lanczos)
		}
		if format == "" {
			format = keepOrDefault(in, "jpg")
		}
	case "image.resize":
		if img, err = resize(img, req.Options); err != nil {
			return "", "", err
		}
		if format == "" {
			format = keepOrDefault(in, "png")
		}
	case "image.crop":
		if img, err = crop(img, req.Options); err != nil {
			return "", "", err
		}
		if format == "" {
			format = keepOrDefault(in, "png")
		}
	default:
		return "", "", fmt.Errorf("image engine cannot handle task %q", req.Task.ID)
	}

	if format == "" {
		format = "jpg"
	}
	if !writable[format] {
		return "", "", usererr.New(usererr.CodeInvalidOptions,
			fmt.Sprintf("Lathe can't save images as %s yet.", strings.ToUpper(format)),
			usererr.ActionChangeOption)
	}

	// Flattening happens before encoding, not during, so the note is accurate.
	if losesTransparency(format) && hasAlpha(img) {
		img = flatten(img)
		note = "Transparent areas became white, because " + strings.ToUpper(format) + " cannot store transparency."
	}

	base := filepath.Base(in)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	out = req.Workspace.UniqueName(base + "." + format)

	if err := encode(out, img, format, req.Options.Int("quality", 85)); err != nil {
		return "", "", err
	}
	return out, note, nil
}

// load decodes an image, translating the two failures users actually hit into
// something they can act on.
func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err == nil {
		return img, nil
	}

	// A HEIC or AVIF named .jpg is the common case, and the honest answer is
	// that it needs the media component rather than that the file is broken.
	if head, seekErr := readHead(f); seekErr == nil {
		if brand := heifBrand(head); brand != "" {
			return nil, usererr.New(usererr.CodeComponentMissing,
				fmt.Sprintf("%s is a %s photo, which needs the video and photo component. It downloads once and works offline afterwards.",
					filepath.Base(path), strings.ToUpper(brand)),
				usererr.ActionDownload)
		}
	}
	return nil, usererr.Wrap(err, usererr.CodeCorruptInput,
		fmt.Sprintf("%s couldn't be read as an image. It may be damaged.", filepath.Base(path)),
		usererr.ActionChooseFile, usererr.ActionCopyDetails)
}

func readHead(f *os.File) ([]byte, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	head := make([]byte, 16)
	n, err := io.ReadFull(f, head)
	if err != nil && n < 12 {
		return nil, err
	}
	return head[:n], nil
}

func heifBrand(head []byte) string {
	if len(head) < 12 || string(head[4:8]) != "ftyp" {
		return ""
	}
	switch string(head[8:12]) {
	case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
		return "heic"
	case "avif", "avis":
		return "avif"
	}
	return ""
}

func encode(path string, img image.Image, format string, quality int) error {
	if quality < 1 || quality > 100 {
		quality = 85
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	switch format {
	case "jpg", "jpeg":
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		err = enc.Encode(f, img)
	case "webp":
		err = nativewebp.Encode(f, img, nil)
	case "tiff", "tif":
		err = tiff.Encode(f, img, &tiff.Options{Compression: tiff.Deflate})
	case "bmp":
		err = bmp.Encode(f, img)
	case "gif":
		err = gif.Encode(f, img, nil)
	default:
		err = fmt.Errorf("no encoder for %q", format)
	}
	if err != nil {
		return err
	}
	return f.Sync()
}

// resizePresets are the sizes people actually ask for by name.
var resizePresets = map[string][2]int{
	"passport": {413, 531},
	"profile":  {512, 512},
	"hd":       {1920, 0},
}

func resize(img image.Image, opts engine.Options) (image.Image, error) {
	width := opts.Int("width", 0)
	height := opts.Int("height", 0)

	if preset := opts.String("preset", "custom"); preset != "custom" {
		dims, ok := resizePresets[preset]
		if !ok {
			return nil, usererr.New(usererr.CodeInvalidOptions,
				"That size isn't one Lathe knows.", usererr.ActionChangeOption)
		}
		width, height = dims[0], dims[1]
	}

	if width <= 0 && height <= 0 {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Set a width or a height to resize to.", usererr.ActionChangeOption)
	}

	// Resize keeps proportions when one dimension is zero; Fill crops to fit
	// when the caller explicitly asked for both and turned proportions off.
	if width > 0 && height > 0 && !opts.Bool("keepAspect", true) {
		return imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos), nil
	}
	if width > 0 && height > 0 {
		return imaging.Fit(img, width, height, imaging.Lanczos), nil
	}
	return imaging.Resize(img, width, height, imaging.Lanczos), nil
}

func crop(img image.Image, opts engine.Options) (image.Image, error) {
	if rect := strings.TrimSpace(opts.String("rect", "")); rect != "" {
		x, y, w, h, err := parseRect(rect)
		if err != nil {
			return nil, err
		}
		cropped := imaging.Crop(img, image.Rect(x, y, x+w, y+h))
		if cropped.Bounds().Empty() {
			return nil, usererr.New(usererr.CodeInvalidOptions,
				"That crop area falls outside the picture.", usererr.ActionChangeOption)
		}
		return cropped, nil
	}

	aspect := opts.String("aspect", "free")
	if aspect == "free" {
		return nil, usererr.New(usererr.CodeInvalidOptions,
			"Choose a shape to crop to, or set the area manually.", usererr.ActionChangeOption)
	}

	ratioW, ratioH, err := parseAspect(aspect)
	if err != nil {
		return nil, err
	}

	// Largest centred rectangle of the requested shape.
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w*ratioH > h*ratioW {
		w = h * ratioW / ratioH
	} else {
		h = w * ratioH / ratioW
	}
	return imaging.CropCenter(img, w, h), nil
}

func parseRect(spec string) (x, y, w, h int, err error) {
	parts := strings.Split(spec, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, usererr.New(usererr.CodeInvalidOptions,
			"Write the crop area as four numbers: left, top, width, height.", usererr.ActionChangeOption)
	}
	out := make([]int, 4)
	for i, p := range parts {
		n, convErr := strconv.Atoi(strings.TrimSpace(p))
		if convErr != nil || n < 0 {
			return 0, 0, 0, 0, usererr.New(usererr.CodeInvalidOptions,
				fmt.Sprintf("%q is not a number of pixels.", strings.TrimSpace(p)), usererr.ActionChangeOption)
		}
		out[i] = n
	}
	if out[2] == 0 || out[3] == 0 {
		return 0, 0, 0, 0, usererr.New(usererr.CodeInvalidOptions,
			"The crop area needs a width and a height.", usererr.ActionChangeOption)
	}
	return out[0], out[1], out[2], out[3], nil
}

func parseAspect(spec string) (w, h int, err error) {
	parts := strings.Split(spec, ":")
	if len(parts) != 2 {
		return 0, 0, usererr.New(usererr.CodeInvalidOptions,
			"Write the shape as two numbers, such as 4:3.", usererr.ActionChangeOption)
	}
	if w, err = strconv.Atoi(parts[0]); err != nil || w <= 0 {
		return 0, 0, usererr.New(usererr.CodeInvalidOptions, "That shape isn't valid.", usererr.ActionChangeOption)
	}
	if h, err = strconv.Atoi(parts[1]); err != nil || h <= 0 {
		return 0, 0, usererr.New(usererr.CodeInvalidOptions, "That shape isn't valid.", usererr.ActionChangeOption)
	}
	return w, h, nil
}

func losesTransparency(format string) bool {
	return format == "jpg" || format == "jpeg" || format == "bmp"
}

func hasAlpha(img image.Image) bool {
	switch img.(type) {
	case *image.RGBA, *image.NRGBA, *image.RGBA64, *image.NRGBA64, *image.Paletted:
	default:
		return false
	}

	// Sampling the whole image is wasteful on a 50 MP photo; a coarse grid
	// finds any real transparency and costs nothing.
	b := img.Bounds()
	stepX := max(1, b.Dx()/64)
	stepY := max(1, b.Dy()/64)
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			if _, _, _, a := img.At(x, y).RGBA(); a < 0xFFFF {
				return true
			}
		}
	}
	return false
}

// flatten composites onto white, which is what people expect when a
// transparent PNG becomes a JPEG.
func flatten(img image.Image) image.Image {
	b := img.Bounds()
	out := imaging.New(b.Dx(), b.Dy(), color.White)
	return imaging.Paste(out, img, image.Pt(0, 0))
}

// keepOrDefault preserves the input's format where Lathe can write it, so a
// resize does not silently change a PNG into a JPEG.
func keepOrDefault(in, fallback string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(in), "."))
	if writable[ext] {
		return ext
	}
	return fallback
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
