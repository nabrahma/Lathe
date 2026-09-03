// Package app is the only package that touches Wails.
//
// Everything below it — tasks, pipeline, engines — is driven through the same
// interfaces the CLI uses, so this layer holds no logic of its own. That is
// what keeps an eventual move to Wails v3 a rewrite of one package rather than
// of the application, and CI enforces the boundary.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/detect"
	"github.com/nabrahma/lathe/internal/engines"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/job"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/settings"
	"github.com/nabrahma/lathe/internal/shellint"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/version"
)

// orphanAge is how old a leftover workspace must be before startup assumes it
// belongs to a crashed run rather than to a second window.
const orphanAge = 6 * time.Hour

// App is the API the interface calls. Every exported method is bound into
// JavaScript by Wails.
type App struct {
	ctx      context.Context
	registry *task.Registry
	queue    *job.Queue
	deps     deps.Manager
	settings *settings.Store
}

// New builds the application backend.
func New() (*App, error) {
	store, err := settings.Load()
	if err != nil {
		return nil, err
	}

	manager, err := deps.New("")
	if err != nil {
		return nil, err
	}

	runner := pipeline.New(engines.Default(manager), manager)
	return &App{
		registry: task.Default(),
		queue:    job.NewQueue(runner, store.Get().Concurrency),
		deps:     manager,
		settings: store,
	}, nil
}

// Startup is called by Wails once the window exists. Nothing expensive belongs
// here: cold start under two seconds does more for perceived nativeness than
// any visual detail, so component detection happens after the first paint.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Geometry is cheap and has to happen before the window is seen.
	a.RestoreWindow()

	go func() {
		// Clear workspaces a previous crash left behind.
		_, _ = fsatomic.CleanOrphans(orphanAge)
		a.forwardJobEvents()
	}()
}

// Shutdown cancels everything in flight so quitting leaves no orphaned engine
// processes.
func (a *App) Shutdown(context.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.queue.Shutdown(ctx)
}

// BeforeClose asks the interface to confirm when work is still running, rather
// than discarding it silently.
func (a *App) BeforeClose(context.Context) bool {
	if a.queue.Active() == 0 {
		return false
	}
	runtime.EventsEmit(a.ctx, "quit:confirm", a.queue.Active())
	return true // prevent the close; the interface decides what happens next
}

// forwardJobEvents relays queue changes to the interface.
func (a *App) forwardJobEvents() {
	events, unsubscribe := a.queue.Subscribe()
	defer unsubscribe()

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			runtime.EventsEmit(a.ctx, "job:update", ev.Job)
			a.reflectInTaskbar()
		case <-a.ctx.Done():
			return
		}
	}
}

// reflectInTaskbar keeps the dock or taskbar in step with the queue. It is a
// small touch that reads strongly as native.
func (a *App) reflectInTaskbar() {
	if a.queue.Active() == 0 {
		runtime.WindowSetTitle(a.ctx, version.Name)
		return
	}
	runtime.WindowSetTitle(a.ctx, fmt.Sprintf("%s — %d running", version.Name, a.queue.Active()))
}

// --------------------------------------------------------------- task API

// TaskView is a task as the interface renders it, with availability resolved.
type TaskView struct {
	task.Task
	// Available is false when the task needs a component that is not
	// installed. The card stays visible and says what it needs, rather than
	// disappearing.
	Available bool `json:"available"`
	// DownloadMB is what the task would need to fetch first, for the badge.
	DownloadMB int `json:"downloadMB"`
	// Requires names the missing component in plain words.
	Requires string `json:"requires,omitempty"`
}

// Tasks returns every task, with availability resolved.
func (a *App) Tasks() []TaskView {
	all := a.registry.All()
	out := make([]TaskView, 0, len(all))
	for _, t := range all {
		out = append(out, a.viewOf(t))
	}
	return out
}

// TasksFor returns the tasks that accept a file of this category, which is
// what turns dropping a file on the home screen into a filtered grid.
func (a *App) TasksFor(category string) []TaskView {
	matching := a.registry.Accepting(detect.Category(category))
	out := make([]TaskView, 0, len(matching))
	for _, t := range matching {
		out = append(out, a.viewOf(t))
	}
	return out
}

func (a *App) viewOf(t task.Task) TaskView {
	v := TaskView{Task: t, Available: true}
	if !a.deps.TierAvailable(t.RequiredTier) {
		v.Available = false
		v.DownloadMB = a.deps.TierDownloadMB(t.RequiredTier)
		v.Requires = a.deps.TierName(t.RequiredTier)
	}
	return v
}

// --------------------------------------------------------------- file API

