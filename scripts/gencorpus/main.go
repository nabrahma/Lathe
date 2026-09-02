// Command gencorpus writes the golden test corpus.
//
// Every file is generated rather than collected, so the corpus is small,
// deterministic, and unambiguously free to redistribute in a public
// repository. The adversarial set is the part that matters: hostile filenames,
// lying extensions, truncated files and zero-byte files are where the real
// bugs live.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

const root = "testdata/corpus"

func main() {
	api.DisableConfigDir()

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gencorpus:", err)
		os.Exit(1)
	}
	fmt.Println("corpus written to", root)
}

func run() error {
	// Start clean. pdfcpu's ImportImagesFile appends to an existing output
	// rather than replacing it, so regenerating over a previous run silently
	// doubles every page count.
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	for _, dir := range []string{"pdf", "images", "adversarial"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	if err := writePDFs(); err != nil {
		return err
	}
	if err := writeImages(); err != nil {
		return err
	}
	if err := writeAdversarial(); err != nil {
		return err
	}
	if err := writeOCRCorpus(); err != nil {
		return err
	}
	return writeManifest()
}

func writePDFs() error {
	dir := filepath.Join(root, "pdf")

	for _, spec := range []struct {
		name  string
		pages int
	}{
		{"single-page.pdf", 1},
		{"five-page.pdf", 5},
		{"many-page.pdf", 60},
	} {
		if err := makePDF(filepath.Join(dir, spec.name), spec.pages); err != nil {
			return fmt.Errorf("%s: %w", spec.name, err)
		}
	}

	// A password-protected PDF, since "compress a locked file" is a real flow.
	plain := filepath.Join(dir, "single-page.pdf")
	locked := filepath.Join(dir, "password-protected.pdf")
	conf := model.NewAESConfiguration("lathe", "lathe", 256)
	conf.ValidationMode = model.ValidationRelaxed
	if err := api.EncryptFile(plain, locked, conf); err != nil {
		return fmt.Errorf("password-protected.pdf: %w", err)
	}

	// A scan-like PDF: one full-page image per page, which is what a phone or
	// a flatbed produces and what OCR has to cope with.
	return makeScanPDF(filepath.Join(dir, "scanned.pdf"), 2)
}

// makePDF builds a text PDF by importing rendered pages, which keeps the
// generator free of a PDF writer of its own.
func makePDF(path string, pages int) error {
	tmp, err := os.MkdirTemp("", "lathe-corpus-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	var imgs []string
	for i := 1; i <= pages; i++ {
		p := filepath.Join(tmp, fmt.Sprintf("page-%03d.png", i))
		if err := writePNG(p, textPage(i, pages)); err != nil {
			return err
		}
		imgs = append(imgs, p)
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return api.ImportImagesFile(imgs, path, api.DefaultImportConfig(), conf)
}

func makeScanPDF(path string, pages int) error {
	tmp, err := os.MkdirTemp("", "lathe-corpus-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	var imgs []string
	for i := 1; i <= pages; i++ {
		p := filepath.Join(tmp, fmt.Sprintf("scan-%03d.jpg", i))
		if err := writeJPEG(p, noisyPage(i), 70); err != nil {
			return err
		}
		imgs = append(imgs, p)
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return api.ImportImagesFile(imgs, path, api.DefaultImportConfig(), conf)
}

func writeImages() error {
	dir := filepath.Join(root, "images")

	if err := writePNG(filepath.Join(dir, "photo.png"), gradient(640, 480)); err != nil {
		return err
	}
	if err := writePNG(filepath.Join(dir, "transparent.png"), withAlpha(320, 320)); err != nil {
		return err
	}
	if err := writeJPEG(filepath.Join(dir, "photo.jpg"), gradient(800, 600), 90); err != nil {
		return err
	}
	if err := writeJPEG(filepath.Join(dir, "large.jpg"), gradient(4000, 3000), 85); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "photo.bmp"))
	if err != nil {
		return err
	}
	if err := bmp.Encode(f, gradient(200, 150)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	tf, err := os.Create(filepath.Join(dir, "scan.tiff"))
	if err != nil {
		return err
	}
	if err := tiff.Encode(tf, gradient(300, 400), &tiff.Options{Compression: tiff.Deflate}); err != nil {
		_ = tf.Close()
		return err
	}
	if err := tf.Close(); err != nil {
		return err
	}

	gf, err := os.Create(filepath.Join(dir, "animation.gif"))
	if err != nil {
		return err
	}
	defer func() { _ = gf.Close() }()
	return gif.EncodeAll(gf, animation())
}

func writeAdversarial() error {
	dir := filepath.Join(root, "adversarial")

	// A zero-byte file: every engine fails on this in its own idiosyncratic way.
	if err := os.WriteFile(filepath.Join(dir, "empty.pdf"), nil, 0o644); err != nil {
		return err
	}

	// A truncated PDF: plausible header, no body.
	if err := os.WriteFile(filepath.Join(dir, "truncated.pdf"),
		[]byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog"), 0o644); err != nil {
		return err
	}

	// An HTML error page saved as .pdf, which is what a failed download leaves.
	if err := os.WriteFile(filepath.Join(dir, "actually-html.pdf"),
		[]byte("<!DOCTYPE html>\n<html><body><h1>404 Not Found</h1></body></html>\n"), 0o644); err != nil {
		return err
	}

	// A PNG named .jpg: the lying-extension case, in its most common form.
	if err := writePNG(filepath.Join(dir, "actually-png.jpg"), gradient(64, 64)); err != nil {
		return err
	}

	// A HEIC header named .jpg, exactly as a phone shares it.
	heic := make([]byte, 128)
	copy(heic[0:4], []byte{0, 0, 0, 0x18})
	copy(heic[4:8], []byte("ftyp"))
	copy(heic[8:12], []byte("heic"))
	copy(heic[16:24], []byte("mif1heic"))
	if err := os.WriteFile(filepath.Join(dir, "actually-heic.jpg"), heic, 0o644); err != nil {
		return err
	}

	// Hostile filenames. Every one of these is inert only because arguments are
	// passed as a slice and never through a shell.
	hostile := []string{
		"spaces in the name.png",
		"apostrophe's file.png",
		"semicolon; rm -rf tmp.png",
		"dollar $(whoami).png",
		"backtick `id`.png",
		"ampersand & pipe.png",
		"unicode-café-日本語.png",
		"emoji-\U0001F4C4.png",
		strings.Repeat("very-long-name-", 12) + ".png",
	}
	if runtime.GOOS != "windows" {
		// Windows rejects these characters in a filename outright, so the
		// corpus cannot carry them there. Unix can, and the argument-escaping
		// tests want them.
		hostile = append(hostile,
			`double"quote.png`,
			"question?mark.png",
			"asterisk*star.png",
			"pipe|char.png",
			"newline\nin-name.png",
		)
	}
	for _, name := range hostile {
		if err := writePNG(filepath.Join(dir, name), gradient(32, 32)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func writeManifest() error {
	manifest := `# Golden corpus
#
# Every file here is generated by scripts/gencorpus, so the corpus is
# deterministic and unambiguously free to redistribute. Regenerate with
# "make corpus" after changing the generator.
#
# license: all files are original work, released under the repository licence.

pdf:
  single-page.pdf:      one page; the baseline for every PDF task
  five-page.pdf:        five pages; exercises page ranges, split, reorder
  many-page.pdf:        sixty pages; catches per-page work that scales badly
  password-protected.pdf: encrypted with the password "lathe"; unlock and the
                        up-front password prompt
  scanned.pdf:          full-page images rather than text; the OCR path

images:
  photo.png:            8-bit RGB, the ordinary case
  transparent.png:      has an alpha channel; conversion to JPEG must flatten
  photo.jpg:            baseline JPEG
  large.jpg:            4000x3000; memory behaviour on a real phone photo
  photo.bmp:            uncompressed, an uncommon input that still turns up
  scan.tiff:            deflate-compressed TIFF, as scanners produce
  animation.gif:        multi-frame; only the first frame survives conversion

adversarial:
  empty.pdf:            zero bytes; must fail with a specific message
  truncated.pdf:        valid header, no body; must not be reported as success
  actually-html.pdf:    an HTML error page from a failed download
  actually-png.jpg:     a PNG with a lying extension
  actually-heic.jpg:    a HEIC header named .jpg, as phones share it
  "spaces in the name.png":     filename with spaces
  "apostrophe's file.png":      single quote
  "semicolon; rm -rf tmp.png":  shell metacharacters; must be inert
  "dollar $(whoami).png":       command substitution; must be inert
  "backtick (id).png":          backtick substitution; must be inert
  "ampersand & pipe.png":       shell operators in a name
  unicode-café-日本語.png:      non-ASCII filename
  emoji-📄.png:                 astral-plane character in a filename
  very-long-name-*.png:         long filename, near path-length limits

  # Unix only. Windows rejects these characters in a filename outright, so
  # on Windows the corpus simply does not contain them.
  'double"quote.png':          double quote
  question?mark.png:            glob character
  asterisk*star.png:            glob character
  pipe|char.png:                shell pipe
  newline-in-name.png:          literal newline inside the filename
`
	return os.WriteFile(filepath.Join(root, "MANIFEST.yaml"), []byte(manifest+ocrManifest()), 0o644)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeJPEG(path string, img image.Image, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: quality}); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func gradient(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(255 * x / w),
				G: uint8(255 * y / h),
				B: uint8(128 + (x+y)%128),
				A: 255,
			})
		}
	}
	return img
}

func withAlpha(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy := w/2, h/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// A soft circle, so the transparent region is unmistakable.
			d := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			a := uint8(0)
			if d < cx*cx {
				a = 255
			}
			img.Set(x, y, color.NRGBA{R: 0xFF, G: 0xE5, B: 0x00, A: a})
		}
	}
	return img
}

// textPage renders bold blocks that read as text at a glance and give the PDF
// tasks realistic per-page content without embedding a font.
func textPage(page, total int) image.Image {
	const w, h = 850, 1100
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	ink := color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}
	// A heading bar plus lines of varying length, per page.
	drawRect(img, 80, 90, 420, 34, ink)
	rnd := rand.New(rand.NewSource(int64(page * 7919)))
	for i := 0; i < 26; i++ {
		y := 180 + i*30
		width := 300 + rnd.Intn(390)
		drawRect(img, 80, y, width, 12, ink)
	}
	// A page-number block whose position encodes the page, so a reordered
	// document is visibly different from an unchanged one.
	drawRect(img, 80+((page-1)%6)*60, h-80, 40, 16, ink)
	_ = total
	return img
}

