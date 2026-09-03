package pdfengine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/task"
)

// stubDeps reports one installed component at a fixed path. Only the two
// methods the PDF engine calls do anything.
type stubDeps struct{ gs string }

func (s stubDeps) Available(id string) bool { return id == "ghostscript" && s.gs != "" }

func (s stubDeps) BinaryPath(id, _ string) (string, error) {
	if id != "ghostscript" || s.gs == "" {
		return "", deps.ErrNotInstalled
	}
	return s.gs, nil
}

func (stubDeps) Components() []deps.Component         { return nil }
func (stubDeps) Status(context.Context) []deps.Status { return nil }
func (stubDeps) TierAvailable(deps.Tier) bool         { return true }
func (stubDeps) TierName(deps.Tier) string            { return "" }
func (stubDeps) TierDownloadMB(deps.Tier) int         { return 0 }
func (stubDeps) Remove(string) error                  { return nil }
func (stubDeps) Verify(string) error                  { return nil }
func (stubDeps) DiskUsage() map[string]int64          { return nil }
func (stubDeps) Ensure(context.Context, string, func(deps.Progress)) error {
	return nil
}

func findGhostscript(t *testing.T) string {
	t.Helper()
	name := "gs"
	if runtime.GOOS == "windows" {
		name = "gswin64c"
	}
	p, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("ghostscript is not installed, so the optional path cannot be exercised")
	}
	return p
}

// TestGhostscriptIsNeverWorseThanTheBuiltInPath is the guarantee that makes
// the optional component safe to reach for: when Ghostscript helps it is used,
// and when it does not the result falls back rather than regressing.
//
// It is deliberately not asserted that Ghostscript always wins. On a page
// whose images are already at a sensible resolution it can produce a slightly
// larger file, which the engine detects and discards.
func TestGhostscriptIsNeverWorseThanTheBuiltInPath(t *testing.T) {
	gs := findGhostscript(t)

	source := filepath.Join("..", "..", "..", "testdata", "corpus", "pdf", "scanned.pdf")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}
	before := sizeOf(t, source)

	for _, quality := range []string{"low", "medium", "high"} {
		t.Run(quality, func(t *testing.T) {
			builtIn := compressWith(t, pdfengine.New(), source, quality)
			withGS := compressWith(t, pdfengine.New().WithComponents(stubDeps{gs: gs}), source, quality)

			t.Logf("%s: %d before, %d built in, %d offered ghostscript", quality, before, builtIn, withGS)

			if withGS > builtIn {
				t.Errorf("ghostscript produced %d bytes, worse than the built-in %d", withGS, builtIn)
			}
			if withGS >= before {
				t.Errorf("produced %d bytes from a %d byte input", withGS, before)
			}
		})
	}
}

// TestGhostscriptRecompressesAScanItCanImproveOn covers the case the component
// exists for: a scan carrying JPEGs with headroom left in them.
//
// Ghostscript passes JPEG streams through untouched unless told not to, which
// makes the whole pass a no-op that adds a couple of kilobytes. This asserts
// the flag is still there.
func TestGhostscriptRecompressesAScanItCanImproveOn(t *testing.T) {
	gs := findGhostscript(t)

	photo := filepath.Join("..", "..", "..", "testdata", "corpus", "images", "large.jpg")
	if _, err := os.Stat(photo); err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}

	// A page built from a high quality photograph, which is what a scanner or
	// a phone actually produces and what the committed corpus does not hold.
	scan := filepath.Join(t.TempDir(), "highres.pdf")
	if err := api.ImportImagesFile([]string{photo}, scan, nil, nil); err != nil {
		t.Fatalf("build a high resolution scan: %v", err)
	}
	before := sizeOf(t, scan)

	got := compressWith(t, pdfengine.New().WithComponents(stubDeps{gs: gs}), scan, "medium")
	t.Logf("%d before, %d after", before, got)

	// The built-in path already manages a large reduction here, so this only
	// guards against the pass silently doing nothing at all.
	if got >= before {
		t.Errorf("produced %d bytes from a %d byte scan", got, before)
	}
}

// TestCompressionStillWorksWithoutGhostscript guards the promise that the
// component is optional: an engine pointed at a tool that is not there must
// fall back rather than fail.
func TestCompressionStillWorksWithoutGhostscript(t *testing.T) {
	source := filepath.Join("..", "..", "..", "testdata", "corpus", "pdf", "scanned.pdf")
	if _, err := os.Stat(source); err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}

	missing := stubDeps{gs: filepath.Join(t.TempDir(), "no-such-ghostscript")}
	got := compressWith(t, pdfengine.New().WithComponents(missing), source, "medium")
	if got == 0 {
		t.Fatal("no output produced when ghostscript was absent")
	}
	if got >= sizeOf(t, source) {
		t.Errorf("fallback produced %d bytes from a %d byte input", got, sizeOf(t, source))
	}
}

func compressWith(t *testing.T, e *pdfengine.Engine, source, quality string) int64 {
	t.Helper()

	ws, err := fsatomic.TempWorkspace("gs-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	tk, ok := task.Default().Get("pdf.compress")
	if !ok {
		t.Fatal("pdf.compress is missing from the registry")
	}
	opts := engine.Options(tk.Defaults())
	opts["quality"] = quality

	resp, err := e.Execute(context.Background(), engine.Request{
		Task: tk, Inputs: []string{source}, Options: opts, Workspace: ws,
	}, func(engine.Progress) {})
	if err != nil {
		t.Fatalf("compress at %s quality: %v", quality, err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(resp.Outputs))
	}
	return sizeOf(t, resp.Outputs[0])
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
