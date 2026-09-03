# Changelog

Notable changes, newest first. Versions follow [semantic versioning](https://semver.org).

## 1.0.0

First release.

### Added

**Twenty-seven tasks.** PDF compress, merge, split, rotate, delete pages, reorder
pages, watermark, protect, unlock, export pages as images, build from images.
Image convert between JPG, PNG, WEBP, TIFF, BMP and GIF, plus compress, resize
and crop. Text extraction from photos, from PDFs, and a scanned-PDF-to-
searchable-PDF pass. Document conversion covering PDF to Word, Office to PDF
and the Office to OpenDocument interchange. Video and audio conversion,
compression, audio extraction and GIF creation.

**A home screen that filters itself.** Drop a file and the grid narrows to what
that file can actually become, so you never have to know which category
anything is in.

**Compression measured against itself.** Compressing a PDF runs the built-in
path and, when Ghostscript is installed, a second pass that can also reduce
image resolution, then keeps whichever file is actually smaller. On the test
corpus that is 250 kB against 162 kB at the best-quality setting. Ghostscript is
optional in the real sense: no task requires it, nothing prompts for it, and the
comparison means it can never make a file worse.

**OCR that survives a shadow.** A photographed page has its lighting flattened
before anything else reads it, and is then binarised with Sauvola's method,
which chooses a threshold for every pixel from its own surroundings rather than
one for the whole page. On a page with a hard-edged shadow across it, the kind
cast by your own hand or by the gutter of a bound book, that is the difference
between reading 37 percent of the text and 99 percent.

**Optional components, downloaded once.** The base install is around 18 MB and
handles PDF and images with no external binary at all. Video, OCR and Office
formats each pull a component the first time you ask for them, verified against
a pinned SHA-256 and then working offline forever.

**Verified atomic output.** Every result is written to a sibling temp file,
flushed and renamed into place, so a crash or a power loss leaves either a
complete file or nothing.

**Translation infrastructure.** The interface ships in English. Strings are
externalised and the layout handles right-to-left, so a new language is a JSON
file and a registry line rather than a refactor. See `docs/TRANSLATING.md`.

**Shell integration.** An optional "Convert with Lathe" entry in the Windows
right-click menu.

**An opt-in update check.** Off by default. When on, it sends a version string
and nothing else, and it never downloads or installs anything by itself.

### Guarantees, and how they are checked

- Files never leave the machine. `scripts/boundary` fails the build on a
  network import outside the two packages allowed to have one, and
  `test/isolation` runs the full conversion matrix with DNS blocked.
- Input files are never modified. `test/integrity` hashes every input before
  and after each task, and kills jobs at randomised points mid-run to prove an
  interrupted conversion leaves both the original and the destination
  untouched.
- Errors are written for people. Every failure carries a cause, a consequence
  and a next step. A raw Go error string reaching the interface is treated as a
  bug.

### Known limitations

HEIC needs the media component. PDF to Word is best-effort on complex layouts.
Compressing a PDF reduces image resolution only when Ghostscript is installed;
without it the built-in path re-encodes at the existing pixel dimensions,
because pdfcpu requires a replacement image to keep them exactly. Recognition
itself is Tesseract, and the transformer recognisers that beat it need cgo,
which would cost the pure-Go command line tool. Windows and Linux builds are
unsigned. The full list, with reasons, is in `docs/KNOWN_GAPS.md`.
