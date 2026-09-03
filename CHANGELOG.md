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
Compressing a PDF cannot reduce image resolution, only re-encode at lower
quality, because pdfcpu requires a replacement image to keep its exact pixel
dimensions. Windows and Linux builds are unsigned. The full list, with reasons,
is in `docs/KNOWN_GAPS.md`.
