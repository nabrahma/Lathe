package ocrengine

import (
	"image"
	"image/color"
	"math"
)

// Binarisation is the step that decides, for every pixel, whether it is ink or
// paper. It is the last thing to touch the image before Tesseract sees it, and
// on a photographed page it is the step that decides whether the text survives.

// binarize is the chain's chosen method. Sauvola ties Otsu on an evenly lit
// page and is far ahead of it on a shadowed one, measured in
// TestBinariserChoice.
func binarize(img image.Image) image.Image { return sauvola(img) }

// threshold applies Otsu's method: it finds the intensity that best separates
// ink from paper, which handles the uneven lighting of a photographed page far
// better than a fixed cutoff.
func threshold(img image.Image) image.Image {
	b := img.Bounds()

	var histogram [256]int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			histogram[r>>8]++
		}
	}

	total := b.Dx() * b.Dy()
	if total == 0 {
		return img
	}

	sum := 0.0
	for i, count := range histogram {
		sum += float64(i * count)
	}

	var (
		sumBackground float64
		weightBack    int
		bestVariance  float64
		cut           = 128
	)
	for t := 0; t < 256; t++ {
		weightBack += histogram[t]
		if weightBack == 0 {
			continue
		}
		weightFore := total - weightBack
		if weightFore == 0 {
			break
		}

		sumBackground += float64(t * histogram[t])
		meanBack := sumBackground / float64(weightBack)
		meanFore := (sum - sumBackground) / float64(weightFore)

		diff := meanBack - meanFore
		between := float64(weightBack) * float64(weightFore) * diff * diff
		if between > bestVariance {
			bestVariance, cut = between, t
		}
	}

	out := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if int(r>>8) > cut {
				out.SetGray(x, y, color.Gray{Y: 255})
			} else {
				out.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}
	return out
}

// Sauvola's method computes a separate threshold for every pixel from the mean
// and standard deviation of its neighbourhood, rather than one cutoff for the
// whole page.
//
// This is what a global method cannot do. Otsu picks a single intensity, so a
// page with a shadow across it forces one choice to serve both halves: light
// enough to keep the shadowed paper white, and the faint strokes in the bright
// half wash out; dark enough to catch those strokes, and the shadow turns into
// a solid block of ink. Judging each pixel against its own surroundings makes
// the gradient irrelevant, because the paper beside a stroke is in the same
// shadow as the stroke.
//
// The term (s/R - 1) is the part that matters on a blank margin: where there is
// no ink the deviation is near zero, the term goes to -1, and the threshold
// drops well below the local mean, so paper grain is not amplified into
// speckle the way a plain local mean would.
const (
	// sauvolaK controls how far below the local mean the threshold sits. The
	// 0.5 of the original paper was tuned for historical manuscripts and eats
	// thin strokes on clean print; 0.25 is the value in common use since.
	//
	// Sweeping it from 0.15 to 0.34 moved the corpus by a third of a point,
	// which four passages cannot resolve, so this stays at the common value
	// rather than being tuned to the noise.
	sauvolaK = 0.25

	// sauvolaR is the assumed dynamic range of the deviation, which for 8-bit
	// grayscale is half of full scale.
	sauvolaR = 128.0

	// The window has to be wide enough to hold both ink and the paper around
	// it. Too small and the middle of a thick stroke looks like its own local
	// background and hollows out; too large and it degenerates towards a global
	// threshold. A fraction of the page height tracks the text size, since the
	// chain has already scaled the page to a known height by this point.
	sauvolaWindowDivisor = 24
	sauvolaMinWindow     = 15
	sauvolaMaxWindow     = 151
)

func sauvola(img image.Image) image.Image {
	return sauvolaWith(img, sauvolaK, sauvolaWindowDivisor)
}

func sauvolaWith(img image.Image, k float64, divisor int) image.Image {
	src := toGray(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return img
	}

	sum, sqSum := integrals(src)
	radius := sauvolaWindow(h, divisor) / 2

	out := image.NewGray(b)
	for y := 0; y < h; y++ {
		y0, y1 := clampInt(y-radius, 0, h-1), clampInt(y+radius, 0, h-1)
		for x := 0; x < w; x++ {
			x0, x1 := clampInt(x-radius, 0, w-1), clampInt(x+radius, 0, w-1)

			n := float64((x1 - x0 + 1) * (y1 - y0 + 1))
			total := boxSum(sum, w, x0, y0, x1, y1)
			totalSq := boxSum(sqSum, w, x0, y0, x1, y1)

			mean := total / n
			// Clamped because the subtraction is exact in theory and slightly
			// negative in floating point on a perfectly flat patch.
			variance := math.Max(0, totalSq/n-mean*mean)
			t := mean * (1 + k*(math.Sqrt(variance)/sauvolaR-1))

			if float64(src.GrayAt(b.Min.X+x, b.Min.Y+y).Y) > t {
				out.SetGray(b.Min.X+x, b.Min.Y+y, color.Gray{Y: 255})
			} else {
				out.SetGray(b.Min.X+x, b.Min.Y+y, color.Gray{Y: 0})
			}
		}
	}
	return out
}

// sauvolaWindow picks an odd window size from the page height.
func sauvolaWindow(height, divisor int) int {
	win := height / divisor
	win = clampInt(win, sauvolaMinWindow, sauvolaMaxWindow)
	if win%2 == 0 {
		win++
	}
	return win
}

