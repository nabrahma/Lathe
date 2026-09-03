// Command demofiles writes the sample documents used for the README
// screenshots.
//
// They are generated rather than committed so the repository carries no
// stray binaries, and generated to resemble what people actually convert: a
// phone photo saved at the quality a camera app uses, a multi-page form, and a
// scan. A screenshot taken against unrealistic input flatters the product,
// which is the opposite of useful.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func main() {
	dir := flag.String("dir", defaultDir(), "where to write the demo files")
	flag.Parse()

	api.DisableConfigDir()

	if err := run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "demofiles:", err)
		os.Exit(1)
	}
	fmt.Println("demo files written to", *dir)
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "lathe-demo"
	}
	return filepath.Join(home, "Documents", "Lathe Demo")
}

func run(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// A phone photo: large, detailed, and saved at the quality a camera app
	// uses. That last part is the whole point, because it is why "compress
	// this so the form will accept it" is a task people search for.
	if err := writeJPEG(filepath.Join(dir, "IMG_20260821_144233.jpg"), photo(4000, 3000), 95); err != nil {
		return err
	}

	if err := writePNG(filepath.Join(dir, "Passport Photo.png"), portrait(413, 531)); err != nil {
		return err
	}

	// A form and a scan, as PDFs of page images.
	if err := writePDF(filepath.Join(dir, "Scholarship Application 2026.pdf"), 12, false); err != nil {
		return err
	}
	if err := writePDF(filepath.Join(dir, "Enrolment Form.pdf"), 4, false); err != nil {
		return err
	}
	return writePDF(filepath.Join(dir, "Fee Receipt Scan.pdf"), 2, true)
}

// photo builds an image with the spatial detail a camera sensor produces:
// smooth large-scale structure, texture at several scales, and per-pixel
// noise. A flat gradient compresses nothing like a real photograph, so using
// one would make the compression figures meaningless.
func photo(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(20260821))

	// A few soft blobs standing in for subjects at different depths.
	type blob struct {
		x, y, r float64
		c       [3]float64
	}
	blobs := make([]blob, 14)
	for i := range blobs {
		blobs[i] = blob{
			x: rnd.Float64() * float64(w),
			y: rnd.Float64() * float64(h),
			r: 200 + rnd.Float64()*900,
			c: [3]float64{rnd.Float64(), rnd.Float64(), rnd.Float64()},
		}
	}

	for y := 0; y < h; y++ {
		fy := float64(y)
		for x := 0; x < w; x++ {
			fx := float64(x)

			// Sky-to-ground gradient.
			t := fy / float64(h)
			r := 120 + 90*(1-t)
			g := 140 + 70*(1-t)
			b := 180 - 60*t

			// Subjects.
			for _, bl := range blobs {
				d := math.Hypot(fx-bl.x, fy-bl.y)
				if d < bl.r {
					falloff := 1 - d/bl.r
					weight := falloff * falloff * 90
					r += bl.c[0] * weight
					g += bl.c[1] * weight
					b += bl.c[2] * weight
				}
			}

			// Texture at two scales, which is what defeats naive compression.
			fine := math.Sin(fx*0.35) * math.Cos(fy*0.29) * 14
			coarse := math.Sin(fx*0.011) * math.Sin(fy*0.013) * 22
			noise := float64(rnd.Intn(23) - 11)

			img.Set(x, y, color.RGBA{
				R: clamp(r + fine + coarse + noise),
				G: clamp(g + fine*0.8 + coarse + noise),
				B: clamp(b + fine*0.6 + coarse*1.2 + noise),
				A: 255,
			})
		}
	}
	return img
}

// portrait is a plain head-and-shoulders placeholder on a flat background, the
// shape a passport photo actually is.
func portrait(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 0xE8, G: 0xEC, B: 0xF0, A: 0xFF}}, image.Point{}, draw.Src)

	skin := color.RGBA{R: 0xC8, G: 0xA2, B: 0x82, A: 0xFF}
	shirt := color.RGBA{R: 0x35, G: 0x40, B: 0x55, A: 0xFF}

	cx := float64(w) / 2
	headY, headR := float64(h)*0.38, float64(w)*0.26

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x), float64(y)
			if math.Hypot(fx-cx, fy-headY) < headR {
				img.Set(x, y, skin)
				continue
			}
			// Shoulders as a wide ellipse rising from the bottom edge.
			sy := (fy - float64(h)*1.05) / (float64(h) * 0.42)
			sx := (fx - cx) / (float64(w) * 0.52)
			if sx*sx+sy*sy < 1 {
				img.Set(x, y, shirt)
			}
		}
	}
	return img
}

// writePDF assembles a document from rendered page images. scan produces the
// off-white, speckled look of something that went through a flatbed.
func writePDF(path string, pages int, scan bool) error {
	tmp, err := os.MkdirTemp("", "lathe-demo-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	var images []string
	for i := 1; i <= pages; i++ {
		p := filepath.Join(tmp, fmt.Sprintf("page-%03d.jpg", i))
		// Scanners write at a quality far above what text needs, which is
		// exactly why "compress this PDF" has anything to work with.
		if err := writeJPEG(p, page(i, scan), 95); err != nil {
			return err
		}
		images = append(images, p)
	}

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	return api.ImportImagesFile(images, path, api.DefaultImportConfig(), conf)
}

func page(index int, scan bool) image.Image {
	const w, h = 1240, 1754 // A4 at 150 DPI
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(int64(index) * 7919))

	paper := color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	ink := color.RGBA{R: 0x22, G: 0x22, B: 0x26, A: 0xFF}
	if scan {
		ink = color.RGBA{R: 0x3A, G: 0x3A, B: 0x3C, A: 0xFF}
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{paper}, image.Point{}, draw.Src)

	if scan {
		// Paper grain and a slight warm cast, which is what a scanner adds.
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := uint8(240 + rnd.Intn(16))
				img.Set(x, y, color.RGBA{R: v, G: v, B: uint8(int(v) - 5), A: 255})
			}
		}
	}

	// A heading, then ruled lines of varying length standing in for text.
	fill(img, 110, 130, 520, 40, ink)
	fill(img, 110, 196, 300, 10, ink)

	y := 280
	for block := 0; block < 5; block++ {
		lines := 4 + rnd.Intn(4)
		for i := 0; i < lines; i++ {
			fill(img, 110, y, 640+rnd.Intn(420), 12, ink)
			y += 36
		}
		y += 44
		if y > h-260 {
			break
		}
	}
	// A footer block whose position encodes the page, so a reordered document
	// is visibly different from an untouched one.
	fill(img, 110+((index-1)%5)*90, h-140, 60, 18, ink)
	return img
}

func fill(img draw.Image, x, y, w, h int, c color.Color) {
	draw.Draw(img, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
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

func clamp(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return uint8(v)
	}
}
