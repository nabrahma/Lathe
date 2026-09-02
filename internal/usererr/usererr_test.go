package usererr_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/usererr"
)

func TestTranslateMapsRealEngineOutput(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want usererr.Code
	}{
		{"pdfcpu encrypted", "pdfcpu: this file is encrypted", usererr.CodePasswordRequired},
		{"pdfcpu wrong password", "pdfcpu: please provide the correct password", usererr.CodePasswordWrong},
		{"ffmpeg damaged", "ffmpeg: Invalid data found when processing input", usererr.CodeCorruptInput},
		{"tesseract blank", "Empty page!! Estimating resolution as 300", usererr.CodeNoTextFound},
		{"libreoffice", "soffice: Error: source file could not be loaded", usererr.CodeCorruptInput},
		{"oom kill", "exit status 137", usererr.CodeOutOfMemory},
		{"disk full", "write /out.pdf: no space left on device", usererr.CodeDiskFull},
		{"windows lock", "rename: The process cannot access the file because it is being used by another process.",
			usererr.CodeFileLocked},
		{"permission", "open /out.pdf: permission denied", usererr.CodeNotWritable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := usererr.Translate(errors.New(tc.raw))
			if got.Code != tc.want {
				t.Fatalf("code %q, want %q (message: %q)", got.Code, tc.want, got.Message)
			}
			assertPresentable(t, got)
		})
	}
}

func TestUnmappedErrorsStillGetAnHonestMessageAndAWayOut(t *testing.T) {
	got := usererr.Translate(errors.New("some entirely novel failure from a future engine"))

	if got.Code != usererr.CodeUnknown {
		t.Fatalf("code %q, want unknown", got.Code)
	}
	if !hasAction(got, usererr.ActionCopyDetails) {
		t.Error("an unmapped error must offer a way to copy the technical detail")
	}
	if got.Detail == "" {
		t.Error("the raw text should be preserved as copyable detail")
	}
	if strings.Contains(got.Message, "novel failure") {
		t.Error("raw engine text leaked into the message shown in the main flow")
	}
}

// Every message the user can see has to survive these rules, because a
// translation table degrades one careless entry at a time.
func TestEveryMappedMessageIsPresentable(t *testing.T) {
	samples := []string{
		"pdfcpu: this file is encrypted", "Empty page!!", "exit status 137",
		"no space left on device", "sharing violation", "decoder not found",
		"failed loading language 'hin'", "cannot allocate memory",
	}
	for _, raw := range samples {
		assertPresentable(t, usererr.Translate(errors.New(raw)))
	}
}

func TestAlreadyTranslatedErrorsAreNotRewrapped(t *testing.T) {
	original := usererr.New(usererr.CodeDiskFull, "There isn't enough space.", usererr.ActionFreeSpace)

	if got := usererr.Translate(original); got != original {
		t.Fatal("Translate should return an already-translated error unchanged")
	}

	wrapped := usererr.Wrap(original, usererr.CodeUnknown, "outer")
	var target *usererr.Error
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should reach a wrapped user error")
	}
}

func TestTranslateOfNilIsNil(t *testing.T) {
	if got := usererr.Translate(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func assertPresentable(t *testing.T, e *usererr.Error) {
	t.Helper()

	if e.Message == "" {
		t.Fatal("empty message")
	}
	if len(e.Actions) == 0 {
		t.Errorf("%q offers no next action", e.Message)
	}

	// Errors are sentence case, never uppercase: caps are chrome.
	if e.Message == strings.ToUpper(e.Message) {
		t.Errorf("message is uppercase: %q", e.Message)
	}
	if first := rune(e.Message[0]); first < 'A' || first > 'Z' {
		t.Errorf("message does not start with a capital: %q", e.Message)
	}
	if !strings.HasSuffix(e.Message, ".") {
		t.Errorf("message is not a complete sentence: %q", e.Message)
	}

	// No jargon, no library names, no exit codes in what the user reads.
	for _, banned := range []string{
		"exit status", "errno", "stderr", "stack", "nil pointer",
		"pdfcpu", "ffmpeg", "tesseract", "soffice", "goroutine", "0x",
	} {
		if strings.Contains(strings.ToLower(e.Message), banned) {
			t.Errorf("message leaks %q: %q", banned, e.Message)
		}
	}
}

func hasAction(e *usererr.Error, a usererr.Action) bool {
	for _, got := range e.Actions {
		if got == a {
			return true
		}
	}
	return false
}
