package matrix_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/ocrengine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
)

// The OCR benchmark measures character-level accuracy against the ground truth
// committed alongside the corpus, and writes the result to docs/evidence so
// every figure quoted in the README traces to a file.
//
// It skips rather than fails when Tesseract is absent: OCR is a detected
// component, and a contributor without it should still get a green test run.

// baselines are the floors each corpus subset must clear: the measured figure
// less the two percentage points of regression the design allows. They come
// from a real run, recorded in docs/evidence/ocr-accuracy.json.
//
// These are synthetic renderings of digital text, so they are an easier target
// than a photograph of physically printed paper. The README says so rather
// than quoting them as though they predicted real-world accuracy. What the
// benchmark is genuinely good at is catching a regression in the preprocessing
// chain, which is what it exists for.
var baselines = map[string]float64{
	"clean":  0.98,
	"photo":  0.98,
	"lowres": 0.97,
}

type accuracyResult struct {
	Subset   string  `json:"subset"`
	File     string  `json:"file"`
	Accuracy float64 `json:"accuracy"`
}

func TestOCRAccuracy(t *testing.T) {
	manager, err := deps.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Available("tesseract") {
		t.Skip("Tesseract is not installed on this machine; see docs/BUNDLING.md")
	}

	dir := filepath.Join(corpus, "ocr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("OCR corpus missing (run: make corpus): %v", err)
	}

	runner := pipeline.New(engine.NewRegistry(ocrengine.New(manager)), nil)
	tk, ok := task.Default().Get("text.from-image")
	if !ok {
		t.Fatal("text.from-image is missing from the registry")
	}

	var results []accuracyResult
	bySubset := map[string][]float64{}

	for _, entry := range entries {
		name := entry.Name()
		subset := subsetOf(name)
		if subset == "" {
			continue
		}

		truth, err := os.ReadFile(filepath.Join(dir, passageOf(name)+".txt"))
		if err != nil {
			t.Fatalf("no ground truth for %s: %v", name, err)
		}

		outDir := t.TempDir()
		opts := engine.Options(tk.Defaults())
		res, err := runner.Run(context.Background(), pipeline.Request{
			Task: tk, Inputs: []string{filepath.Join(dir, name)}, Options: opts, OutputDir: outDir,
		})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}

		got, err := os.ReadFile(res.Outputs[0])
		if err != nil {
			t.Fatal(err)
		}

		accuracy := characterAccuracy(string(truth), string(got))
		results = append(results, accuracyResult{Subset: subset, File: name, Accuracy: accuracy})
		bySubset[subset] = append(bySubset[subset], accuracy)
		t.Logf("%-28s %s  %.1f%%", name, subset, accuracy*100)
	}

	if len(results) == 0 {
		t.Skip("no OCR corpus files found")
	}

	summary := map[string]float64{}
	for subset, scores := range bySubset {
		summary[subset] = mean(scores)
		floor, known := baselines[subset]
		if known && summary[subset] < floor {
			t.Errorf("%s accuracy regressed to %.1f%%, below the %.0f%% baseline",
				subset, summary[subset]*100, floor*100)
		}
	}
	writeEvidence(t, results, summary)
}

// subsetOf classifies a corpus file by the capture condition it represents.
func subsetOf(name string) string {
	switch {
	case strings.HasSuffix(name, "-clean.png"):
		return "clean"
	case strings.HasSuffix(name, "-photo.jpg"):
		return "photo"
	case strings.HasSuffix(name, "-lowres.jpg"):
		return "lowres"
	default:
		return ""
	}
}

func passageOf(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	for _, suffix := range []string{"-clean", "-photo", "-lowres"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base
}

// characterAccuracy is 1 minus the normalised edit distance, which is the
// standard OCR measure. Whitespace is collapsed first: line wrapping differences
// are a layout artefact, not a recognition error.
func characterAccuracy(want, got string) float64 {
	want = normalise(want)
	got = normalise(got)

	if want == "" {
		return 0
	}
	distance := levenshtein([]rune(want), []rune(got))
	accuracy := 1 - float64(distance)/float64(len([]rune(want)))
	if accuracy < 0 {
		return 0
	}
	return accuracy
}

func normalise(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// levenshtein uses two rows rather than a full matrix: the passages are a few
// hundred characters and the full matrix is pure waste.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min3(current[j-1]+1, previous[j]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// writeEvidence records the measurement so the README can cite a file rather
// than a memory.
func writeEvidence(t *testing.T, results []accuracyResult, summary map[string]float64) {
	t.Helper()

	dir := filepath.Join("..", "..", "docs", "evidence")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Logf("could not write evidence: %v", err)
		return
	}

	sort.Slice(results, func(i, j int) bool { return results[i].File < results[j].File })
	payload := struct {
		Summary map[string]float64 `json:"summaryBySubset"`
		Files   []accuracyResult   `json:"files"`
	}{summary, results}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(dir, "ocr-accuracy.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Logf("could not write evidence: %v", err)
		return
	}

	var lines []string
	for _, subset := range []string{"clean", "photo", "lowres"} {
		if v, ok := summary[subset]; ok {
			lines = append(lines, fmt.Sprintf("%-8s %.1f%%", subset, v*100))
		}
	}
	t.Logf("wrote %s\n%s", path, strings.Join(lines, "\n"))
}
