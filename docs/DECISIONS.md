# Decisions

Architecture decisions, with the reasoning that produced them. Entries are
append-only: superseding a decision means adding a new entry that says so.

## D1. Wails v2, not v3

**Decision.** Build on Wails v2 (`v2.15.0`).

**Why.** v3 is technically better in every respect (first-class dialogs, a
system tray, GTK4/WebKitGTK 6.0, Windows ARM64) but is still pre-stable. Lathe
handles people's documents; a pre-stable shell is not an acceptable dependency
for that.

**Consequence.** v3 will eventually be the right target, so migration is kept
cheap: no package under `internal/` may import Wails except `internal/app`,
which is a thin binding layer over the same interfaces the CLI drives. The rule
is enforced in CI by `scripts/boundary`. If the boundary holds, migrating is
rewriting one package.

**Rejected.** Electron (~150 MB before a line of code, against a 60 MB
installer budget); a local web server plus the system browser (loses OS file
dialogs and drag-from-Explorer, and reproduces the "localhost tab" experience
Lathe exists to avoid); three native UIs (not viable for one maintainer).

## D2. Shell out to external binaries, never link them

**Decision.** Tesseract, FFmpeg and LibreOffice are invoked as subprocesses via
`internal/exec`. Nothing is linked with cgo.

**Why.** Keeps `CGO_ENABLED=0` for the main binary, which makes
cross-compilation trivial; isolates an engine crash in a subprocess rather than
taking down the app with the user's file open; and makes every engine a
uniformly managed component in the tier system rather than a special case.

**Cost.** One process spawn per job, which is negligible next to the runtime of
the work itself.

## D3. Pure-Go image pipeline, with FFmpeg as the exotic-format tier

**Decision.** Diverges from the original plan of libvips. Core image work uses
`golang.org/x/image`, `disintegration/imaging` and `nativewebp`, all pure Go.
HEIC, AVIF and other codecs that have no pure-Go implementation are handled by
the FFmpeg tier.

**Why.** libvips means cgo, which means D2's `CGO_ENABLED=0` guarantee is lost
and every platform needs a bundled shared library built and shipped. The
pure-Go set covers JPEG, PNG, GIF, BMP, TIFF and WebP, which is the
overwhelming majority of real input. HEIC is the one important gap, and it is
a common one since phones shoot it, but FFmpeg decodes it well.

**Consequence.** HEIC conversion prompts for the media component the first
time. That is honest and visible, which is the tier system working as designed,
not a workaround. Recorded in `KNOWN_GAPS.md` as a candidate for a pure-Go HEIC
decoder later.

## D4. AGPL-3.0

**Decision.** Lathe itself is AGPL-3.0. Component licenses are catalogued in
`LICENSES.md`.

**Why.** FFmpeg static builds are typically GPL. Because Lathe downloads them at
runtime rather than bundling them, it is arguably not a derivative work, but
"arguably" is an uncomfortable place to stand when the alternative costs
nothing. AGPL is compatible with every component in the tree, and it prevents
the work being re-hosted as a paid SaaS, which is precisely the model Lathe
exists as an alternative to.

**Consequence.** The runtime-download architecture stays. Bundling a GPL FFmpeg
build into the installer would change this analysis and must not be done
without revisiting this entry.

## D5. Frameless on Windows and macOS, native title bar on Linux

**Decision.** Custom window chrome on Windows and macOS; the window manager's
own title bar on Linux.

**Why.** A grey OS title bar above a near-black app looks unfinished, and
frameless is well-trodden on both Windows and macOS. Linux window managers vary
too much, with dragging, snapping and maximise behaviour all differing, to own
that surface in v1.

**Consequence.** The window looks different on Linux. That is stated in the
README rather than papered over.

## D6. Pure-Go PDF rasterization

**Decision.** PDF page rendering goes through pdfcpu's content extraction plus
an internal rasterizer rather than pdfium.

**Why.** pdfium means either cgo or a large WebAssembly runtime shipped in the
core tier. Both conflict with D2 and the 60 MB installer budget. Where
full-fidelity rasterization is genuinely required, meaning OCR of a scanned
PDF, the already-present engines handle it.

