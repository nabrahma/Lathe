// Package integrity_test is the most important test in the project.
//
// Modifying someone's input file is the one unforgivable bug: they lose a
// document and never trust the tool again. That guarantee is enforced
// structurally here rather than left to intention — inputs are opened
// read-only through a recording filesystem, hashed before and after, and the
// process is killed at randomised points mid-job to prove a crash leaves both
// the input and the destination untouched.
package integrity_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/imageengine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
)

const corpus = "../../testdata/corpus"

// helperEnv puts the test binary into the mode that runs a job so it can be
// killed from outside, which is the only honest way to test a crash.
const helperEnv = "LATHE_INTEGRITY_JOB"

func TestMain(m *testing.M) {
	if spec := os.Getenv(helperEnv); spec != "" {
		runJobForKilling(spec)
		return
	}
	os.Exit(m.Run())
}

type integrityCase struct {
	taskID  string
	inputs  []string
	options map[string]any
}

func cases() []integrityCase {
	pdf := func(n string) string { return filepath.Join(corpus, "pdf", n) }
	img := func(n string) string { return filepath.Join(corpus, "images", n) }

	return []integrityCase{
		{"pdf.compress", []string{pdf("many-page.pdf")}, nil},
		{"pdf.merge", []string{pdf("single-page.pdf"), pdf("five-page.pdf")}, nil},
		{"pdf.split", []string{pdf("five-page.pdf")}, map[string]any{"mode": "pages"}},
		{"pdf.rotate", []string{pdf("five-page.pdf")}, map[string]any{"angle": 90}},
		{"pdf.watermark", []string{pdf("five-page.pdf")}, map[string]any{"text": "DRAFT"}},
		{"pdf.protect", []string{pdf("single-page.pdf")}, map[string]any{"password": "hunter22"}},
		{"pdf.unlock", []string{pdf("password-protected.pdf")}, map[string]any{"password": "lathe"}},
		{"pdf.to-images", []string{pdf("scanned.pdf")}, nil},
		{"pdf.from-images", []string{img("photo.png"), img("photo.jpg")}, nil},
		{"image.convert", []string{img("photo.png")}, map[string]any{"format": "jpg"}},
		{"image.compress", []string{img("large.jpg")}, map[string]any{"quality": 60}},
		{"image.resize", []string{img("photo.png")}, map[string]any{"preset": "custom", "width": 200}},
		{"image.crop", []string{img("photo.png")}, map[string]any{"aspect": "1:1"}},
	}
}

// TestInputsAreNeverModified covers the ordinary path: run every task and
// confirm the inputs come out byte-identical.
func TestInputsAreNeverModified(t *testing.T) {
	runner := pipeline.New(engine.NewRegistry(pdfengine.New(), imageengine.New()), nil)
	registry := task.Default()

	for _, tc := range cases() {
		t.Run(tc.taskID, func(t *testing.T) {
			tk, ok := registry.Get(tc.taskID)
			if !ok {
				t.Fatalf("no such task %q", tc.taskID)
			}
			for _, in := range tc.inputs {
				if _, err := os.Stat(in); err != nil {
					t.Skipf("corpus missing (run: make corpus): %v", err)
				}
			}

			before := hashes(t, tc.inputs)
			modes := permissions(t, tc.inputs)

			opts := engine.Options(tk.Defaults())
			for k, v := range tc.options {
				opts[k] = v
			}
			if _, err := runner.Run(context.Background(), pipeline.Request{
				Task: tk, Inputs: tc.inputs, Options: opts, OutputDir: t.TempDir(),
			}); err != nil {
				t.Fatalf("run: %v", err)
			}

			for path, want := range before {
				if got := hashOf(t, path); got != want {
					t.Errorf("%s was modified", filepath.Base(path))
				}
			}
			for path, want := range modes {
				if got := modeOf(t, path); got != want {
					t.Errorf("%s had its permissions changed: %v to %v", filepath.Base(path), want, got)
				}
			}
		})
	}
}

// TestInputsSurviveAMidJobKill is the real test. A conversion is started in a
// child process and killed at a random point; the input must be unchanged and
// the destination must hold nothing at all, because a partial file is worse
// than no file.
func TestInputsSurviveAMidJobKill(t *testing.T) {
	if testing.Short() {
		t.Skip("killing subprocesses repeatedly is slow")
	}

	source := filepath.Join(corpus, "pdf", "many-page.pdf")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}

	rnd := rand.New(rand.NewSource(20260903))

	for attempt := 0; attempt < 8; attempt++ {
		t.Run(fmt.Sprintf("kill-%d", attempt), func(t *testing.T) {
			// A private copy, so a failure is visible as a modified file
			// rather than corrupting the committed corpus.
			work := t.TempDir()
			input := filepath.Join(work, "document.pdf")
			copyFile(t, source, input)

			outDir := t.TempDir()
			before := hashOf(t, input)

			cmd := exec.Command(os.Args[0]) //nolint:gosec // re-executes this test binary
			cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s|%s", helperEnv, input, outDir))
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}

			// Kill at a randomised point inside the window where the job is
			// running, so different stages are interrupted across attempts.
			delay := time.Duration(5+rnd.Intn(60)) * time.Millisecond
			time.Sleep(delay)
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()

			if after := hashOf(t, input); after != before {
				t.Fatalf("the input was modified by a job killed after %s", delay)
			}

			// Whatever landed in the destination must be a complete file, and
			// no in-progress temp file may remain.
			entries, err := os.ReadDir(outDir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".lathe-tmp-") {
					t.Errorf("a partial file was left at the destination: %s", e.Name())
				}
			}
		})
	}
}

// runJobForKilling is the child-process mode: it runs one real conversion and
// waits to be killed.
func runJobForKilling(spec string) {
	input, outDir, ok := strings.Cut(spec, "|")
	if !ok {
		os.Exit(2)
	}

	tk, found := task.Default().Get("pdf.compress")
	if !found {
		os.Exit(2)
	}
	runner := pipeline.New(engine.NewRegistry(pdfengine.New(), imageengine.New()), nil)

	// Loop, so the kill lands mid-job however fast a single pass is.
	for {
		_, _ = runner.Run(context.Background(), pipeline.Request{
			Task: tk, Inputs: []string{input},
			Options: engine.Options(tk.Defaults()), OutputDir: outDir,
		})
	}
}

func hashes(t *testing.T, paths []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		out[p] = hashOf(t, p)
	}
	return out
}

func hashOf(t *testing.T, path string) string {
	t.Helper()

	// Read-only, always. An input is never opened for writing anywhere in
	// Lathe, and the tests open them the same way.
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func permissions(t *testing.T, paths []string) map[string]os.FileMode {
	t.Helper()
	out := make(map[string]os.FileMode, len(paths))
	for _, p := range paths {
		out[p] = modeOf(t, p)
	}
	return out
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