// noisyPage looks like a scan: off-white paper, speckle, slightly grey ink.
func noisyPage(page int) image.Image {
	const w, h = 850, 1100
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(int64(page * 104729)))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(238 + rnd.Intn(18))
			img.Set(x, y, color.RGBA{R: v, G: v, B: uint8(int(v) - 4), A: 255})
		}
	}
	ink := color.RGBA{R: 0x38, G: 0x38, B: 0x3A, A: 0xFF}
	for i := 0; i < 24; i++ {
		drawRect(img, 90, 160+i*34, 280+rnd.Intn(400), 11, ink)
	}
	return img
}

func drawRect(img draw.Image, x, y, w, h int, c color.Color) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
}

func animation() *gif.GIF {
	out := &gif.GIF{}
	for i := 0; i < 4; i++ {
		var buf bytes.Buffer
		frame := image.NewRGBA(image.Rect(0, 0, 120, 120))
		draw.Draw(frame, frame.Bounds(),
			&image.Uniform{color.RGBA{R: uint8(60 * i), G: 0x40, B: 0x80, A: 0xFF}}, image.Point{}, draw.Src)
		if err := gif.Encode(&buf, frame, nil); err != nil {
			continue
		}
		decoded, err := gif.Decode(&buf)
		if err != nil {
			continue
		}
		out.Image = append(out.Image, decoded.(*image.Paletted))
		out.Delay = append(out.Delay, 20)
	}
	return out
}
