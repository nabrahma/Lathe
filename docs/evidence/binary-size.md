# Binary size

Measured from a release build, not estimated.

| Platform | Artefact | Size | Target | Hard limit |
|---|---|---:|---:|---:|
| Windows | `lathe.exe` | 18.1 MB | 55 MB | 70 MB |

The figure is the whole application: the Go core, every PDF and image engine,
and the embedded interface. Nothing else is installed alongside it.

What is *not* in that number, by design:

| Component | Size | When |
|---|---:|---|
| Video and photo support (FFmpeg) | 111 MB download | First video task, or the first HEIC photo |
| Text recognition (Tesseract) | detected, not downloaded | First OCR task |
| Office documents (LibreOffice) | detected, not downloaded | First Word or Excel task |

Regenerate with `make build`, then `bash scripts/size-budget.sh`, which is the
same gate CI applies.
