// Package cli builds the headless command line over the same task registry the
// desktop app uses.
//
// Every task becomes a subcommand automatically, so a task added for the UI is
// scriptable the same day without extra work.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
	"github.com/nabrahma/lathe/internal/version"
)

// App is the command-line front end.
type App struct {
	Registry *task.Registry
	Runner   *pipeline.Runner
	Stdout   io.Writer
	Stderr   io.Writer
}

// Run executes one invocation and returns the process exit code.
func (a *App) Run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.usage()
		return 2
	}

	switch args[0] {
	case "-h", "--help", "help":
		a.usage()
		return 0
	case "-v", "--version", "version":
		fmt.Fprintf(a.Stdout, "%s %s\n", strings.ToLower(version.Name), version.Version)
		return 0
	case "--list-tasks", "list", "tasks":
		a.listTasks()
		return 0
	}

	tk, ok := a.Registry.ByCommand(args[0])
	if !ok {
		fmt.Fprintf(a.Stderr, "unknown command %q\n\nRun \"lathe --list-tasks\" to see what Lathe can do.\n", args[0])
		return 2
	}

	inputs, opts, outPath, err := parseArgs(tk, args[1:])
	if err != nil {
		fmt.Fprintln(a.Stderr, err)
		return 2
	}
	if len(inputs) == 0 {
		a.taskUsage(tk)
		return 2
	}

	outDir := outPath
	renameTo := ""
	if outDir == "" {
		outDir = filepath.Dir(inputs[0])
	} else if filepath.Ext(outDir) != "" {
		// A file path was given rather than a directory, which only makes
		// sense for a task that produces exactly one output.
		renameTo = outDir
		outDir = filepath.Dir(outDir)
		if outDir == "" {
			outDir = "."
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(a.Stderr, "cannot use %s: %v\n", outDir, err)
		return 1
	}

	started := time.Now()
	res, err := a.Runner.Run(ctx, pipeline.Request{
		Task: tk, Inputs: inputs, Options: opts, OutputDir: outDir,
		Progress: a.progressReporter(),
	})
	if err != nil {
		a.reportError(err)
		return 1
	}

	if renameTo != "" && len(res.Outputs) == 1 {
		if err := os.Rename(res.Outputs[0], renameTo); err == nil {
			res.Outputs[0] = renameTo
		}
	}

	for _, out := range res.Outputs {
		fmt.Fprintln(a.Stdout, out)
	}
	for _, note := range res.Notes {
		fmt.Fprintf(a.Stderr, "note: %s\n", note)
	}
	if res.InputBytes > 0 && res.OutputBytes > 0 {
		fmt.Fprintf(a.Stderr, "%s -> %s in %s\n",
			humanBytes(res.InputBytes), humanBytes(res.OutputBytes),
			started.Sub(started.Add(-time.Since(started))).Round(time.Millisecond))
	}
	return 0
}

// progressReporter writes stage changes to stderr, so stdout stays a clean
// list of output paths that a script can consume.
func (a *App) progressReporter() func(engine.Progress) {
	last := ""
	return func(p engine.Progress) {
		if p.Stage == "" || p.Stage == last {
			return
		}
		last = p.Stage
		fmt.Fprintf(a.Stderr, "%s\n", p.Stage)
	}
}

func (a *App) reportError(err error) {
	var ue *usererr.Error
	if errors.As(err, &ue) {
		fmt.Fprintln(a.Stderr, ue.Message)
		if ue.Detail != "" && ue.Detail != ue.Message {
			fmt.Fprintf(a.Stderr, "details: %s\n", ue.Detail)
		}
		return
	}
	fmt.Fprintln(a.Stderr, err)
}

