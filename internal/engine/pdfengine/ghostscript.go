package pdfengine

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	lexec "github.com/nabrahma/lathe/internal/exec"
)

// componentID is the optional component that makes compression go further.
const componentID = "ghostscript"

// gsTimeout is generous: a few hundred scanned pages is a normal job and the
// user can cancel, so a short limit would only punish large documents.
const gsTimeout = 30 * time.Minute

// gsProfile is the resolution and quality one of the three settings maps to.
//
// The resolution is the point of using Ghostscript at all. pdfcpu can only
// re-encode an image at its existing pixel dimensions, so a 600 dpi phone
// photo of a receipt stays 600 dpi however hard it is squeezed. Bringing that
// down to something a screen or a printer can actually use is most of the
// saving on a scan.
type gsProfile struct {
	// preset is one of Ghostscript's own bundles. It carries the quantisation
	// factor, which is the part that actually varies the quality: -dJPEGQ is
	// accepted and then ignored by the pdfwrite device, so setting it alone
	// produces the same bytes for every setting.
	preset   string
	colorDPI int
	monoDPI  int
}

func gsProfileFor(quality string) gsProfile {
	switch quality {
	case "low":
		return gsProfile{preset: "/screen", colorDPI: 110, monoDPI: 300}
	case "high":
		return gsProfile{preset: "/printer", colorDPI: 220, monoDPI: 600}
	default:
		return gsProfile{preset: "/ebook", colorDPI: 150, monoDPI: 400}
	}
}

// ghostscriptAvailable reports whether the optional component is installed and
// runs. A nil manager means the engine was built without one, which is the
// case in the tests and the headless CLI.
func (e *Engine) ghostscriptAvailable() bool {
	return e.deps != nil && e.runner != nil && e.deps.Available(componentID)
}

// compressWithGhostscript rewrites in to out, downsampling images as well as
// re-encoding them.
//
// Every failure here is a fallback rather than an error: the built-in path
// still produces a good file, so a missing or unhappy Ghostscript should never
// turn into a message for the user.
func (e *Engine) compressWithGhostscript(
	ctx context.Context, in, out, password, quality string,
) bool {
	bin, err := e.deps.BinaryPath(componentID, "gs")
	if err != nil {
		return false
	}

	p := gsProfileFor(quality)
	args := []string{
		"-sDEVICE=pdfwrite",
		"-dCompatibilityLevel=1.7",
		"-dNOPAUSE", "-dBATCH", "-dQUIET",
		// The preset comes first so the explicit settings below override it.
		"-dPDFSETTINGS=" + p.preset,
		// Ghostscript defaults to SAFER on modern versions; naming it keeps
		// the guarantee explicit rather than assumed.
		"-dSAFER",
		"-dDetectDuplicateImages=true",
		// Without this Ghostscript copies JPEG streams through untouched, so
		// it neither downsamples nor re-encodes and the whole pass is a no-op
		// that adds a couple of kilobytes. Measured: with passthrough left on,
		// a 1.4 MB scan came out 1.47 MB; with it off, 266 kB.
		"-dPassThroughJPEGImages=false",

		"-dDownsampleColorImages=true",
		"-dColorImageDownsampleType=/Bicubic",
		"-dColorImageResolution=" + strconv.Itoa(p.colorDPI),
		// The default threshold refuses to downsample unless the source
		// exceeds the target by half again, which silently skips most real
		// scans. Any excess resolution is worth removing.
		"-dColorImageDownsampleThreshold=1.0",
		"-dDownsampleGrayImages=true",
		"-dGrayImageDownsampleType=/Bicubic",
		"-dGrayImageResolution=" + strconv.Itoa(p.colorDPI),
		"-dGrayImageDownsampleThreshold=1.0",
		// Text scanned to black and white survives downsampling far worse
		// than a photograph, so it keeps much more resolution.
		"-dDownsampleMonoImages=true",
		"-dMonoImageDownsampleType=/Subsample",
		"-dMonoImageResolution=" + strconv.Itoa(p.monoDPI),
		"-dMonoImageDownsampleThreshold=1.0",

		"-dAutoFilterColorImages=false",
		"-dAutoFilterGrayImages=false",
		"-dColorImageFilter=/DCTEncode",
		"-dGrayImageFilter=/DCTEncode",
	}
	if password != "" {
		args = append(args, "-sPDFPassword="+password)
	}
	args = append(args, "-sOutputFile="+out, in)

	res, err := e.runner.Run(ctx, bin, args, lexec.Options{Timeout: gsTimeout})
	if err != nil || res.ExitCode != 0 {
		return false
	}

	// Ghostscript can exit zero having written nothing useful, so the caller
	// still has to compare it against what the built-in path produced.
	info, err := os.Stat(out)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(out)
		return false
	}
	return true
}

// gsNote describes what happened in the same voice as the built-in path: what
// changed, not which library did it.
//
// It deliberately does not claim a resolution change. The cap only bites on a
// genuinely high resolution scan, and on an ordinary one the saving comes from
// re-encoding, so promising fewer dots per inch would be wrong most of the
// time.
func gsNote(quality string) string {
	return fmt.Sprintf("Pictures recompressed at %s quality.", qualityWord(quality))
}
