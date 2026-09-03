// Package i18n holds the user-facing strings the Go side produces.
//
// It exists from the start because retrofitting translation is painful: every
// message has to be found, extracted and re-checked, and the ones that get
// missed are always the error messages nobody reads until something breaks.
//
// English ships in v1. The structure is here so a translator only has to
// supply a map, not go hunting through the source.
package i18n

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// DefaultLanguage is the fallback for any string a translation omits.
const DefaultLanguage = "en"

// Catalog is one language's strings, keyed by message id.
type Catalog map[string]string

var (
	mu        sync.RWMutex
	current   = DefaultLanguage
	catalogs  = map[string]Catalog{DefaultLanguage: english}
	languages = []Language{{Code: "en", Name: "English", Endonym: "English"}}
)

// Language describes a translation for the settings screen.
type Language struct {
	Code string `json:"code"`
	// Name is the language in English; Endonym is the language in itself,
	// which is what a speaker of it will actually scan for in a list.
	Name    string `json:"name"`
	Endonym string `json:"endonym"`
}

// Register adds a translation. Registering an existing code replaces it.
func Register(lang Language, catalog Catalog) {
	mu.Lock()
	defer mu.Unlock()

	catalogs[lang.Code] = catalog
	for i, existing := range languages {
		if existing.Code == lang.Code {
			languages[i] = lang
			return
		}
	}
	languages = append(languages, lang)
	sort.Slice(languages, func(i, j int) bool { return languages[i].Name < languages[j].Name })
}

// Use selects a language. An unknown code falls back to English rather than
// leaving the interface blank.
func Use(code string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := catalogs[code]; ok {
		current = code
		return
	}
	current = DefaultLanguage
}

// Languages lists what is available.
func Languages() []Language {
	mu.RLock()
	defer mu.RUnlock()
	return append([]Language(nil), languages...)
}

// T returns the string for an id, falling back to English and then to the id
// itself. A missing translation degrades to English, never to an empty label.
func T(id string) string {
	mu.RLock()
	defer mu.RUnlock()

	if s, ok := catalogs[current][id]; ok && s != "" {
		return s
	}
	if s, ok := catalogs[DefaultLanguage][id]; ok {
		return s
	}
	return id
}

// Tf is T with formatting applied.
func Tf(id string, args ...any) string {
	return fmt.Sprintf(T(id), args...)
}

// Missing reports which ids a translation has not covered yet, which is what a
// translator needs to see to know when they are done.
func Missing(code string) []string {
	mu.RLock()
	defer mu.RUnlock()

	catalog := catalogs[code]
	var out []string
	for id := range english {
		if s, ok := catalog[id]; !ok || strings.TrimSpace(s) == "" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// english is the source of truth. Every other catalog is measured against it.
//
// Ids are dotted and describe where the string appears, so a translator working
// without the app in front of them still has some context.
var english = Catalog{
	"app.name":    "Lathe",
	"app.tagline": "Convert, compress and read files. Offline.",

	"home.search":         "What do you want to do?",
	"home.filtered":       "Showing what you can do with it",
	"home.clear":          "Clear",
	"home.nothingFound":   "Nothing here matches that. Try a different word.",
	"home.group.pdf":      "PDF",
	"home.group.image":    "Images",
	"home.group.text":     "Text and reading",
	"home.group.doc":      "Documents",
	"home.group.media":    "Video and audio",
	"home.badge.needs":    "Needs setup",
	"home.badge.download": "+%d MB",

	"task.back":         "Back",
	"task.drop":         "Drop files here",
	"task.choose":       "Choose files",
	"task.addMore":      "Add more",
	"task.moreOptions":  "More options",
	"task.fewerOptions": "Fewer options",
	"task.saveBeside":   "Save beside the original",
	"task.changeFolder": "Change folder",
	"task.oneAtATime":   "One file at a time for this one",
	"task.atLeast":      "At least %d files",
	"task.oneOrSeveral": "One or several",

	"file.actually":  "Actually %s",
	"file.protected": "Protected",
	"file.notHere":   "Not supported here",
	"file.remove":    "Remove %s",

	"progress.working": "Working",
	"progress.cancel":  "Cancel",

	"result.done":       "Done",
	"result.open":       "Open",
	"result.showFolder": "Show in folder",
	"result.another":    "Convert another",
	"result.smaller":    "%d%% smaller",
	"result.larger":     "%d%% larger",
	"result.same":       "same size",

	"error.title":     "Didn't work",
	"error.details":   "Technical details",
	"error.copy":      "Copy details",
	"error.retry":     "Try again",
	"error.otherFile": "Choose another file",
	"error.cancelled": "The job was cancelled. Nothing was written, and your original file is exactly as it was.",
	"error.startup":   "Lathe couldn't start up properly. Restarting usually fixes it.",

	"quit.title":  "Still working",
	"quit.stay":   "Keep working",
	"quit.quit":   "Quit anyway",
	"quit.oneJob": "One job is still running. Quitting now cancels it; your original files are untouched either way.",
	"quit.jobs":   "%d jobs are still running. Quitting now cancels them; your original files are untouched either way.",

	"settings.title":       "Settings",
	"settings.components":  "Components",
	"settings.converting":  "Converting",
	"settings.privacy":     "Privacy",
	"settings.concurrency": "Jobs at the same time",
	"settings.enhance":     "Enhance images before reading text",
	"settings.shell":       "Add \"Convert with Lathe\" to the right-click menu",
	"settings.updates":     "Check for updates",
	"settings.download":    "Download",
	"settings.remove":      "Remove",
	"settings.installed":   "Installed",
}
