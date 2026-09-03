// Command appicon renders the Lathe mark to build/appicon.png.
//
// The mark is defined once, in docs/brand/logo.svg, and repeated here in the
// same 64-unit coordinate space so the two cannot drift apart. Rendering it in
// Go rather than shelling out to a rasteriser keeps the toolchain to what
// go.mod already declares.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/nabrahma/lathe/internal/fsatomic"
)

// The mark, in the SVG's 64-unit space. Changing anything here means changing
// docs/brand/logo.svg to match.
const (
	unit = 64.0

	cx, cy = 29.0, 32.0

	ring1R, ring1W = 22.0, 4.0
	ring2R, ring2W = 12.5, 3.4
	coreR          = 5.5

	// Half the arc the tool has cut out of the outer face, in degrees.
	gapHalfDeg = 13.0

	tickInset, tickLen, tickW = 3.0, 5.0, 1.5
)

var (
	ground = color.NRGBA{0x08, 0x08, 0x08, 0xff}
	metal  = color.NRGBA{0xed, 0xed, 0xed, 0xff}
	tick   = color.NRGBA{0x3d, 0x3d, 0x3d, 0xff}
	accent = color.NRGBA{0xff, 0xe5, 0x00, 0xff}
)

// samples is the supersampling factor per axis. Coverage is averaged over
// samples*samples points per pixel, which is what gives the curves their edge
// without a path rasteriser.
const samples = 4

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "appicon:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	// 1024 is what Wails wants for macOS; Windows and Linux downscale from it.
	const size = 1024
	img := render(size)

	out := filepath.Join(root, "build", "appicon.png")
	if err := fsatomic.WriteFile(out, func(w io.Writer) error {
		return png.Encode(w, img)
	}, 0); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := unit / float64(size)

	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// Start on the ground and lay each element over it, so a partly
			// covered pixel blends against what is actually beneath it.
			c := ground
			c = over(c, tick, coverage(px, py, scale, ticksAt))
			c = over(c, metal, coverage(px, py, scale, ringsAt))
			c = over(c, accent, coverage(px, py, scale, toolAt))
			img.SetNRGBA(px, py, c)
		}
	}
	return img
}

// coverage averages a shape test over a grid of sample points inside one pixel.
func coverage(px, py int, scale float64, inside func(x, y float64) bool) float64 {
	hit := 0
	for sy := 0; sy < samples; sy++ {
		for sx := 0; sx < samples; sx++ {
			x := (float64(px) + (float64(sx)+0.5)/samples) * scale
			y := (float64(py) + (float64(sy)+0.5)/samples) * scale
			if inside(x, y) {
				hit++
			}
		}
	}
	return float64(hit) / float64(samples*samples)
}

// ringsAt is the workpiece: the outer face broken where the tool cuts, the
// inner face whole, and the solid centre.
func ringsAt(x, y float64) bool {
	dx, dy := x-cx, y-cy
	d := math.Hypot(dx, dy)

	if d <= coreR {
		return true
	}
	if onRing(d, ring2R, ring2W) {
		return false
	}
	if onRing(d, ring1R, ring1W) {
		// The gap sits on the right, centred on the horizontal, which is where
		// the tool meets the work.
		deg := math.Abs(math.Atan2(dy, dx) * 180 / math.Pi)
		return deg > gapHalfDeg
	}
	return false
}

func onRing(d, r, w float64) bool {
	return math.Abs(d-r) <= w/2
}

// toolAt is the cutting tool: a straight shank running in from the right edge,
// ground to a point that sits inside the cut it has made.
func toolAt(x, y float64) bool {
	const tipX, tipY = 47.0, 32.0
	const shoulderX, halfH = 56.0, 4.5

	if x < tipX || x > unit {
		return false
	}
	half := halfH
	if x < shoulderX {
		half = halfH * (x - tipX) / (shoulderX - tipX)
	}
	return math.Abs(y-tipY) <= half
}

// ticksAt is the four corner registration marks.
func ticksAt(x, y float64) bool {
	for _, c := range [4][2]float64{
		{tickInset, tickInset},
		{unit - tickInset, tickInset},
		{unit - tickInset, unit - tickInset},
		{tickInset, unit - tickInset},
	} {
		if onTick(x, y, c[0], c[1]) {
			return true
		}
	}
	return false
}

// onTick draws the corner as two strokes meeting at a right angle, each
// running inward from the corner point.
func onTick(x, y, ox, oy float64) bool {
	towards := func(v, o float64) bool {
		if o < unit/2 {
			return v >= o && v <= o+tickLen
		}
		return v <= o && v >= o-tickLen
	}
	onArm := math.Abs(y-oy) <= tickW/2 && towards(x, ox)
	onLeg := math.Abs(x-ox) <= tickW/2 && towards(y, oy)
	return onArm || onLeg
}

// over composites src onto dst at the given coverage.
func over(dst, src color.NRGBA, a float64) color.NRGBA {
	if a <= 0 {
		return dst
	}
	if a >= 1 {
		return src
	}
	mix := func(d, s uint8) uint8 {
		return uint8(math.Round(float64(d)*(1-a) + float64(s)*a))
	}
	return color.NRGBA{mix(dst.R, src.R), mix(dst.G, src.G), mix(dst.B, src.B), 0xff}
}

// repoRoot walks up until it finds go.mod, so the script runs from anywhere.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
