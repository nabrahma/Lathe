// Package matrix_test runs every task against representative corpus files.
//
// This is the test that catches a regression when a dependency is upgraded.
// For each case it asserts one of two acceptable outcomes — a valid output, or
// a mapped error with a next action — and, in both cases, that the input file
// is byte-for-byte unchanged.
package matrix_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/imageengine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
)

const corpus = "../../testdata/corpus"

// outcome is what a matrix case is allowed to do.
type outcome int

const (
	// wantSuccess: the task must produce a valid output.
	wantSuccess outcome = iota
	// wantMappedError: the task must fail, but with a translated message and a
	// next action — never a raw engine string.
	wantMappedError
)

type matrixCase struct {
	name    string
	taskID  string
	inputs  []string
	options map[string]any
	want    outcome
}

func cases() []matrixCase {
	pdf := func(n string) string { return filepath.Join(corpus, "pdf", n) }
	img := func(n string) string { return filepath.Join(corpus, "images", n) }
	bad := func(n string) string { return filepath.Join(corpus, "adversarial", n) }

	return []matrixCase{
		// PDF tasks against well-formed input.
		{"compress one page", "pdf.compress", []string{pdf("single-page.pdf")}, nil, wantSuccess},
		{"compress sixty pages", "pdf.compress", []string{pdf("many-page.pdf")}, nil, wantSuccess},
		{"compress a scan", "pdf.compress", []string{pdf("scanned.pdf")}, nil, wantSuccess},
		{"merge two", "pdf.merge", []string{pdf("single-page.pdf"), pdf("five-page.pdf")}, nil, wantSuccess},
		{"merge three", "pdf.merge",
			[]string{pdf("single-page.pdf"), pdf("five-page.pdf"), pdf("scanned.pdf")}, nil, wantSuccess},
		{"split into pages", "pdf.split", []string{pdf("five-page.pdf")},
			map[string]any{"mode": "pages"}, wantSuccess},
		{"split every two", "pdf.split", []string{pdf("five-page.pdf")},
			map[string]any{"mode": "every", "span": 2}, wantSuccess},
		{"split by range", "pdf.split", []string{pdf("five-page.pdf")},
			map[string]any{"mode": "range", "pages": "2-4"}, wantSuccess},
		{"rotate all", "pdf.rotate", []string{pdf("five-page.pdf")},
			map[string]any{"angle": 90}, wantSuccess},
		{"rotate a selection", "pdf.rotate", []string{pdf("five-page.pdf")},
			map[string]any{"angle": 180, "pages": "1,3"}, wantSuccess},
		{"delete pages", "pdf.delete-pages", []string{pdf("five-page.pdf")},
			map[string]any{"pages": "2,4"}, wantSuccess},
		{"reorder pages", "pdf.reorder", []string{pdf("five-page.pdf")},
			map[string]any{"order": "5,4,3,2,1"}, wantSuccess},
		{"watermark", "pdf.watermark", []string{pdf("five-page.pdf")},
			map[string]any{"text": "DRAFT", "opacity": 0.3, "position": "diagonal"}, wantSuccess},
		{"protect", "pdf.protect", []string{pdf("single-page.pdf")},
			map[string]any{"password": "correct horse"}, wantSuccess},
		{"unlock with the right password", "pdf.unlock", []string{pdf("password-protected.pdf")},
			map[string]any{"password": "lathe"}, wantSuccess},
		{"pdf to images from a scan", "pdf.to-images", []string{pdf("scanned.pdf")},
			map[string]any{"format": "png"}, wantSuccess},
		{"images to pdf", "pdf.from-images",
			[]string{img("photo.png"), img("photo.jpg")}, nil, wantSuccess},
		{"images to pdf, letter", "pdf.from-images", []string{img("photo.png")},
			map[string]any{"pageSize": "Letter"}, wantSuccess},

		// Image tasks.
		{"png to jpg", "image.convert", []string{img("photo.png")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"jpg to png", "image.convert", []string{img("photo.jpg")},
			map[string]any{"format": "png"}, wantSuccess},
		{"png to webp", "image.convert", []string{img("photo.png")},
			map[string]any{"format": "webp"}, wantSuccess},
		{"tiff to jpg", "image.convert", []string{img("scan.tiff")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"bmp to png", "image.convert", []string{img("photo.bmp")},
			map[string]any{"format": "png"}, wantSuccess},
		{"gif to png", "image.convert", []string{img("animation.gif")},
			map[string]any{"format": "png"}, wantSuccess},
		{"transparent png to jpg flattens", "image.convert", []string{img("transparent.png")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"convert several at once", "image.convert",
			[]string{img("photo.png"), img("photo.jpg"), img("photo.bmp")},
			map[string]any{"format": "png"}, wantSuccess},
		{"compress a large photo", "image.compress", []string{img("large.jpg")},
			map[string]any{"quality": 60}, wantSuccess},
		{"compress with a width limit", "image.compress", []string{img("large.jpg")},
			map[string]any{"quality": 70, "maxWidth": 1200}, wantSuccess},
		{"resize by width", "image.resize", []string{img("photo.png")},
			map[string]any{"preset": "custom", "width": 320}, wantSuccess},
		{"resize to a preset", "image.resize", []string{img("photo.jpg")},
			map[string]any{"preset": "passport"}, wantSuccess},
		{"crop to square", "image.crop", []string{img("photo.png")},
			map[string]any{"aspect": "1:1"}, wantSuccess},
		{"crop to an area", "image.crop", []string{img("photo.png")},
			map[string]any{"aspect": "free", "rect": "10,10,200,150"}, wantSuccess},

		// Hostile filenames must behave exactly like ordinary ones.
		{"filename with spaces", "image.convert", []string{bad("spaces in the name.png")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"filename with a semicolon", "image.convert", []string{bad("semicolon; rm -rf tmp.png")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"filename with command substitution", "image.convert", []string{bad("dollar $(whoami).png")},
			map[string]any{"format": "jpg"}, wantSuccess},
		{"filename with non-ASCII characters", "image.convert", []string{bad("unicode-café-日本語.png")},
			map[string]any{"format": "png"}, wantSuccess},
		{"filename with an emoji", "image.convert", []string{bad("emoji-📄.png")},
			map[string]any{"format": "png"}, wantSuccess},

		// Adversarial input must fail, but legibly.
		{"empty file", "pdf.compress", []string{bad("empty.pdf")}, nil, wantMappedError},
		{"truncated pdf", "pdf.compress", []string{bad("truncated.pdf")}, nil, wantMappedError},
		{"html named pdf", "pdf.compress", []string{bad("actually-html.pdf")}, nil, wantMappedError},
		{"png named jpg still converts", "image.convert", []string{bad("actually-png.jpg")},
			map[string]any{"format": "png"}, wantSuccess},
		{"heic named jpg asks for the component", "image.convert", []string{bad("actually-heic.jpg")},
			map[string]any{"format": "jpg"}, wantMappedError},
		{"locked pdf without a password", "pdf.compress", []string{pdf("password-protected.pdf")}, nil, wantMappedError},
		{"unlock with the wrong password", "pdf.unlock", []string{pdf("password-protected.pdf")},
			map[string]any{"password": "not the password"}, wantMappedError},
		{"delete every page", "pdf.delete-pages", []string{pdf("five-page.pdf")},
			map[string]any{"pages": "1-5"}, wantMappedError},
		{"page range past the end", "pdf.split", []string{pdf("five-page.pdf")},
			map[string]any{"mode": "range", "pages": "40-50"}, wantMappedError},
		{"resize with no size", "image.resize", []string{img("photo.png")},
			map[string]any{"preset": "custom", "width": 0, "height": 0}, wantMappedError},
		{"crop with no shape", "image.crop", []string{img("photo.png")},
			map[string]any{"aspect": "free", "rect": ""}, wantMappedError},
	}
}

func TestConversionMatrix(t *testing.T) {
	registry := task.Default()
	runner := pipeline.New(engine.NewRegistry(pdfengine.New(), imageengine.New()), nil)

	for _, tc := range cases() {
		t.Run(tc.name, func(t *testing.T) {
			tk, ok := registry.Get(tc.taskID)
			if !ok {
				t.Fatalf("no such task %q", tc.taskID)
			}

			for _, in := range tc.inputs {
				if _, err := os.Stat(in); err != nil {
					t.Skipf("corpus file missing (run: make corpus): %v", err)
				}
			}
			before := hashAll(t, tc.inputs)

			outDir := t.TempDir()
			opts := engine.Options(tk.Defaults())
			for k, v := range tc.options {
				opts[k] = v
			}

			res, err := runner.Run(context.Background(), pipeline.Request{
				Task: tk, Inputs: tc.inputs, Options: opts, OutputDir: outDir,
			})

			// Rule one, always: the input is untouched, whatever happened.
			if after := hashAll(t, tc.inputs); after != before {
				t.Fatal("an input file was modified")
			}

			switch tc.want {
			case wantSuccess:
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				if len(res.Outputs) == 0 {
					t.Fatal("succeeded with no output")
				}
				for _, out := range res.Outputs {
					info, statErr := os.Stat(out)
					if statErr != nil {
						t.Fatalf("output missing: %v", statErr)
					}
					if info.Size() < 64 {
						t.Errorf("%s is %d bytes, which is too small to be real", filepath.Base(out), info.Size())
					}
					if filepath.Dir(out) != outDir {
						t.Errorf("%s was written outside the chosen folder", out)
					}
				}

			case wantMappedError:
				if err == nil {
					t.Fatalf("expected a failure, got %d outputs", len(res.Outputs))
				}
				assertPresentable(t, err)
				// A failed job must leave the output folder exactly as it was.
				if entries, _ := os.ReadDir(outDir); len(entries) != 0 {
					t.Errorf("a failed job left %d files behind", len(entries))
				}
			}
		})
	}
}

// TestEveryTaskIsCovered keeps the matrix honest as tasks are added.
func TestEveryLocalTaskIsCoveredByTheMatrix(t *testing.T) {
	covered := map[string]bool{}
	for _, c := range cases() {
		covered[c.taskID] = true
	}

	// Only the engines this matrix registers are in scope. Tasks driven by a
	// downloaded component are covered by their own engine's tests, which skip
	// when the component is absent.
	local := map[string]bool{task.EnginePDF: true, task.EngineImage: true}

	for _, tk := range task.Catalog() {
		if !local[tk.Engine] {
			continue
		}
		if !covered[tk.ID] {
			t.Errorf("task %q has no matrix case", tk.ID)
		}
	}
}

func assertPresentable(t *testing.T, err error) {
	t.Helper()

	var ue *usererr.Error
	if !errors.As(err, &ue) {
		t.Fatalf("error was not translated for display: %v", err)
	}
	if len(ue.Actions) == 0 {
		t.Errorf("error offers no next action: %q", ue.Message)
	}
	for _, banned := range []string{"pdfcpu", "ffmpeg", "tesseract", "exit status", "0x", "goroutine"} {
		if strings.Contains(strings.ToLower(ue.Message), banned) {
			t.Errorf("message leaks %q: %q", banned, ue.Message)
		}
	}
	if ue.Message == strings.ToUpper(ue.Message) {
		t.Errorf("message is uppercase: %q", ue.Message)
	}
}

func hashAll(t *testing.T, paths []string) string {
	t.Helper()
	h := sha256.New()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}
	return hex.EncodeToString(h.Sum(nil))
}
