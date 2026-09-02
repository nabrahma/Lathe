package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The OCR corpus is rendered from known text, so accuracy can be measured
// against real ground truth rather than described with an adjective. Each page
// is written twice: the image, and the exact text it contains.

// passages are ordinary sentences rather than pangrams, because OCR accuracy
// on natural language is what the benchmark is meant to predict.
var passages = []struct {
	name string
	text string
}{
	{"notice", `Public Notice

The office will remain closed on Monday 14 August
for the annual audit. Applications received after
5 pm on Friday will be processed the following week.

Please bring one form of photo identification and
a copy of this notice when you attend.`},

	{"receipt", `Receipt No. 4471

Date: 12 March 2026
Paid by: Priya Sharma
Amount: 2,450.00

Tuition fee for the spring term, paid in full.
This receipt is valid without a signature.`},

	{"letter", `Dear Sir or Madam,

I am writing to request a duplicate copy of my
transcript for the academic year 2024 to 2025.
My enrolment number is 21BCS4419.

I have attached a copy of my identity card and
the prescribed fee receipt for your reference.

Yours faithfully,
Anil Kumar`},

	{"instructions", `How to submit the form

1. Fill in every field marked with an asterisk.
2. Attach a recent photograph in the box provided.
3. Sign and date the declaration at the bottom.
4. Submit the completed form at counter number 3.

Incomplete forms will be returned without notice.`},
}

func writeOCRCorpus() error {
	dir := filepath.Join(root, "ocr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	face, err := loadFace(34)
	if err != nil {
		return err
	}

	for _, p := range passages {
		page := renderText(p.text, face)

		// A clean scan: what a flatbed produces.
		if err := writePNG(filepath.Join(dir, p.name+"-clean.png"), page); err != nil {
			return err
		}
		// A phone photo: skewed, unevenly lit, noisy, slightly soft.
		if err := writeJPEG(filepath.Join(dir, p.name+"-photo.jpg"), photograph(page, p.name), 78); err != nil {
			return err
		}
		// A low-resolution capture, which is where accuracy genuinely suffers.
		small := imaging.Resize(page, 0, page.Bounds().Dy()/3, imaging.Lanczos)
		if err := writeJPEG(filepath.Join(dir, p.name+"-lowres.jpg"), small, 60); err != nil {
			return err
		}

		// Ground truth, shared by all three renderings of this passage.
		if err := os.WriteFile(filepath.Join(dir, p.name+".txt"), []byte(p.text+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func loadFace(size float64) (font.Face, error) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: size, DPI: 96, Hinting: font.HintingFull,
	})
}

// renderText lays the passage out on a page with generous margins, the way a
// printed document is set.
func renderText(text string, face font.Face) image.Image {
	const (
		width  = 1240 // roughly A4 at 150 DPI
		margin = 90
		lead   = 52
	)

	lines := strings.Split(text, "\n")
	height := margin*2 + lead*len(lines)
	if height < 600 {
		height = 600
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{color.RGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xFF}},
		Face: face,
	}
	for i, line := range lines {
		drawer.Dot = fixed.P(margin, margin+lead*(i+1))
		drawer.DrawString(line)
	}
	return img
}

// photograph simulates a hand-held capture: a few degrees of skew, a lighting
// gradient across the page, sensor noise and a little softness. This is the
// input the preprocessing chain exists for.
func photograph(src image.Image, seed string) image.Image {
	rnd := rand.New(rand.NewSource(int64(len(seed)) * 7717))

	// Skew by a small angle, in the range a hand-held photo actually lands in.
	angle := 2.5 + rnd.Float64()*2
	if rnd.Intn(2) == 0 {
		angle = -angle
	}
	rotated := imaging.Rotate(src, angle, color.White)

	b := rotated.Bounds()
	out := image.NewRGBA(b)
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	maxDist := math.Hypot(cx, cy)

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := rotated.At(x, y).RGBA()

			// Vignette: brighter near the middle, as a phone flash or a window
			// lights a page.
			dist := math.Hypot(float64(x)-cx, float64(y)-cy) / maxDist
			light := 1.05 - 0.35*dist*dist

			noise := float64(rnd.Intn(19) - 9)
			out.Set(x, y, color.RGBA{
				R: clamp(float64(r>>8)*light + noise),
				G: clamp(float64(g>>8)*light + noise),
				B: clamp(float64(bl>>8)*light + noise),
				A: 255,
			})
		}
	}
	return imaging.Blur(out, 0.6)
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

func ocrManifest() string {
	var b strings.Builder
	b.WriteString("\nocr:\n")
	b.WriteString("  # Rendered from known text, so OCR accuracy is measured against real\n")
	b.WriteString("  # ground truth. <name>.txt is the exact text in all three renderings.\n")
	for _, p := range passages {
		b.WriteString(fmt.Sprintf("  %s-clean.png:   a clean scan of the %s passage\n", p.name, p.name))
		b.WriteString(fmt.Sprintf("  %s-photo.jpg:   the same page skewed, unevenly lit and noisy\n", p.name))
		b.WriteString(fmt.Sprintf("  %s-lowres.jpg:  the same page at a third of the resolution\n", p.name))
		b.WriteString(fmt.Sprintf("  %s.txt:         ground truth for the three above\n", p.name))
	}
	return b.String()
}
