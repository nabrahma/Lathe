package engine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/engine"
)

// Options arrive as JSON from the UI and as strings from the CLI, so every
// accessor has to coerce rather than type-assert.
func TestOptionAccessorsCoerceAcrossSources(t *testing.T) {
	o := engine.Options{
		"fromJSON":   float64(150), // JSON has no integer type
		"fromCLI":    "150",
		"fromGo":     150,
		"boolJSON":   true,
		"boolCLI":    "true",
		"emptyText":  "   ",
		"floatAsInt": 0.75,
	}

	for _, key := range []string{"fromJSON", "fromCLI", "fromGo"} {
		if got := o.Int(key, -1); got != 150 {
			t.Errorf("Int(%q) = %d, want 150", key, got)
		}
	}
	if !o.Bool("boolJSON", false) || !o.Bool("boolCLI", false) {
		t.Error("Bool should accept both a real bool and its textual spelling")
	}
	if got := o.Float("floatAsInt", 0); got != 0.75 {
		t.Errorf("Float = %v, want 0.75", got)
	}
	if got := o.String("emptyText", "fallback"); got != "fallback" {
		t.Errorf("a whitespace-only value should fall back, got %q", got)
	}
	if got := o.Int("absent", 42); got != 42 {
		t.Errorf("missing key should return the default, got %d", got)
	}
	if o.Has("emptyText") || o.Has("absent") {
		t.Error("Has should be false for empty and missing values")
	}
}

func TestParsePagesUnderstandsHowPeopleWriteRanges(t *testing.T) {
	cases := []struct {
		spec string
		want []int
	}{
		{"", []int{1, 2, 3, 4, 5}},
		{"1", []int{1}},
		{"1-3", []int{1, 2, 3}},
		{"1-3, 5", []int{1, 2, 3, 5}},
		{"5,1", []int{1, 5}},           // sorted
		{"1-3,2-4", []int{1, 2, 3, 4}}, // overlapping ranges merge
		{"3-", []int{3, 4, 5}},         // open end
		{"-2", []int{1, 2}},            // open start
		{" 1 - 2 , 4 ", []int{1, 2, 4}},
		{"1–2", []int{1, 2}},           // en dash, as pasted from a document
		{"1 to 2", []int{1, 2}},        // written out
		{"3-1", []int{1, 2, 3}},        // reversed range
		{"1-99", []int{1, 2, 3, 4, 5}}, // clamped to the document
		{"1,,2,", []int{1, 2}},         // stray separators
	}

	for _, tc := range cases {
		got, err := engine.ParsePages(tc.spec, 5)
		if err != nil {
			t.Errorf("ParsePages(%q): %v", tc.spec, err)
			continue
		}
		if !reflect.DeepEqual([]int(got), tc.want) {
			t.Errorf("ParsePages(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}
}

func TestParsePagesRejectsWhatItCannotGuess(t *testing.T) {
	for _, spec := range []string{"abc", "1-abc", "99", "0"} {
		if _, err := engine.ParsePages(spec, 5); err == nil {
			t.Errorf("ParsePages(%q) should have failed", spec)
		}
	}
}

func TestParsePagesErrorsAreReadable(t *testing.T) {
	_, err := engine.ParsePages("99", 5)
	if err == nil {
		t.Fatal("expected an error")
	}
	// The user needs to know what the document actually contains.
	if want := "5 pages"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q should mention %q", err, want)
	}
}

func TestRegistryResolvesAndReportsMissingEngines(t *testing.T) {
	r := engine.NewRegistry()
	if _, err := r.Get("nope"); err == nil {
		t.Fatal("expected an error for an unregistered engine")
	}
	if ids := r.IDs(); len(ids) != 0 {
		t.Errorf("empty registry listed %v", ids)
	}
}
