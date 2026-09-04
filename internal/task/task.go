// Package task defines what Lathe can do, as a registry of user-facing
// operations rather than a matrix of format pairs.
//
// A task is the unit the home screen shows, the CLI exposes as a subcommand,
// and the pipeline executes. Adding a capability means adding a Task here and
// teaching one engine to handle it; nothing else changes.
package task

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nabrahma/lathe/internal/detect"
)

// Category groups tasks the way the home screen groups them.
type Category string

// The five task groups.
const (
	CategoryPDF      Category = "pdf"
	CategoryImage    Category = "image"
	CategoryText     Category = "text"
	CategoryDocument Category = "document"
	CategoryMedia    Category = "media"
)

// Tier is how much has to be downloaded before a task can run.
type Tier uint8

// Tiers, in install order. Core and Bundled ship with the app.
//
// TierEnhance is the odd one: no task requires it. It holds components that
// make a task better when they happen to be present, so the work still runs
// without them rather than stopping to ask for a download.
const (
	TierCore Tier = iota
	TierBundled
	TierMedia
	TierOffice
	TierEnhance
)

// String renders a tier for logs and the settings screen.
func (t Tier) String() string {
	switch t {
	case TierCore:
		return "core"
	case TierBundled:
		return "bundled"
	case TierMedia:
		return "media"
	case TierOffice:
		return "office"
	case TierEnhance:
		return "enhance"
	default:
		return "unknown"
	}
}

// OptionType determines which control the UI renders for an option.
type OptionType string

// The control kinds a task option can request.
const (
	OptionChoice    OptionType = "choice"
	OptionRange     OptionType = "range"
	OptionToggle    OptionType = "toggle"
	OptionText      OptionType = "text"
	OptionPageRange OptionType = "pagerange"
	OptionPassword  OptionType = "password"
)

// Choice is one entry in a Choice option. At most four are allowed on a
// primary option; anything longer belongs behind "More options".
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
	// Hint is one short line under the label, in sentence case.
	Hint string `json:"hint,omitempty"`
}

// Option is a single control on a task screen.
type Option struct {
	ID      string     `json:"id"`
	Label   string     `json:"label"`
	Type    OptionType `json:"type"`
	Default any        `json:"default"`
	Choices []Choice   `json:"choices,omitempty"`

	// Min, Max and Step apply to Range options.
	Min  float64 `json:"min,omitempty"`
	Max  float64 `json:"max,omitempty"`
	Step float64 `json:"step,omitempty"`

	// Advanced options are hidden until the user asks for them. A task screen
	// shows at most MaxPrimaryOptions non-advanced controls.
	Advanced bool `json:"advanced"`

	// Placeholder is shown in an empty Text or Password field.
	Placeholder string `json:"placeholder,omitempty"`

	// Help is one sentence saying what the control actually does, shown when
	// the pointer rests on it.
	//
	// Most controls do not have one and should not. It is for labels that name
	// a concept rather than an effect, which is where jargon hides: "Add a
	// bookmark per file" is exact and tells someone who has never opened a PDF
	// outline nothing at all. A label that already says what happens, such as
	// "Read scanned pages too", is finished, and explaining it again is noise
	// that trains people to ignore the explanations that matter.
	Help string `json:"help,omitempty"`
}

// MaxPrimaryOptions caps how many controls a task screen may show before the
// user opens "More options". Every extra control makes the app harder to use
// for the people it is aimed at.
const MaxPrimaryOptions = 3

// Task is one user-facing operation.
type Task struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    Category `json:"category"`
	// Icon is a name from the frontend icon set, not a glyph.
	Icon string `json:"icon"`
	// Verb is the primary button label: "COMPRESS", never "SUBMIT".
	Verb string `json:"verb"`

	// Accepts lists the input categories the task can handle.
	Accepts   []detect.Category `json:"accepts"`
	MinInputs int               `json:"minInputs"`
	// MaxInputs of 0 means unlimited, as merge requires.
	MaxInputs int `json:"maxInputs"`

	Options      []Option `json:"options"`
	RequiredTier Tier     `json:"requiredTier"`
	Engine       string   `json:"engine"`

	// CLIName is the subcommand for lathe-cli. Empty means ID with dots
	// replaced by dashes.
	CLIName string `json:"cliName,omitempty"`

	// OrderMatters marks the tasks that build one document out of several
	// files, where the order they were given in is the order they appear in
	// the result. Those are the only ones where rearranging the list changes
	// anything, so they are the only ones offered handles to do it with.
	OrderMatters bool `json:"orderMatters"`

	// ShrinksFile marks the tasks whose whole purpose is to make a file
	// smaller, which are the only ones where a before-and-after size is worth
	// reporting.
	//
	// Everywhere else the comparison is noise at best and a lie at worst.
	// Merging two PDFs of 171 kB into one of 131 kB is not a 24 percent saving
	// on anything the user asked for; it is what happens when two documents
	// stop repeating each other's fonts, and reporting it as a saving invites
	// the reader to wonder what was thrown away.
	ShrinksFile bool `json:"shrinksFile"`
}