// FileInfo describes a dropped or chosen file, as the file list shows it.
type FileInfo struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	Category  string `json:"category"`
	Extension string `json:"extension"`
	Encrypted bool   `json:"encrypted"`
	// Mismatch is set when the contents disagree with the extension, so the
	// interface can say "this is actually a HEIC image".
	Mismatch string `json:"mismatch,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Inspect identifies files by content. It never fails as a whole: a file that
// cannot be read reports its own error and the rest are still described.
func (a *App) Inspect(paths []string) []FileInfo {
	out := make([]FileInfo, 0, len(paths))
	for _, p := range paths {
		name := filepath.Base(p)
		info := FileInfo{Path: p, Name: name}

		ft, err := detect.Detect(p)
		if err != nil {
			info.Error = err.Error()
			info.Category = string(detect.CategoryUnknown)
			out = append(out, info)
			continue
		}

		info.SizeBytes = ft.SizeBytes
		info.Category = string(ft.Category)
		info.Extension = ft.Extension
		info.Encrypted = ft.Encrypted
		if ft.MismatchesName(name) {
			info.Mismatch = ft.Extension
		}
		out = append(out, info)
	}
	return out
}

// ChooseFiles opens the operating system's own file dialog. Never an HTML file
// input: that shortcut is the single most obvious tell that a window is a web
// page, and it has a way of surviving to release.
func (a *App) ChooseFiles(title string) ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                title,
		ShowHiddenFiles:      false,
		CanCreateDirectories: false,
	})
}

// ChooseFolder opens the OS folder picker, for the output location.
func (a *App) ChooseFolder(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                title,
		CanCreateDirectories: true,
	})
}

// Reveal shows a finished file in Explorer or Finder.
func (a *App) Reveal(path string) {
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(filepath.Dir(path)))
}

// Open hands a finished file to whatever application owns it.
func (a *App) Open(path string) {
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(path))
}

// ---------------------------------------------------------------- job API

// Submit queues a job and returns it immediately.
func (a *App) Submit(taskID string, inputs []string, options map[string]any, outputDir string) (*job.Job, error) {
	t, ok := a.registry.Get(taskID)
	if !ok {
		return nil, fmt.Errorf("no such task %q", taskID)
	}
	if outputDir == "" && len(inputs) > 0 {
		outputDir = filepath.Dir(inputs[0])
	}
	return a.queue.Submit(t, inputs, options, outputDir)
}

// Cancel stops a job.
func (a *App) Cancel(jobID string) error { return a.queue.Cancel(jobID) }

// Jobs lists every job, newest first.
func (a *App) Jobs() []*job.Job { return a.queue.List() }

// ActiveJobs reports how many jobs have not finished, which is what the quit
// prompt needs.
func (a *App) ActiveJobs() int { return a.queue.Active() }

// Quit closes the app after cancelling anything still running.
func (a *App) Quit() {
	a.queue.CancelAll()
	runtime.Quit(a.ctx)
}

// --------------------------------------------------------- component API

// Components reports what is installed, for the settings screen.
func (a *App) Components() []deps.Status {
	statuses := a.deps.Status(a.ctx)
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Component.Tier < statuses[j].Component.Tier
	})
	return statuses
}

// InstallComponent downloads a component, reporting progress as events.
func (a *App) InstallComponent(id string) error {
	return a.deps.Ensure(a.ctx, id, func(p deps.Progress) {
		runtime.EventsEmit(a.ctx, "component:progress", p)
	})
}

// RemoveComponent uninstalls a component to reclaim disk space.
func (a *App) RemoveComponent(id string) error { return a.deps.Remove(id) }

// ------------------------------------------------------------ settings API

// Settings returns the user's preferences.
func (a *App) Settings() settings.Settings { return a.settings.Get() }

// SaveSettings persists preferences and applies the ones that change something
// outside Lathe. The shell entry is the only such setting: recording it without
// acting on it would leave the toggle lying to the user.
func (a *App) SaveSettings(next settings.Settings) error {
	previous := a.settings.Get()
	if err := a.settings.Save(next); err != nil {
		return err
	}

	if next.ShellIntegration != previous.ShellIntegration {
		if err := a.applyShellIntegration(next.ShellIntegration); err != nil {
			// Put the setting back, so the toggle reflects reality.
			previous.ShellIntegration = !next.ShellIntegration
			reverted := next
			reverted.ShellIntegration = previous.ShellIntegration
			_ = a.settings.Save(reverted)
			return err
		}
	}
	return nil
}

// applyShellIntegration adds or removes the "Convert with Lathe" entry.
func (a *App) applyShellIntegration(enabled bool) error {
	integrator := shellint.New()
	if !enabled {
		return integrator.Remove()
	}

	if status := integrator.Status(); !status.Supported {
		return fmt.Errorf("%s", status.Detail)
	}
	executable, err := shellint.Executable()
	if err != nil {
		return err
	}
	return integrator.Install(executable)
}

// ShellIntegrationStatus reports whether the context-menu entry is present, so
// the settings screen can show what is actually true rather than what was last
// requested.
func (a *App) ShellIntegrationStatus() shellint.Status {
	return shellint.New().Status()
}

// Platform tells the interface which operating system it is on, so it can pick
// the right modifier key and window chrome without guessing from the user
// agent.
func (a *App) Platform() map[string]string {
	return map[string]string{
		"os":      hostOS(),
		"version": version.Version,
		"name":    version.Name,
	}
}

// ------------------------------------------------------- window and shell

// Context exposes the Wails context to the menu builder, which lives in cmd/
// and must not reach for runtime state of its own.
func (a *App) Context() context.Context { return a.ctx }

// OnSecondInstance focuses the window that is already open rather than letting
// a second one appear, and routes any file the launch carried into it. This is
// what makes "Convert with Lathe" from the context menu behave sensibly while
// the app is running.
func (a *App) OnSecondInstance(data options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)

	if files := filterPaths(data.Args); len(files) > 0 {
		runtime.EventsEmit(a.ctx, "files:opened", a.Inspect(files))
	}
}

// RequestOpenFiles asks the interface to run its own open-files flow, so the
// menu item and the drop zone end up in exactly the same state.
func (a *App) RequestOpenFiles() {
	runtime.EventsEmit(a.ctx, "menu:openFiles")
}

// RequestScreen asks the interface to navigate, for menu items that map onto a
// screen rather than an action.
func (a *App) RequestScreen(name string) {
	runtime.EventsEmit(a.ctx, "menu:screen", name)
}

// SaveWindowState records geometry so the window reopens where it was. A
// maximised window records the size it would restore to, not the screen size.
func (a *App) SaveWindowState() {
	w, h := runtime.WindowGetSize(a.ctx)
	x, y := runtime.WindowGetPosition(a.ctx)

	current := a.settings.Get()
	current.Window = settings.WindowState{
		Width: w, Height: h, X: x, Y: y,
		Maximised: runtime.WindowIsMaximised(a.ctx),
	}
	_ = a.settings.Save(current)
}

// The three window controls behind the custom chrome Lathe draws on Windows
// and macOS. On Linux the window manager owns the title bar and these are
// unused.

// Minimise sends the window to the taskbar or dock.
func (a *App) Minimise() { runtime.WindowMinimise(a.ctx) }

// ToggleMaximise switches between the restored and maximised window.
func (a *App) ToggleMaximise() { runtime.WindowToggleMaximise(a.ctx) }

// Close quits, going through BeforeClose so running jobs are confirmed first.
func (a *App) Close() { runtime.Quit(a.ctx) }

// filterPaths keeps only arguments that name an existing file, so a stray flag
// in a second launch is not mistaken for a document.
func filterPaths(args []string) []string {
	var out []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if info, err := os.Stat(arg); err == nil && !info.IsDir() {
			out = append(out, arg)
		}
	}
	return out
}

// RestoreWindow puts the window back where it was, unless that position is no
// longer on any screen. Restoring off-screen leaves someone with a running app
// and no visible window, which looks exactly like a crash.
func (a *App) RestoreWindow() {
	saved := a.settings.Get().Window
	if saved.Maximised {
		runtime.WindowMaximise(a.ctx)
		return
	}
	if saved.X < 0 || saved.Y < 0 {
		runtime.WindowCenter(a.ctx)
		return
	}

	if !a.plausiblyOnScreen(saved) {
		runtime.WindowCenter(a.ctx)
		return
	}
	runtime.WindowSetPosition(a.ctx, saved.X, saved.Y)
}

// plausiblyOnScreen is a conservative guard against restoring a window nobody
// can reach.
//
// Wails v2 reports each screen's size but not its origin, so the exact bounds
// of a multi-monitor desktop are not knowable here. What it can rule out is a
// position far outside any arrangement those sizes could produce — which is
// what a disconnected second monitor leaves behind. Anything it cannot rule
// out is allowed through, because wrongly re-centring a window someone
// deliberately placed is the more annoying failure of the two.
func (a *App) plausiblyOnScreen(w settings.WindowState) bool {
	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil || len(screens) == 0 {
		return false
	}

	// The widest possible desktop is every screen side by side, and the
	// tallest is every screen stacked. Allowing both at once is deliberately
	// generous: it covers any arrangement without needing the origins.
	var totalWidth, totalHeight int
	for _, screen := range screens {
		totalWidth += screen.Size.Width
		totalHeight += screen.Size.Height
	}

	// The title bar has to be grabbable, so require a strip of the window to
	// fall inside that extent rather than merely touching it.
	const grabbable = 120
	return w.X+grabbable < totalWidth && w.Y+40 < totalHeight
}
