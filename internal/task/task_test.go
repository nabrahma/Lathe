package task_test

import (
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/detect"
	"github.com/nabrahma/lathe/internal/task"
)

func TestShippedCatalogIsValid(t *testing.T) {
	if _, err := task.NewRegistry(task.Catalog()...); err != nil {
		t.Fatalf("shipped catalog is malformed: %v", err)
	}
}

// The interface rules from the design are enforced here rather than trusted,
// because they are exactly the kind of thing that erodes one task at a time.
func TestEveryTaskRespectsTheInterfaceRules(t *testing.T) {
	for _, tk := range task.Catalog() {
		t.Run(tk.ID, func(t *testing.T) {
			primary := 0
			for _, o := range tk.Options {
				if !o.Advanced {
					primary++
				}
			}
			if primary > task.MaxPrimaryOptions {
				t.Errorf("shows %d options outside Advanced, at most %d allowed", primary, task.MaxPrimaryOptions)
			}
			if tk.Verb == strings.ToUpper(tk.Verb) && len(tk.Verb) > 3 {
				t.Errorf("verb %q is stored uppercase; casing is the UI's job so it can be lowered for other locales", tk.Verb)
			}
			if strings.HasSuffix(tk.Description, ".") {
				t.Errorf("description %q ends with a full stop; card descriptions are fragments", tk.Description)
			}
			for _, banned := range []string{"codec", "bitrate", "DPI", "colorspace", "container"} {
				if strings.Contains(tk.Name, banned) || strings.Contains(tk.Description, banned) {
					t.Errorf("jargon %q appears in a primary label", banned)
				}
			}
		})
	}
}

func TestNoJargonInPrimaryOptionLabels(t *testing.T) {
	jargon := []string{"codec", "bitrate", "dpi", "colorspace", "chroma", "container"}
	for _, tk := range task.Catalog() {
		for _, o := range tk.Options {
			if o.Advanced {
				continue // Advanced is where the technical vocabulary belongs.
			}
			lower := strings.ToLower(o.Label)
			for _, j := range jargon {
				if strings.Contains(lower, j) {
					t.Errorf("%s: primary option %q uses jargon %q", tk.ID, o.Label, j)
				}
			}
		}
	}
}

func TestRegistryRejectsDuplicateIDs(t *testing.T) {
	dup := task.Task{
		ID: "x", Name: "X", Verb: "Do", Engine: "e",
		Accepts: []detect.Category{detect.CategoryPDF}, MinInputs: 1, MaxInputs: 1,
	}
	if _, err := task.NewRegistry(dup, dup); err == nil {
		t.Fatal("expected duplicate task ids to be rejected")
	}
}

func TestValidateRejectsTooManyPrimaryOptions(t *testing.T) {
	tk := task.Task{
		ID: "x", Name: "X", Verb: "Do", Engine: "e",
		Accepts: []detect.Category{detect.CategoryPDF}, MinInputs: 1, MaxInputs: 1,
		Options: []task.Option{
			{ID: "a", Type: task.OptionToggle}, {ID: "b", Type: task.OptionToggle},
			{ID: "c", Type: task.OptionToggle}, {ID: "d", Type: task.OptionToggle},
		},
	}
	if err := task.Validate(tk); err == nil {
		t.Fatal("expected four primary options to be rejected")
	}
}

func TestValidateRejectsLongChoiceLists(t *testing.T) {
	choices := make([]task.Choice, 5)
	for i := range choices {
		choices[i] = task.Choice{Value: string(rune('a' + i)), Label: "L"}
	}
	tk := task.Task{
		ID: "x", Name: "X", Verb: "Do", Engine: "e",
		Accepts: []detect.Category{detect.CategoryPDF}, MinInputs: 1, MaxInputs: 1,
		Options: []task.Option{{ID: "fmt", Type: task.OptionChoice, Choices: choices}},
	}
	if err := task.Validate(tk); err == nil {
		t.Fatal("expected a five-entry primary choice list to be rejected")
	}

	tk.Options[0].Advanced = true
	if err := task.Validate(tk); err != nil {
		t.Fatalf("a long choice list behind Advanced is allowed: %v", err)
	}
}

func TestAcceptingDrivesDragToFilter(t *testing.T) {
	r := task.Default()

	imageTasks := r.Accepting(detect.CategoryImage)
	if len(imageTasks) < 5 {
		t.Fatalf("dropping an image should offer several tasks, got %d", len(imageTasks))
	}
	// The specific set someone dropping a photo expects to see.
	want := []string{"image.convert", "image.compress", "image.resize", "pdf.from-images", "text.from-image"}
	for _, id := range want {
		if !containsTask(imageTasks, id) {
			t.Errorf("dropping an image should offer %s", id)
		}
	}
	for _, tk := range imageTasks {
		if tk.ID == "pdf.merge" {
			t.Error("dropping an image should not offer Merge PDFs")
		}
	}
}

func TestCLICommandNamesAreUniqueAndShellSafe(t *testing.T) {
	r := task.Default()
	seen := map[string]bool{}
	for _, name := range r.Commands() {
		if seen[name] {
			t.Errorf("duplicate CLI command %q", name)
		}
		seen[name] = true
		if strings.ContainsAny(name, " .:/\\") {
			t.Errorf("CLI command %q contains a character that needs quoting", name)
		}
	}
}

func TestDefaultsCoverEveryOption(t *testing.T) {
	for _, tk := range task.Catalog() {
		defaults := tk.Defaults()
		for _, o := range tk.Options {
			if o.Type == task.OptionPassword {
				continue // A password has no sensible default.
			}
			if _, ok := defaults[o.ID]; !ok {
				t.Errorf("%s: option %q has no default, so the task screen is not usable as-is", tk.ID, o.ID)
			}
		}
	}
}

func TestChoiceDefaultsAreAmongTheChoices(t *testing.T) {
	for _, tk := range task.Catalog() {
		for _, o := range tk.Options {
			if o.Type != task.OptionChoice {
				continue
			}
			def, _ := o.Default.(string)
			found := false
			for _, c := range o.Choices {
				if c.Value == def {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: option %q defaults to %q, which is not one of its choices", tk.ID, o.ID, def)
			}
		}
	}
}

func containsTask(tasks []task.Task, id string) bool {
	for _, t := range tasks {
		if t.ID == id {
			return true
		}
	}
	return false
}
