// Command appicon renders the Lathe mark to build/appicon.png.
//
// The mark is defined once, in docs/brand/logo.svg, and repeated here in the
// same 64-unit coordinate space so the two cannot drift apart. Rendering it in
// Go rather than shelling out to a rasteriser keeps the toolchain to what
// go.mod already declares.
package main

import (
	"bytes"
	"encoding/binary"
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
)

var (
	metal  = color.NRGBA{0xed, 0xed, 0xed, 0xff}
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

// icoSizes are the sizes Windows picks between, from the taskbar up to the
// large view in Explorer. Each is rendered at its own resolution rather than
// scaled down from one, because these shapes are geometry, and rasterising
// them directly keeps the edges hard where a resample would blur them.
var icoSizes = []int{16, 24, 32, 48, 64, 128, 256}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	// 1024 is what Wails wants for macOS; Linux uses it directly.
	png1024 := filepath.Join(root, "build", "appicon.png")
	if err := writePNG(png1024, render(1024)); err != nil {
		return err
	}
	fmt.Println("wrote", png1024)

	// Wails only generates this from appicon.png when it is missing, so a
	// stale one silently keeps shipping on the executable.
	ico := filepath.Join(root, "build", "windows", "icon.ico")
	if err := writeICO(ico, icoSizes); err != nil {
		return err
	}
	fmt.Println("wrote", ico)
	return nil
}

func writePNG(path string, img *image.NRGBA) error {
	return fsatomic.WriteFile(path, func(w io.Writer) error {
		return png.Encode(w, img)
	}, 0)
}

// writeICO emits a Windows icon holding one PNG-compressed image per size.
// PNG inside ICO has been supported since Vista, and this app targets far
// newer than that.
func writeICO(path string, sizes []int) error {
	var frames [][]byte
	for _, s := range sizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, render(s)); err != nil {
			return fmt.Errorf("encode %dpx: %w", s, err)
		}
		frames = append(frames, buf.Bytes())
	}

	return fsatomic.WriteFile(path, func(w io.Writer) error {
		// ICONDIR: reserved, type 1 (icon), image count.
		if err := binary.Write(w, binary.LittleEndian,
			[3]uint16{0, 1, uint16(len(frames))}); err != nil {
			return err
		}

		// Directory entries are fixed width, so the first image starts after
		// all of them.
		offset := uint32(6 + 16*len(frames))
		for i, f := range frames {
			// 0 means 256 in a single byte.
			dim := byte(sizes[i])
			entry := struct {
				W, H, Colors, Reserved byte
				Planes, Bits           uint16
				Size, Offset           uint32
			}{dim, dim, 0, 0, 1, 32, uint32(len(f)), offset}
			if err := binary.Write(w, binary.LittleEndian, entry); err != nil {
				return err
			}
			offset += uint32(len(f))
		}

		for _, f := range frames {
			if _, err := w.Write(f); err != nil {
				return err
			}
		}
		return nil
	}, 0)
}

// The tile belongs to the lockup on a page, where the mark has to survive a
// light background. An application icon sits in a dock or a task bar beside
// other transparent icons, so it is the bare mark: no ground, no registration
// ticks, and centred on itself rather than in the tile.
const markShiftX = 2.5

func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := unit / float64(size)

	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			// Nothing underneath, so each element composites onto transparency
			// and a partly covered pixel keeps a partial alpha.
			var c color.NRGBA
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
			x := (float64(px)+(float64(sx)+0.5)/samples)*scale + markShiftX
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

// over composites src onto dst at the given coverage, in straight alpha, so a
// pixel on the edge of a shape ends up partly transparent rather than blended
// against a background that is not there.
func over(dst, src color.NRGBA, a float64) color.NRGBA {
	if a <= 0 {
		return dst
	}
	if a >= 1 {
		return src
	}

	da := float64(dst.A) / 255
	outA := a + da*(1-a)
	if outA <= 0 {
		return color.NRGBA{}
	}
	mix := func(d, s uint8) uint8 {
		v := (float64(s)*a + float64(d)*da*(1-a)) / outA
		return uint8(math.Round(v))
	}
	return color.NRGBA{
		mix(dst.R, src.R),
		mix(dst.G, src.G),
		mix(dst.B, src.B),
		uint8(math.Round(outA * 255)),
	}
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
