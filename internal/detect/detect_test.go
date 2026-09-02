package detect_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/nabrahma/lathe/internal/detect"
)

func TestDetectIdentifiesByContentNotExtension(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name     string
		filename string
		data     []byte
		wantExt  string
		wantCat  detect.Category
	}{
		{"png", "a.png", pngBytes(t, 120, 80), "png", detect.CategoryImage},
		{"jpeg", "a.jpg", jpegBytes(t, 64, 48), "jpg", detect.CategoryImage},
		{"gif", "a.gif", gifBytes(t, 32, 16), "gif", detect.CategoryImage},
		{"pdf", "a.pdf", minimalPDF(), "pdf", detect.CategoryPDF},

		// The cases that make content detection worth doing at all.
		{"heic named jpg", "photo.jpg", heicHeader(), "heic", detect.CategoryImage},
		{"html named pdf", "download.pdf", []byte("<!DOCTYPE html><html><body>404</body></html>"), "html", detect.CategoryText},
		{"png named docx", "report.docx", pngBytes(t, 8, 8), "png", detect.CategoryImage},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.filename)
			if err := os.WriteFile(path, tc.data, 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := detect.Detect(path)
			if err != nil {
				t.Fatalf("detect: %v", err)
			}
			if got.Extension != tc.wantExt {
				t.Errorf("extension %q, want %q (mime %q)", got.Extension, tc.wantExt, got.MIME)
			}
			if got.Category != tc.wantCat {
				t.Errorf("category %q, want %q", got.Category, tc.wantCat)
			}
		})
	}
}

func TestMismatchesNameFlagsALyingExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(path, heicHeader(), 0o644); err != nil {
		t.Fatal(err)
	}

	ft, err := detect.Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ft.MismatchesName("photo.jpg") {
		t.Error("a HEIC named .jpg should be reported as a mismatch")
	}
	if ft.MismatchesName("photo.heic") {
		t.Error("a HEIC named .heic should not be a mismatch")
	}
}

func TestMismatchesNameToleratesEquivalentExtensions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jpeg")
	if err := os.WriteFile(path, jpegBytes(t, 10, 10), 0o644); err != nil {
		t.Fatal(err)
	}
	ft, err := detect.Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if ft.MismatchesName("a.jpeg") {
		t.Error(".jpeg and .jpg name the same format")
	}
}

func TestDetectReportsAnEmptyFileClearly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.pdf")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := detect.Detect(path); !errors.Is(err, detect.ErrEmptyFile) {
		t.Fatalf("got %v, want ErrEmptyFile", err)
	}
}

func TestDetectFindsAnEncryptedPDFUpFront(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.pdf")
	body := append(minimalPDF(), []byte("\ntrailer\n<< /Encrypt 5 0 R /Root 1 0 R >>\n%%EOF\n")...)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	ft, err := detect.Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ft.Encrypted {
		t.Error("an encrypted PDF should be flagged before a job starts")
	}
}

func TestDetectReadsImageDimensions(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		data []byte
		w, h string
	}{
		{"a.png", pngBytes(t, 321, 123), "321", "123"},
		{"a.jpg", jpegBytes(t, 200, 100), "200", "100"},
		{"a.gif", gifBytes(t, 40, 20), "40", "20"},
	} {
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, tc.data, 0o644); err != nil {
			t.Fatal(err)
		}
		ft, err := detect.Detect(path)
		if err != nil {
			t.Fatal(err)
		}
		if ft.Details["width"] != tc.w || ft.Details["height"] != tc.h {
			t.Errorf("%s: got %sx%s, want %sx%s", tc.name,
				ft.Details["width"], ft.Details["height"], tc.w, tc.h)
		}
	}
}

func TestDetectRejectsADirectory(t *testing.T) {
	if _, err := detect.Detect(t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory")
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage(w, h)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, testImage(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xFF})
		}
	}
	return img
}

// heicHeader is a real HEIC file signature: the brand lives inside the ftyp
// box at offset 4, which is exactly why prefix-only detection misses it.
func heicHeader() []byte {
	b := make([]byte, 64)
	copy(b[0:4], []byte{0, 0, 0, 0x18})
	copy(b[4:8], []byte("ftyp"))
	copy(b[8:12], []byte("heic"))
	copy(b[12:16], []byte{0, 0, 0, 0})
	copy(b[16:24], []byte("mif1heic"))
	return b
}

func minimalPDF() []byte {
	return []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")
}
