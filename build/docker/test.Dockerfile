# Test image for the parts of Lathe that need external engines. Windows and
# macOS developers usually have neither Tesseract nor FFmpeg installed, and
# this makes the OCR benchmark runnable anywhere Docker is.
FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
        tesseract-ocr tesseract-ocr-eng ffmpeg \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod
CMD ["go", "test", "./..."]
