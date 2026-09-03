package pdfengine

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/nabrahma/lathe/internal/engine"
)

// Compressing a PDF that is mostly scanned pages means recompressing the
// images inside it. pdfcpu's optimiser is lossless, so on a scan it removes
// almost nothing: the pictures are the file.
//
// That is the case people actually have. "I have a 14 MB scan and the form
// accepts 2 MB" is the reason Compress PDF exists, and an optimiser pass alone
// would answer it with "already as small as it can get", which is true of the
// structure and useless to the user.

// profile is what a quality word means in numbers. The user never sees any of
// this; they see "Smaller file", "Balanced" and "Best quality".
//
// Only the encoder quality is adjustable. pdfcpu replaces an image object in
// place and requires the replacement to have identical pixel dimensions, so
// downsampling a 300 DPI scan to 150 DPI is not available through this path;
// it is recorded in docs/KNOWN_GAPS.md. Quality alone still does most of the
// work, because a scanner writes JPEG at a quality far above what text needs.
type profile struct {
	quality int
}

func profileFor(quality string) profile {
	switch quality {
	case "low":
		return profile{quality: 40}
	case "high":
		return profile{quality: 78}
	default:
		return profile{quality: 58}
	}
}

// minWorthwhile skips images too small to be worth touching. Icons and logos
// cost nothing, and re-encoding them tends to make them larger.
const minWorthwhile = 16 << 10

// maxImageBytes bounds a single embedded image, so a hostile or broken
// document cannot exhaust memory here.
const maxImageBytes = 256 << 20

// replacement is one image to swap in.
type replacement struct {
	objNr int
	data  []byte
}

// recompressImages rewrites the pictures inside a PDF at the requested
// quality, returning the new document. It reports how many images it touched
// so the caller can tell the user something true about what happened.
func recompressImages(in string, p profile, conf *model.Configuration,
	progress func(engine.Progress),
) (result []byte, changed int, err error) {
	original, err := os.ReadFile(in)
	if err != nil {
		return nil, 0, err
	}

	// A PDF whose images cannot be enumerated is not a failure: it means there
	// is nothing here to recompress, and the optimiser pass still runs. The
	// document is returned untouched rather than the error propagated.
	images, err := api.ExtractImagesRaw(bytes.NewReader(original), nil, conf)
	if err != nil || len(images) == 0 {
		return original, 0, nil //nolint:nilerr // an unreadable image set means no work, not an error
	}

	// The readers pdfcpu hands back are bound to the extraction pass, so each
	// image has to be read here and now. Collecting them to process later
	// yields empty readers and, silently, no compression at all.
	type source struct {
		objNr int
		body  []byte
	}
	var sources []source
	seen := make(map[int]bool)

	for _, page := range images {
		for _, img := range page {
			// An object can appear on several pages; recompressing it twice
			// would be wasted work and a second round of quality loss.
			if seen[img.ObjNr] {
				continue
			}
			seen[img.ObjNr] = true

			// An image mask is a stencil, not a picture, and re-encoding one
			// as JPEG destroys it.
			if img.IsImgMask || img.HasSMask || img.Reader == nil {
				continue
			}

			body, readErr := io.ReadAll(io.LimitReader(img, maxImageBytes))
			if readErr != nil || len(body) < minWorthwhile {
				continue
			}
			sources = append(sources, source{objNr: img.ObjNr, body: body})
		}
	}
	if len(sources) == 0 {
		return original, 0, nil
	}

	var replacements []replacement
	for i, src := range sources {
		progress(engine.Progress{
			Fraction: float64(i) / float64(len(sources)),
			Stage:    fmt.Sprintf("Compressing picture %d of %d", i+1, len(sources)),
		})

		encoded, ok := shrink(src.body, p)
		if !ok {
			continue
		}
		replacements = append(replacements, replacement{objNr: src.objNr, data: encoded})
	}

	if len(replacements) == 0 {
		return original, 0, nil
	}

	// Each replacement rewrites the document, so this is applied in memory
	// rather than through the filesystem.
	current := original
	var lastErr error
	for _, r := range replacements {
		var out bytes.Buffer
		if err := api.UpdateImages(bytes.NewReader(current), bytes.NewReader(r.data),
			&out, r.objNr, 0, "", conf); err != nil {
			// One awkward image must not lose the rest of the work, but if
			// every one fails the caller needs to know rather than being told
			// the document was already optimal.
			lastErr = err
			continue
		}
		current = out.Bytes()
		changed++
	}
	if changed == 0 && lastErr != nil {
		return original, 0, fmt.Errorf("recompress images: %w", lastErr)
	}
	return current, changed, nil
}

// shrink re-encodes one embedded image, and reports whether the result is
// actually worth using. Returning a larger image would be worse than doing
// nothing, which is a real outcome when the source is already well compressed.
func shrink(raw []byte, p profile) ([]byte, bool) {
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, decoded, &jpeg.Options{Quality: p.quality}); err != nil {
		return nil, false
	}

	// Only worth it if it saves something meaningful. Replacing a picture to
	// gain two percent is a quality loss the user did not get paid for.
	if out.Len() >= len(raw)*95/100 {
		return nil, false
	}
	return out.Bytes(), true
}
