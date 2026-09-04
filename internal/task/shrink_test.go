package task_test

import (
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/task"
)

// A before-and-after size is only meaningful where making the file smaller was
// the point. Marking anything else would put a saving on a result nobody asked
// to be smaller, which reads as something having been thrown away.
func TestOnlyCompressionTasksReportASizeChange(t *testing.T) {
	for _, tk := range task.Default().All() {
		compresses := strings.Contains(tk.ID, "compress")
		if tk.ShrinksFile != compresses {
			t.Errorf("%s: ShrinksFile is %v but the task %s a file",
				tk.ID, tk.ShrinksFile,
				map[bool]string{true: "compresses", false: "does not compress"}[compresses])
		}
	}
}