// parseArgs reads inputs and options. Options are long flags named after the
// task's own option IDs, so the CLI never drifts from the task screen.
func parseArgs(tk task.Task, args []string) (inputs []string, opts engine.Options, out string, err error) {
	opts = engine.Options(tk.Defaults())

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) {
				return nil, nil, "", fmt.Errorf("%s needs a path", arg)
			}
			i++
			out = args[i]

		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
			opt, known := tk.Option(name)
			if !known {
				return nil, nil, "", fmt.Errorf("%s does not take --%s\n\nRun \"lathe %s --help\" to see its options",
					tk.Command(), name, tk.Command())
			}
			if !hasValue {
				// A toggle may stand alone; anything else needs a value.
				if opt.Type == task.OptionToggle {
					value = "true"
				} else {
					if i+1 >= len(args) {
						return nil, nil, "", fmt.Errorf("--%s needs a value", name)
					}
					i++
					value = args[i]
				}
			}
			opts[name] = coerce(opt, value)

		case arg == "-h" || arg == "--help":
			return nil, nil, "", nil

		default:
			inputs = append(inputs, arg)
		}
	}
	return inputs, opts, out, nil
}

// coerce converts a flag string to the type the option expects, so engines see
// the same value shape whether it came from the UI or the shell.
func coerce(opt task.Option, value string) any {
	switch opt.Type {
	case task.OptionToggle:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return value
		}
		return b
	case task.OptionRange:
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return value
}

func (a *App) usage() {
	fmt.Fprintf(a.Stdout, `%s %s — convert, compress and read files, offline.

Usage:
  lathe <task> <file>... [options] [-o output]
  lathe --list-tasks
  lathe <task> --help

Examples:
  lathe pdf-compress report.pdf --quality medium
  lathe pdf-merge a.pdf b.pdf c.pdf -o merged.pdf
  lathe image-convert photo.heic --format jpg
  lathe text-from-image scan.jpg --lang eng+hin -o text.txt

Output paths are printed to stdout, one per line; progress and notes go to
stderr, so piping stdout gives you a clean list of results.
`, strings.ToLower(version.Name), version.Version)
}

func (a *App) listTasks() {
	w := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	byCategory := map[task.Category][]task.Task{}
	for _, tk := range a.Registry.All() {
		byCategory[tk.Category] = append(byCategory[tk.Category], tk)
	}

	for _, cat := range a.Registry.Categories() {
		fmt.Fprintf(w, "\n%s\n", strings.ToUpper(string(cat)))
		tasks := byCategory[cat]
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].Command() < tasks[j].Command() })
		for _, tk := range tasks {
			fmt.Fprintf(w, "  %s\t%s\n", tk.Command(), tk.Description)
		}
	}
}

func (a *App) taskUsage(tk task.Task) {
	fmt.Fprintf(a.Stderr, "%s — %s\n\n", tk.Command(), tk.Description)

	inputs := "<file>"
	switch {
	case tk.MaxInputs == 0:
		inputs = "<file>..."
	case tk.MinInputs > 1:
		inputs = fmt.Sprintf("<file> <file>... (%d or more)", tk.MinInputs)
	}
	fmt.Fprintf(a.Stderr, "Usage:\n  lathe %s %s [options] [-o output]\n", tk.Command(), inputs)

	if len(tk.Options) == 0 {
		return
	}
	fmt.Fprintln(a.Stderr, "\nOptions:")

	w := tabwriter.NewWriter(a.Stderr, 0, 0, 2, ' ', 0)
	defer func() { _ = w.Flush() }()

	for _, opt := range tk.Options {
		spec := "--" + opt.ID
		if opt.Type == task.OptionChoice {
			values := make([]string, len(opt.Choices))
			for i, c := range opt.Choices {
				values[i] = c.Value
			}
			spec += " " + strings.Join(values, "|")
		} else if opt.Type != task.OptionToggle {
			spec += " <value>"
		}

		suffix := ""
		if opt.Default != nil && opt.Default != "" {
			suffix = fmt.Sprintf(" (default %v)", opt.Default)
		}
		fmt.Fprintf(w, "  %s\t%s%s\n", spec, opt.Label, suffix)
	}
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGT"[exp])
}
