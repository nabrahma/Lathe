# Known gaps

Deliberate omissions and honest limitations. Not a bug list: these are things
that are missing on purpose, with the reason.

## Not in v1

- **HEIC without the media component.** No pure-Go HEIC decoder exists that is
  worth depending on, so HEIC conversion prompts for the FFmpeg tier the first
  time. See `DECISIONS.md` D3.
- **Light mode.** The identity is machine black. The tokens are structured to
  support a light variant, but shipping a half-considered one dilutes the thing
  that makes Lathe recognisable in a screenshot.
- **Background removal.** Needs a bundled ONNX model and a runtime. Planned as a
  tier addition once the tier system has proven itself on FFmpeg and
  LibreOffice.
- **Mobile.** Different platform, different constraints, different product.
- **PDF content editing.** Rearranging pages is in scope. Editing text inside a
  page is a fundamentally harder product and is not a direction Lathe is going.
- **AI features.** No summarisation, no chat-with-your-PDF. Either it breaks the
  offline guarantee or it adds hundreds of megabytes of model to the install.
  Both are disqualifying.

## Fidelity limits worth stating plainly

- **PDF → Word is best-effort.** LibreOffice does the conversion. Simple
  documents come through well; complex multi-column layouts with floating
  figures do not. The UI says so before the conversion, not after.
- **PDF page rasterization is lower fidelity than pdfium.** Pages that are
  mostly scanned images, which is the case that matters for OCR, render
  correctly.
  Pages with complex vector artwork may not. See `DECISIONS.md` D5.
- **OCR on a phone photo is not OCR on a scan.** Measured accuracy for both is
  published in the README rather than described with an adjective.
- **The OCR corpus is rendered, not photographed.** The pages are drawn from a
  digital font and then degraded in code, which is a kinder target than a
  photograph of physically printed paper: real accuracy on a creased receipt
  under a desk lamp will be lower than the published figures. The benchmark
  earns its place by catching regressions in the preprocessing chain, not by
  predicting field accuracy. Replacing it with photographs of real documents
  needs ground truth nobody has transcribed yet.
- **Recognition itself is Tesseract, and that is the ceiling here.** The
  transformer recognisers that beat it, PaddleOCR and its kin, need ONNX
  Runtime, whose Go bindings require cgo; adopting them would cost the pure-Go
  CLI described in `DECISIONS.md` D2. The preprocessing in front of Tesseract
  has been measured and improved instead. See `DECISIONS.md` D9.

## Platform inconsistencies

- **Linux uses the window manager's title bar** while Windows and macOS use
  custom chrome, so the window looks different on Linux. See `DECISIONS.md` D6.
- **Windows builds are unsigned** until the project can justify a certificate.
  The README shows exactly what SmartScreen displays and which button to click.

## PDF compression only reduces resolution when Ghostscript is installed

Compressing a PDF re-encodes the pictures inside it, which is what shrinks a
scan: pdfcpu's optimiser is lossless, so on a scanned document it removes
almost nothing, because the pictures *are* the file.

What the built-in path cannot do is downsample them. pdfcpu replaces an image
object in place and requires the replacement to have identical pixel
dimensions, so a 600 DPI scan stays 600 DPI and only the JPEG quality changes.

Lathe therefore uses Ghostscript when it is present, which does downsample, and
that is most of the saving on a large scan. It is deliberately optional: no
task requires it, nothing prompts for a download, and without it compression
still runs on the built-in path. The consequence worth knowing is that the same
file compresses further on a machine that has Ghostscript than on one that does
not.

Bundling it instead was rejected on the licence: Ghostscript is AGPL-3.0, and
fetching it at runtime as a separate process keeps it out of Lathe's own
licensing. On Windows, Lathe downloads and runs Artifex's own installer, pinned
by checksum, into its own folder; on macOS and Linux, where the answer is a
package manager and a root password, it is detected instead. See
`BUNDLING.md`.