**Consequence.** Page rendering fidelity is lower than pdfium's for pages with
complex vector content. Recorded honestly in `KNOWN_GAPS.md`.

## D7. Ghostscript as an optional enhancement, not a tier

**Decision.** Compress PDF uses Ghostscript when it is already installed, and
falls back to the built-in pdfcpu path when it is not. No task requires it,
nothing prompts for a download, and it sits in its own tier that gates nothing.

**Why.** Downsampling is the whole difference on a large scan, and pdfcpu
cannot do it: it replaces an image object in place and demands identical pixel
dimensions, so quality is the only lever. Ghostscript does downsample. But
Compress PDF is a core task that has to work on a bare install, so making it
depend on an external tool would be a worse product than compressing less far.

**Alternatives rejected.** Bundling Ghostscript: Artifex ships platform
installers rather than portable checksummed archives, so it fails the same
verification rule that made Tesseract and LibreOffice detected rather than
downloaded. Making it a required tier: it would put a download prompt in front
of the most used task in the app.

**Consequence.** The same file compresses further on a machine that has
Ghostscript than on one that does not, so two results are not directly
comparable. Recorded in `KNOWN_GAPS.md`.

## D8. Keep Go's JPEG encoder, despite jpegli

**Decision.** Image and PDF compression keep the standard library's
`image/jpeg` encoder.

**Why.** jpegli is reported to beat libjpeg-turbo and MozJPEG on human
preference at matched size, and `gen2brain/jpegli` provides it CGo-free through
wazero, so it looked like a free upgrade. Measured on this project's own corpus
it was not: matching the standard library's SSIM required 9 to 22 percent
**more** bytes on clean sources, and it only won on one already-compressed
photograph at the highest quality setting.

The published claim is about human preference, which cannot be asserted in a
test. The measurable claim went the other way, and shipping an encoder that
makes files larger on the metric available would contradict the numbers the
README publishes.

**Consequence.** Compression is a little behind what a perceptually tuned
encoder could achieve. Revisit if a Go implementation of butteraugli or
SSIMULACRA2 appears, which would make the trade measurable rather than
asserted.

## D9. Sauvola binarisation, with the lighting flattened first

**Decision.** The OCR preprocessing chain flattens the page lighting before any
step that reads the page as a whole, and binarises with Sauvola's method rather
than Otsu's.

**Why.** Otsu picks one intensity to separate ink from paper across the entire
page. That is the right question for a flatbed scan and the wrong one for a
photograph: a shadow lying across the page forces a single choice to serve both
halves at once, light enough to keep the shaded paper white and the faint
strokes wash out, dark enough to catch them and the shadow becomes a block of
ink. Sauvola asks the question per pixel instead, against that pixel's own
surroundings, which makes the gradient irrelevant because the paper beside a
stroke lies in the same shadow as the stroke.

The ordering was the larger surprise. Deskew scores a rotation by how much the
row darkness varies, and the border trim crops rows that are mostly dark; under
a shadow both are measuring the lighting rather than the page, and the trim
quietly removes shaded text as though it were the dark edge of a desk.
Flattening after these steps left 84.7 percent on a lit gradient; flattening
before them gave 100 percent.

Measured on the corpus under a hard-edged shadow, the case a coarse estimate of
the lighting cannot follow:

| Chain | Accuracy |
|---|---:|
| Otsu, no flattening | 37% |
| Otsu, lighting flattened | 68% |
| Sauvola, lighting flattened | 99% |

All three are identical, at 100 percent, on an evenly lit page, so neither step
costs anything on easy input.

**Alternatives rejected.** Flattening alone, which is cheaper than Sauvola, gets
two thirds of the way and stops: it estimates the lighting on a coarse grid,
which follows a gradient closely and cannot follow a step at all. Sauvola's own
parameters were swept from k=0.15 to k=0.34 and across three window sizes; once
flattening was in place the spread was a third of a point, which four passages
cannot resolve, so k stays at the value in common use rather than being fitted
to the noise.

**Consequence.** Two extra passes over the image, both linear: the flattening
estimate is a coarse grid, and Sauvola runs off summed-area tables so the window
size does not affect the cost. Neither is measurable beside Tesseract itself.

