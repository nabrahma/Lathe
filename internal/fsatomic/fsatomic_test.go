package fsatomic_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/fsatomic"
)

func TestWriteFileProducesTheWholeFileOrNothing(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out.pdf")
	want := "complete contents"

	if err := fsatomic.WriteFile(dst, func(w io.Writer) error {
		_, err := io.WriteString(w, want)
		return err
	}, 0); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteFileLeavesNothingBehindWhenTheWriterFails(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.pdf")
	sentinel := errors.New("engine gave up half way")

	err := fsatomic.WriteFile(dst, func(w io.Writer) error {
		_, _ = io.WriteString(w, "partial")
		return sentinel
	}, 0)
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want the writer's error", err)
	}

	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Error("a partial file was published to the destination")
	}
	assertNoTempLeftovers(t, dir)
}

func TestWriteFileDoesNotDisturbAnExistingFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "existing.pdf")
	original := "the user's original bytes"
	if err := os.WriteFile(dst, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = fsatomic.WriteFile(dst, func(w io.Writer) error {
		_, _ = io.WriteString(w, "replacement")
		return errors.New("failed")
	}, 0)

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("existing file was modified: got %q, want %q", got, original)
	}
	assertNoTempLeftovers(t, dir)
}

func TestUniquePathNeverOverwrites(t *testing.T) {
	dir := t.TempDir()

	first := fsatomic.UniquePath(dir, "report", ".pdf")
	if filepath.Base(first) != "report.pdf" {
		t.Fatalf("first name %q, want report.pdf", filepath.Base(first))
	}
	touch(t, first)

	second := fsatomic.UniquePath(dir, "report", "pdf") // extension without a dot
	if filepath.Base(second) != "report (1).pdf" {
		t.Fatalf("second name %q, want report (1).pdf", filepath.Base(second))
	}
	touch(t, second)

	third := fsatomic.UniquePath(dir, "report", ".pdf")
	if filepath.Base(third) != "report (2).pdf" {
		t.Fatalf("third name %q, want report (2).pdf", filepath.Base(third))
	}
}

func TestWorkspacePathContainsHostileNames(t *testing.T) {
	ws, err := fsatomic.TempWorkspace("unit-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	hostile := []string{
		"../../escape.pdf",
		`..\..\escape.pdf`,
		"/etc/passwd",
		"",
		"   ",
		"con",
		strings.Repeat("a", 500) + ".pdf",
	}
	for _, name := range hostile {
		got := ws.Path(name)
		rel, err := filepath.Rel(ws.Dir(), got)
		if err != nil {
			t.Errorf("%q produced an unrelated path %q", name, got)
			continue
		}
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			t.Errorf("%q escaped the workspace: %q", name, got)
		}
	}
}

func TestWorkspaceCloseIsIdempotent(t *testing.T) {
	ws, err := fsatomic.TempWorkspace("unit-test")
	if err != nil {
		t.Fatal(err)
	}
	dir := ws.Dir()

	if err := ws.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := ws.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("workspace directory survived Close")
	}
}

func TestCleanOrphansRemovesOnlyStaleWorkspaces(t *testing.T) {
	stale, err := fsatomic.TempWorkspace("stale")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale.Dir(), old, old); err != nil {
		t.Skipf("cannot backdate directory on this filesystem: %v", err)
	}

	fresh, err := fsatomic.TempWorkspace("fresh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })

	if _, err := fsatomic.CleanOrphans(time.Hour); err != nil {
		t.Fatalf("clean: %v", err)
	}
	if _, err := os.Stat(stale.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Error("stale workspace survived cleanup")
	}
	if _, err := os.Stat(fresh.Dir()); err != nil {
		t.Error("fresh workspace was wrongly removed")
	}
}

func TestPublishMovesAResultWithoutTouchingTheInput(t *testing.T) {
	src := filepath.Join(t.TempDir(), "engine-output.pdf")
	dst := filepath.Join(t.TempDir(), "final.pdf")
	if err := os.WriteFile(src, []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := fsatomic.Publish(src, dst, 0); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "result" {
		t.Fatalf("published contents %q, err %v", got, err)
	}
}

func TestCheckWritableRejectsAMissingFolder(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := fsatomic.CheckWritable(missing); err == nil {
		t.Fatal("expected an error for a missing output folder")
	}

	if err := fsatomic.CheckWritable(t.TempDir()); err != nil {
		t.Fatalf("a normal temp dir should be writable: %v", err)
	}
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertNoTempLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".lathe-tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// The writability probe is a file in the user's own output folder, so what it
// leaves behind matters. These pin the three things that can go wrong: debris
// in the normal case, debris surviving a killed run, and a sweep that reaches
// past its own files into somebody's data.

const checkPrefix = ".lathe-check-"

func TestCheckWritableLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	if err := fsatomic.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the probe left %d file(s) in the output folder", len(entries))
	}
}

// A run killed between creating the probe and removing it cannot clean up after
// itself, because the kill is not deliverable to the program. The next job in
// that folder has to do it instead.
func TestCheckWritableClearsProbesLeftByAKilledRun(t *testing.T) {
	dir := t.TempDir()
	abandoned := filepath.Join(dir, checkPrefix+"651484437")
	writeStaleFile(t, abandoned, time.Now().Add(-time.Hour))

	if err := fsatomic.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an abandoned probe survived the next job: %v", err)
	}
}

// Two jobs can start in the same folder at once. Sweeping a probe that another
// process is still holding would make that job report the folder unwritable, so
// only files old enough to be debris are touched.
func TestCheckWritableKeepsAProbeFromAConcurrentJob(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, checkPrefix+"inflight")
	writeStaleFile(t, live, time.Now())

	if err := fsatomic.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Errorf("a probe belonging to a job starting now was deleted: %v", err)
	}
}

// The sweep must never reach an in-progress result. Those carry the other
// prefix and are half of somebody's document, however old they are.
func TestCheckWritableNeverSweepsAPartialResult(t *testing.T) {
	dir := t.TempDir()
	partial := filepath.Join(dir, ".lathe-tmp-halfadocument")
	writeStaleFile(t, partial, time.Now().Add(-24*time.Hour))

	if err := fsatomic.CheckWritable(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(partial); err != nil {
		t.Errorf("an in-progress result was swept away as debris: %v", err)
	}
}

func writeStaleFile(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}
