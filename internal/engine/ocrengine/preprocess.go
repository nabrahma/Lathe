package ocrengine

import (
	"image"
	"image/color"
	"math"

	"github.com/disintegration/imaging"
)

// Preprocessing is what separates usable OCR from garbage. Tesseract on a raw
// phone photo performs poorly; on a deskewed, thresholded, adequately
// resolved image it performs well. The chain below is applied in this order:
// orientation, grayscale, deskew, denoise, upscale, threshold.

// targetHeight is roughly a 300 DPI A4 page. Tesseract's accuracy falls off
// sharply below that, and upscaling a small image genuinely helps.
const targetHeight = 3300

// maxHeight stops a 50 MP photo from being upscaled into gigabytes.
const maxHeight = 4400

// enhance runs the full preprocessing chain and hands back a bilevel image.
func enhance(src image.Image) image.Image {
	return binarize(prepareGray(src))
}

// prepareGray runs everything up to the point of binarisation. It is separate
// so the binariser can be swapped and measured against the same input, which
// is how the choice between the two in binarize.go was settled.
func prepareGray(src image.Image) image.Image {
	// Flattening comes first, before anything else reads the page as a whole.
	// Deskew judges a page by how its row darkness varies and trimBorder crops
	// rows that are mostly dark, so under a shadow both are answering a question
	// about the lighting rather than about the page: the trim quietly eats the
	// shaded margin as though it were the dark edge of a desk. Measured on the
	// corpus under a lighting gradient, flattening after these steps reads at
	// 84.7% and flattening before them at 100%.
	var gray image.Image = flattenIllumination(imaging.Grayscale(src))

	if angle := detectSkew(gray); math.Abs(angle) > 0.25 {
		// Rotating on white keeps the corners the same colour as the paper.
		gray = imaging.Grayscale(imaging.Rotate(gray, -angle, color.White))
	}

	gray = trimBorder(gray)

	// A gentle blur removes camera speckle without eating thin strokes.
	gray = imaging.Blur(gray, 0.4)

	if h := gray.Bounds().Dy(); h < targetHeight {
		scale := float64(targetHeight) / float64(h)
		if scale > 3 {
			scale = 3
		}
		newH := int(float64(h) * scale)
		if newH > maxHeight {
			newH = maxHeight
		}
		gray = imaging.Resize(gray, 0, newH, imaging.Lanczos)
	}

	return gray
}

// detectSkew estimates the text baseline angle by finding the rotation whose
// row-darkness profile has the sharpest peaks. Text lines align into strong
// horizontal bands only when the page is straight, so variance of the row sums
// is maximised at the correct angle.
//
// A page photographed at four degrees loses meaningful accuracy uncorrected,
// and four degrees is what a hand-held photo looks like.
func detectSkew(img image.Image) float64 {
	// Work small: the angle is a global property and a thumbnail finds it just
	// as well for a fraction of the cost.
	small := imaging.Resize(imaging.Grayscale(img), 600, 0, imaging.Box)

	best, bestScore := 0.0, -1.0
	for _, coarse := range steps(-6, 6, 1) {
		if s := sharpness(small, coarse); s > bestScore {
			best, bestScore = coarse, s
		}
	}
	// Refine around the coarse winner rather than scanning finely everywhere.
	for _, fine := range steps(best-0.9, best+0.9, 0.3) {
		if s := sharpness(small, fine); s > bestScore {
			best, bestScore = fine, s
		}
	}
	return best
}

func steps(from, to, by float64) []float64 {
	var out []float64
	for v := from; v <= to+1e-9; v += by {
		out = append(out, v)
	}
	return out
}

// sharpness is the variance of per-row darkness after rotating by angle.
func sharpness(img image.Image, angle float64) float64 {
	rotated := img
	if angle != 0 {
		rotated = imaging.Rotate(img, -angle, color.White)
	}

	b := rotated.Bounds()
	rows := make([]float64, 0, b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		sum := 0.0
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := rotated.At(x, y).RGBA()
			sum += float64(0xFFFF-r) / 0xFFFF
		}
		rows = append(rows, sum)
	}
	if len(rows) < 2 {
		return 0
	}

	mean := 0.0
	for _, v := range rows {
		mean += v
	}
	mean /= float64(len(rows))

	variance := 0.0
	for _, v := range rows {
		variance += (v - mean) * (v - mean)
	}
	return variance / float64(len(rows))
}

// trimBorder crops the dark margin left by photographing a page on a desk,
// which Tesseract otherwise tries to read as content.
func trimBorder(img image.Image) image.Image {
	b := img.Bounds()
	if b.Dx() < 64 || b.Dy() < 64 {
		return img
	}

	// A border row is one that is almost entirely dark.
	const darkFraction = 0.9
	isDarkRow := func(y int) bool {
		dark := 0
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 80 {
				dark++
			}
		}
		return float64(dark)/float64(b.Dx()) > darkFraction
	}
	isDarkCol := func(x int) bool {
		dark := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r>>8 < 80 {
				dark++
			}
		}
		return float64(dark)/float64(b.Dy()) > darkFraction
	}

	// Only ever trim a thin frame; anything more would risk eating content.
	limitY := b.Dy() / 10
	limitX := b.Dx() / 10

	top, bottom := b.Min.Y, b.Max.Y-1
	for top < b.Min.Y+limitY && isDarkRow(top) {
		top++
	}
	for bottom > b.Max.Y-1-limitY && isDarkRow(bottom) {
		bottom--
	}

	left, right := b.Min.X, b.Max.X-1
	for left < b.Min.X+limitX && isDarkCol(left) {
		left++
	}
	for right > b.Max.X-1-limitX && isDarkCol(right) {
		right--
	}

	if left >= right || top >= bottom {
		return img
	}
	return imaging.Crop(img, image.Rect(left, top, right+1, bottom+1))
}
