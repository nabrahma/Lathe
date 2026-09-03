package ocrengine

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The preprocessing chain has two steps in it that were chosen rather than
// inherited: flattening the lighting, and binarising locally. Both were settled
// by measurement, and this is that measurement, kept so that undoing either one
// fails rather than quietly costing accuracy nobody notices.
//
// The conditions matter as much as the methods. A page under a smooth gradient
// is solved by flattening alone, so it cannot tell the binarisers apart; a page
// under a hard-edged shadow, which is what the edge of a hand or the gutter of
// an open book casts, is the case that separates them. Both are measured.

// corpusPassages are the corpus pages whose ground truth is committed beside
// them.
var corpusPassages = []string{"notice", "receipt", "letter", "instructions"}

func TestBinariserChoice(t *testing.T) {
	bin := findTesseract(t)

	for _, cond := range []struct {
		name    string
		degrade func(image.Image) image.Image
		floor   float64
	}{
		{name: "evenly lit", degrade: unchanged, floor: 0.99},
		{name: "lighting gradient", degrade: rampShadow(0.12), floor: 0.97},
		{name: "hard-edged shadow", degrade: hardShadow(0.15), floor: 0.97},
	} {
		t.Run(cond.name, func(t *testing.T) {
			got := scoreChain(t, bin, cond.degrade, binarize)
			t.Logf("%s: %.2f%%", cond.name, got*100)
			if got < cond.floor {
				t.Errorf("read %.2f%% of a page under %s, below the %.0f%% floor",
					got*100, cond.name, cond.floor*100)
			}
		})
	}
}

// TestLocalBinarisationBeatsGlobal is the evidence for Sauvola over Otsu.
//
// Flattening alone does not make the choice moot: it estimates the lighting on
// a coarse grid, which follows a gradient well and cannot follow a step at all.
// Under a hard-edged shadow a single cutoff still has to serve both sides of
// the edge at once, and cannot.
func TestLocalBinarisationBeatsGlobal(t *testing.T) {
	bin := findTesseract(t)

	global := scoreChain(t, bin, hardShadow(0.15), threshold)
	local := scoreChain(t, bin, hardShadow(0.15), sauvola)
	t.Logf("hard-edged shadow: otsu %.2f%%, sauvola %.2f%%", global*100, local*100)

	// A wide margin, because the measured gap is wide: 68% against 99%. This
	// asserts that the two are not interchangeable, not any particular figure.
	if local < global+0.10 {
		t.Errorf("sauvola %.2f%% did not clear otsu %.2f%% by ten points, "+
			"which is the whole reason for preferring it", local*100, global*100)
	}
}

// TestFlatteningComesBeforeGeometry guards the ordering inside prepareGray.
//
// Deskew and the border trim both read the page as a whole, and under a shadow
// they answer a question about the lighting rather than about the page: the
// trim crops shaded paper as though it were the dark edge of a desk. Putting
// flattening first costs nothing and was worth fifteen points.
func TestFlatteningComesBeforeGeometry(t *testing.T) {
	// Blank paper, lit far more brightly on one side than the other.
	src := image.NewGray(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			src.SetGray(x, y, color.Gray{Y: uint8(40 + x*200/400)})
		}
	}

	out := toGray(prepareGray(src))
	b := out.Bounds()
	shaded := out.GrayAt(b.Min.X+b.Dx()/10, b.Min.Y+b.Dy()/2).Y
	lit := out.GrayAt(b.Max.X-b.Dx()/10, b.Min.Y+b.Dy()/2).Y

	// Both sides are paper, so both should still be paper afterwards.
	if diff := int(lit) - int(shaded); diff < -60 || diff > 60 {
		t.Errorf("blank paper came out %d on the shaded side and %d on the lit side, "+
			"so the lighting survived preprocessing", shaded, lit)
	}
}

// scoreChain runs every corpus passage through prepareGray, binarises it with
// the given method, and returns the mean accuracy Tesseract reads back.
func scoreChain(t *testing.T, bin string, degrade, method func(image.Image) image.Image) float64 {
	t.Helper()

	dir := filepath.Join("..", "..", "..", "testdata", "corpus", "ocr")
	total := 0.0

	for _, name := range corpusPassages {
		src := loadImage(t, filepath.Join(dir, name+"-clean.png"))
		truth, err := os.ReadFile(filepath.Join(dir, name+".txt"))
		if err != nil {
			t.Skipf("corpus missing (run: make corpus): %v", err)
		}
		total += accuracyOf(t, bin, method(prepareGray(degrade(src))), string(truth))
	}
	return total / float64(len(corpusPassages))
}

func unchanged(img image.Image) image.Image { return img }

// rampShadow darkens the page smoothly towards one edge, as a window does.
func rampShadow(floor float64) func(image.Image) image.Image {
	return func(src image.Image) image.Image {
		return lightMap(src, func(u float64) float64 { return 1 - (1-floor)*u })
	}
}

// hardShadow drops the light abruptly partway across, as the edge of a hand or
// the gutter of an open book does. A grid estimate cannot follow a step, which
// is what makes this the case that separates the binarisers.
func hardShadow(floor float64) func(image.Image) image.Image {
	return func(src image.Image) image.Image {
		return lightMap(src, func(u float64) float64 {
			if u < 0.45 {
				return 1
			}
			return floor
		})
	}
}

func lightMap(src image.Image, light func(u float64) float64) image.Image {
	b := src.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			l := light(float64(x-b.Min.X) / float64(b.Dx()))
			r, g, bl, _ := src.At(x, y).RGBA()
			out.Set(x, y, color.RGBA{
				R: scaleChannel(r, l), G: scaleChannel(g, l), B: scaleChannel(bl, l), A: 255,
			})
		}
	}
	return out
}

func scaleChannel(v uint32, light float64) uint8 {
	return clamp8(float64(v>>8) * light)
}

// accuracyOf writes the bilevel page out and reads it back with Tesseract.
func accuracyOf(t *testing.T, bin string, img image.Image, truth string) float64 {
	t.Helper()

	path := filepath.Join(t.TempDir(), "page.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	cmd := exec.CommandContext(context.Background(), bin, path, "stdout", "-l", "eng", "--psm", "3")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		// An unreadable page scores zero rather than failing the run: that is
		// precisely the outcome under measurement.
		return 0
	}
	return charAccuracy(truth, stdout.String())
}

func findTesseract(t *testing.T) string {
	t.Helper()

	name := "tesseract"
	if runtime.GOOS == "windows" {
		name = "tesseract.exe"
	}
	bin, err := exec.LookPath(name)
	if err != nil {
		t.Skip("Tesseract is not installed on this machine; see docs/BUNDLING.md")
	}
	return bin
}

func loadImage(t *testing.T, path string) image.Image {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return img
}

// charAccuracy is 1 minus the normalised edit distance, the standard OCR
// measure. Whitespace is collapsed first because line wrapping is a layout
// artefact rather than a recognition error.
func charAccuracy(want, got string) float64 {
	a := []rune(normaliseText(want))
	b := []rune(normaliseText(got))
	if len(a) == 0 {
		return 0
	}
	if accuracy := 1 - float64(editDistance(a, b))/float64(len(a)); accuracy > 0 {
		return accuracy
	}
	return 0
}

func normaliseText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func editDistance(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = int(math.Min(math.Min(
				float64(current[j-1]+1), float64(previous[j]+1)),
				float64(previous[j-1]+cost)))
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}
