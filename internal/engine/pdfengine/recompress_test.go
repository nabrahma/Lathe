package pdfengine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/task"
)

const corpus = "../../../testdata/corpus"

// Compressing a scan is the reason Compress PDF exists, so the sizes it
// produces are asserted rather than assumed. The optimiser alone is lossless
// and would leave a scan essentially untouched.
func TestCompressingAScanActuallyShrinksIt(t *testing.T) {
	in := filepath.Join(corpus, "pdf", "scanned.pdf")
	before, err := os.Stat(in)
	if err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}

	tk, ok := task.Default().Get("pdf.compress")
	if !ok {
		t.Fatal("pdf.compress is missing from the registry")
	}

	sizes := map[string]int64{}
	for _, quality := range []string{"low", "medium", "high"} {
		ws, err := fsatomic.TempWorkspace("compress-test")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ws.Close() })

		opts := engine.Options(tk.Defaults())
		opts["quality"] = quality

		res, err := pdfengine.New().Execute(context.Background(), engine.Request{
			Task: tk, Inputs: []string{in}, Options: opts, Workspace: ws,
		}, func(engine.Progress) {})
		if err != nil {
			t.Fatalf("%s: %v", quality, err)
		}

		info, err := os.Stat(res.Outputs[0])
		if err != nil {
			t.Fatal(err)
		}
		sizes[quality] = info.Size()
		t.Logf("%-7s %d -> %d bytes (%d%% smaller)", quality, before.Size(), info.Size(),
			100-(info.Size()*100/before.Size()))

		if info.Size() >= before.Size() {
			t.Errorf("%s quality produced a file no smaller than the original", quality)
		}
	}

	// Lower quality must mean a smaller file, or the setting is decorative.
	if sizes["low"] >= sizes["medium"] || sizes["medium"] >= sizes["high"] {
		t.Errorf("quality settings are not ordered by size: %v", sizes)
	}
}

// A PDF that is already tight must come back whole rather than larger, and the
// user must be told why nothing changed.
func TestCompressingAnAlreadySmallPDFIsHonest(t *testing.T) {
	in := filepath.Join(corpus, "pdf", "single-page.pdf")
	before, err := os.Stat(in)
	if err != nil {
		t.Skipf("corpus missing (run: make corpus): %v", err)
	}

	tk, _ := task.Default().Get("pdf.compress")
	ws, err := fsatomic.TempWorkspace("compress-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })

	opts := engine.Options(tk.Defaults())
	opts["quality"] = "high"

	res, err := pdfengine.New().Execute(context.Background(), engine.Request{
		Task: tk, Inputs: []string{in}, Options: opts, Workspace: ws,
	}, func(engine.Progress) {})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(res.Outputs[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > before.Size() {
		t.Errorf("compression produced a larger file: %d -> %d", before.Size(), info.Size())
	}
	if info.Size() == before.Size() && len(res.Notes) == 0 {
		t.Error("a file that could not be improved should say so rather than looking like a no-op")
	}
}
