package pdfengine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/task"
)

// The bookmark toggle was offered on the task screen and read by nothing, so
// turning it off changed the file not at all. These pin both halves: that it
// does something, and that it does nothing when it is off.

func TestMergeAddsABookmarkPerFileWhenAsked(t *testing.T) {
	out := mergeCorpus(t, true)

	marks, err := bookmarksOf(out)
	if err != nil {
		t.Fatalf("read bookmarks: %v", err)
	}
	if len(marks) != 2 {
		t.Errorf("got %d bookmarks for 2 merged files, want 2", len(marks))
	}
}

func TestMergeLeavesBookmarksOutWhenNotAsked(t *testing.T) {
	out := mergeCorpus(t, false)

	marks, err := bookmarksOf(out)
	if err != nil {
		// pdfcpu reports an outline-free document as an error on some
		// versions, which is the same answer as an empty list.
		return
	}
	if len(marks) != 0 {
		t.Errorf("got %d bookmarks with the option off, want none", len(marks))
	}
}

// TestMergeKeepsTheOrderItWasGiven is the promise the reorder handles on the
// task screen make. If merge sorted its inputs, dragging a file up the list
// would change the screen and not the document.
func TestMergeKeepsTheOrderItWasGiven(t *testing.T) {
	single := filepath.Join("..", "..", "..", "testdata", "corpus", "pdf", "single-page.pdf")
	five := filepath.Join("..", "..", "..", "testdata", "corpus", "pdf", "five-page.pdf")

	// Five pages first, then one: if the order survived, page six is the last
	// page and the total is six either way, so the count alone proves nothing.
	out := mergeFiles(t, []string{five, single}, true)

	n, err := api.PageCountFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("merged to %d pages, want 6", n)
	}

	// The bookmarks are created per input in the order they were merged, so
	// they record the order the document was actually built in.
	marks, err := bookmarksOf(out)
	if err != nil {
		t.Skipf("bookmarks unavailable, so order cannot be checked here: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("got %d bookmarks, want 2", len(marks))
	}
	if marks[0].PageFrom != 1 {
		t.Errorf("first bookmark starts at page %d, want 1", marks[0].PageFrom)
	}
	// The five-page file was given first, so the second file starts at page 6.
	if marks[1].PageFrom != 6 {
		t.Errorf("second bookmark starts at page %d, want 6, so the inputs were reordered",
			marks[1].PageFrom)
	}
}

func mergeCorpus(t *testing.T, bookmarks bool) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "testdata", "corpus", "pdf")
	return mergeFiles(t,
		[]string{filepath.Join(dir, "single-page.pdf"), filepath.Join(dir, "five-page.pdf")},
		bookmarks)
}

func mergeFiles(t *testing.T, inputs []string, bookmarks bool) string {
	t.Helper()

	for _, in := range inputs {
		if _, err := api.PageCountFile(in); err != nil {
			t.Skipf("corpus missing (run: make corpus): %v", err)
		}
	}

	ws, err := fsatomic.TempWorkspace("merge-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	tk, ok := task.Default().Get("pdf.merge")
	if !ok {
		t.Fatal("pdf.merge is missing from the registry")
	}
	opts := engine.Options(tk.Defaults())
	opts["bookmarks"] = bookmarks

	resp, err := pdfengine.New().Execute(context.Background(), engine.Request{
		Task: tk, Inputs: inputs, Options: opts, Workspace: ws,
	}, func(engine.Progress) {})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(resp.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(resp.Outputs))
	}
	return resp.Outputs[0]
}

func bookmarksOf(path string) ([]pdfcpu.Bookmark, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return api.Bookmarks(f, nil)
}
