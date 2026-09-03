package i18n_test

import (
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/i18n"
)

func TestUnknownLanguageFallsBackRatherThanBlanking(t *testing.T) {
	i18n.Use("does-not-exist")
	t.Cleanup(func() { i18n.Use("en") })

	if got := i18n.T("result.open"); got != "Open" {
		t.Errorf("got %q, want the English fallback", got)
	}
}

func TestAPartialTranslationFallsBackPerString(t *testing.T) {
	i18n.Register(i18n.Language{Code: "test", Name: "Test", Endonym: "Test"},
		i18n.Catalog{"result.open": "Ouvrir"})
	i18n.Use("test")
	t.Cleanup(func() { i18n.Use("en") })

	if got := i18n.T("result.open"); got != "Ouvrir" {
		t.Errorf("translated string: got %q", got)
	}
	// A missing string must degrade to English, never to an empty label.
	if got := i18n.T("result.another"); got != "Convert another" {
		t.Errorf("untranslated string: got %q, want the English text", got)
	}
}

func TestMissingReportsWhatATranslatorStillHasToDo(t *testing.T) {
	i18n.Register(i18n.Language{Code: "partial", Name: "Partial", Endonym: "Partial"},
		i18n.Catalog{"result.open": "x"})

	missing := i18n.Missing("partial")
	if len(missing) == 0 {
		t.Fatal("a one-string catalog should report many missing ids")
	}
	for _, id := range missing {
		if id == "result.open" {
			t.Error("a translated id was reported as missing")
		}
	}
}

func TestUnknownIDReturnsTheIDRatherThanNothing(t *testing.T) {
	if got := i18n.T("no.such.id"); got != "no.such.id" {
		t.Errorf("got %q, want the id itself so the gap is visible", got)
	}
}

func TestEnglishStringsAreNotShouting(t *testing.T) {
	// Caps are applied by the interface as chrome styling, so the catalogue
	// itself must stay in sentence case for locales where caps read badly.
	for _, id := range []string{
		"result.open", "task.choose", "error.retry", "settings.download",
	} {
		s := i18n.T(id)
		if s == strings.ToUpper(s) && len(s) > 3 {
			t.Errorf("%s is stored uppercase: %q", id, s)
		}
	}
}