// integrals builds the summed-area tables for the values and their squares, so
// the mean and deviation of any window cost four lookups each rather than one
// pass per pixel. Without this the window size would drive the running time and
// a page would take minutes.
//
// Both tables have a zero first row and column, which removes the bounds check
// from every lookup in boxSum.
func integrals(src *image.Gray) (sum, sqSum []float64) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	stride := w + 1

	sum = make([]float64, stride*(h+1))
	sqSum = make([]float64, stride*(h+1))

	for y := 0; y < h; y++ {
		var rowSum, rowSqSum float64
		for x := 0; x < w; x++ {
			v := float64(src.GrayAt(b.Min.X+x, b.Min.Y+y).Y)
			rowSum += v
			rowSqSum += v * v

			i := (y+1)*stride + x + 1
			sum[i] = sum[i-stride] + rowSum
			sqSum[i] = sqSum[i-stride] + rowSqSum
		}
	}
	return sum, sqSum
}

// boxSum totals an inclusive rectangle from a summed-area table.
func boxSum(table []float64, w, x0, y0, x1, y1 int) float64 {
	stride := w + 1
	return table[(y1+1)*stride+x1+1] -
		table[y0*stride+x1+1] -
		table[(y1+1)*stride+x0] +
		table[y0*stride+x0]
}

// toGray avoids a per-pixel interface call in the hot loops above, which on a
// 4400 pixel high page is the difference between a moment and a wait.
func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	b := img.Bounds()
	out := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Illumination flattening removes the lighting itself rather than coping with
// it. The page is divided into a coarse grid, the brightness of the paper in
// each cell is estimated, that estimate is interpolated back to full size, and
// the page is divided by it. What comes out is the page as a flatbed would have
// seen it, with the shadow gone and the ink untouched.
//
// It is worth doing even though the binariser below is already local, because
// the two solve different halves of the problem: flattening restores contrast
// that a shadow has compressed, and no thresholding rule can recover detail
// that is no longer in the pixels.
const (
	// flattenCells is the grid across the page's long edge. Coarse on purpose:
	// the estimate has to follow the lighting, which varies slowly, without
	// following the text, which does not.
	flattenCells = 12

	// flattenPercentile is the point in a cell's brightness distribution taken
	// as paper. The mean would be dragged down by the ink; the maximum would
	// catch a specular highlight. The bright end, short of the extreme, is the
	// paper.
	flattenPercentile = 0.85

	// minBackground stops a cell that is entirely ink, such as a solid rule or
	// a photograph, from dividing the page by nearly zero.
	minBackground = 24.0
)

func flattenIllumination(img image.Image) *image.Gray {
	src := toGray(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return src
	}

	cols, rows := gridFor(w, h)
	background := make([]float64, cols*rows)
	values := make([]uint8, 0, (w/cols+1)*(h/rows+1))

	for gy := 0; gy < rows; gy++ {
		for gx := 0; gx < cols; gx++ {
			x0, x1 := w*gx/cols, w*(gx+1)/cols
			y0, y1 := h*gy/rows, h*(gy+1)/rows

			values = values[:0]
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					values = append(values, src.GrayAt(b.Min.X+x, b.Min.Y+y).Y)
				}
			}
			background[gy*cols+gx] = math.Max(minBackground, percentile(values, flattenPercentile))
		}
	}

	out := image.NewGray(b)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			bg := sampleGrid(background, cols, rows, w, h, x, y)
			v := float64(src.GrayAt(b.Min.X+x, b.Min.Y+y).Y) / bg * 255
			out.SetGray(b.Min.X+x, b.Min.Y+y, color.Gray{Y: clamp8(v)})
		}
	}
	return out
}

// gridFor keeps the cells roughly square, so a landscape page is not divided
// into tall strips that smear the estimate vertically.
func gridFor(w, h int) (cols, rows int) {
	if w >= h {
		cols = flattenCells
		rows = clampInt(h*flattenCells/w, 2, flattenCells)
		return cols, rows
	}
	rows = flattenCells
	cols = clampInt(w*flattenCells/h, 2, flattenCells)
	return cols, rows
}

// sampleGrid reads the background estimate at full resolution, interpolating
// bilinearly between cell centres so the correction cannot leave visible seams
// at the cell edges.
func sampleGrid(grid []float64, cols, rows, w, h, x, y int) float64 {
	// Position in cell-centre space, where cell i is centred at i+0.5.
	fx := float64(x)/float64(w)*float64(cols) - 0.5
	fy := float64(y)/float64(h)*float64(rows) - 0.5

	x0 := int(math.Floor(fx))
	y0 := int(math.Floor(fy))
	tx := fx - float64(x0)
	ty := fy - float64(y0)

	at := func(cx, cy int) float64 {
		return grid[clampInt(cy, 0, rows-1)*cols+clampInt(cx, 0, cols-1)]
	}
	top := at(x0, y0)*(1-tx) + at(x0+1, y0)*tx
	bottom := at(x0, y0+1)*(1-tx) + at(x0+1, y0+1)*tx
	return top*(1-ty) + bottom*ty
}

// percentile returns the value at p through the sorted order, found by counting
// rather than sorting: the values are bytes, so a 256-bin histogram gets there
// in one pass instead of n log n.
func percentile(values []uint8, p float64) float64 {
	if len(values) == 0 {
		return 255
	}

	var histogram [256]int
	for _, v := range values {
		histogram[v]++
	}

	target := int(p * float64(len(values)))
	seen := 0
	for v, count := range histogram {
		seen += count
		if seen > target {
			return float64(v)
		}
	}
	return 255
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