// Command is the CLI subcommand name for the task.
func (t Task) Command() string {
	if t.CLIName != "" {
		return t.CLIName
	}
	return strings.ReplaceAll(t.ID, ".", "-")
}

// AcceptsCategory reports whether the task can take a file of this category.
func (t Task) AcceptsCategory(c detect.Category) bool {
	for _, a := range t.Accepts {
		if a == c {
			return true
		}
	}
	return false
}

// Option returns the named option definition.
func (t Task) Option(id string) (Option, bool) {
	for _, o := range t.Options {
		if o.ID == id {
			return o, true
		}
	}
	return Option{}, false
}

// Defaults returns every option's default, which is the starting point for a
// job's options map. A task screen is usable with no changes at all.
func (t Task) Defaults() map[string]any {
	out := make(map[string]any, len(t.Options))
	for _, o := range t.Options {
		if o.Default != nil {
			out[o.ID] = o.Default
		}
	}
	return out
}

// Registry is the set of tasks the app knows about.
type Registry struct {
	byID  map[string]Task
	order []string
}

// NewRegistry builds a registry from tasks, rejecting duplicates and
// definitions that break the interface rules.
func NewRegistry(tasks ...Task) (*Registry, error) {
	r := &Registry{byID: make(map[string]Task, len(tasks))}
	for _, t := range tasks {
		if err := Validate(t); err != nil {
			return nil, err
		}
		if _, dup := r.byID[t.ID]; dup {
			return nil, fmt.Errorf("duplicate task id %q", t.ID)
		}
		r.byID[t.ID] = t
		r.order = append(r.order, t.ID)
	}
	return r, nil
}

// Get returns a task by ID.
func (r *Registry) Get(id string) (Task, bool) {
	t, ok := r.byID[id]
	return t, ok
}

// ByCommand returns a task by its CLI subcommand name.
func (r *Registry) ByCommand(name string) (Task, bool) {
	for _, id := range r.order {
		if t := r.byID[id]; t.Command() == name {
			return t, true
		}
	}
	return Task{}, false
}

// All returns every task in registration order.
func (r *Registry) All() []Task {
	out := make([]Task, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id])
	}
	return out
}

// Accepting returns the tasks that can handle a file of this category, which
// is what turns dropping a file on the home screen into a filtered grid.
func (r *Registry) Accepting(c detect.Category) []Task {
	var out []Task
	for _, t := range r.All() {
		if t.AcceptsCategory(c) {
			out = append(out, t)
		}
	}
	return out
}

// Categories returns the task groups present, in display order.
func (r *Registry) Categories() []Category {
	order := []Category{CategoryPDF, CategoryImage, CategoryText, CategoryDocument, CategoryMedia}
	seen := map[Category]bool{}
	for _, t := range r.All() {
		seen[t.Category] = true
	}
	out := make([]Category, 0, len(order))
	for _, c := range order {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out
}

// Commands returns every CLI subcommand name, sorted.
func (r *Registry) Commands() []string {
	out := make([]string, 0, len(r.order))
	for _, t := range r.All() {
		out = append(out, t.Command())
	}
	sort.Strings(out)
	return out
}

// Validate enforces the rules a task definition must satisfy. It runs at
// registry construction, so a malformed task fails at startup and in tests
// rather than in front of a user.
func Validate(t Task) error {
	switch {
	case t.ID == "":
		return fmt.Errorf("task has no id")
	case t.Name == "":
		return fmt.Errorf("task %q has no name", t.ID)
	case t.Verb == "":
		return fmt.Errorf("task %q has no button verb", t.ID)
	case t.Engine == "":
		return fmt.Errorf("task %q has no engine", t.ID)
	case len(t.Accepts) == 0:
		return fmt.Errorf("task %q accepts nothing", t.ID)
	case t.MinInputs < 1:
		return fmt.Errorf("task %q must accept at least one input", t.ID)
	case t.MaxInputs != 0 && t.MaxInputs < t.MinInputs:
		return fmt.Errorf("task %q has MaxInputs below MinInputs", t.ID)
	}

	primary := 0
	seen := map[string]bool{}
	for _, o := range t.Options {
		if o.ID == "" {
			return fmt.Errorf("task %q has an option with no id", t.ID)
		}
		if seen[o.ID] {
			return fmt.Errorf("task %q has duplicate option %q", t.ID, o.ID)
		}
		seen[o.ID] = true

		if !o.Advanced {
			primary++
		}
		if o.Type == OptionChoice {
			if len(o.Choices) < 2 {
				return fmt.Errorf("task %q option %q is a choice with fewer than two entries", t.ID, o.ID)
			}
			if !o.Advanced && len(o.Choices) > 4 {
				return fmt.Errorf("task %q option %q offers %d choices; at most 4 outside Advanced",
					t.ID, o.ID, len(o.Choices))
			}
		}
	}
	if primary > MaxPrimaryOptions {
		return fmt.Errorf("task %q shows %d options; at most %d may sit outside Advanced",
			t.ID, primary, MaxPrimaryOptions)
	}
	return nil
}
