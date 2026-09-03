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

## Platform inconsistencies

- **Linux uses the window manager's title bar** while Windows and macOS use
  custom chrome, so the window looks different on Linux. See `DECISIONS.md` D6.
- **Windows builds are unsigned** until the project can justify a certificate.
  The README shows exactly what SmartScreen displays and which button to click.

## PDF compression cannot reduce resolution

Compressing a PDF re-encodes the pictures inside it, which is what shrinks a
scan: pdfcpu's optimiser is lossless, so on a scanned document it removes
almost nothing, because the pictures *are* the file.

What it cannot do is downsample them. pdfcpu replaces an image object in place
and requires the replacement to have identical pixel dimensions, so a 600 DPI
scan stays 600 DPI and only the JPEG quality changes. Quality alone still does
most of the work, since scanners write at a quality far above what text needs,
but a tool that could also halve the resolution would go further on the largest
files.

Doing better means rebuilding the document from downsampled page images, which
is correct for a pure scan and destructive for anything with a text layer,
links or form fields. That distinction has to be detected reliably before the
rebuild path is worth offering, so it is not in v1.
