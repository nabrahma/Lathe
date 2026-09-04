package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nabrahma/lathe/internal/usererr"
)

// Opening a finished file has to go through the platform's own launcher.
//
// Handing "file://" + a path to the browser opener looks right and is not. On
// Windows a path begins C:\, so the result is file://C:/Users/..., in which
// C: parses as the host name and the rest as a path on it. Nothing local is
// named, the request is dropped without an error, and both buttons appear to
// do nothing at all. That is the bug this file exists to fix.

// Open hands a finished file to whatever application owns it.
func (a *App) Open(path string) error {
	return a.launch(path, openCommand)
}

// Reveal shows a finished file in the system file manager.
func (a *App) Reveal(path string) error {
	return a.launch(path, revealCommand)
}

func (a *App) launch(path string, build func(string) *exec.Cmd) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeCorruptInput,
			"That file's location could not be worked out.", usererr.ActionChooseFile)
	}

	// Only files Lathe made. These two methods are reachable from the web view,
	// so the path arriving here is not something this side of the boundary
	// chose, and handing an arbitrary one to the system launcher would mean
	// asking the machine to run whatever it was pointed at. Restricting it to
	// this session's own results costs nothing, since the buttons exist only on
	// a finished job, and it turns the argument into one of a set the backend
	// produced itself.
	if !a.produced(abs) {
		return usererr.New(usererr.CodeCorruptInput,
			"Lathe only opens files it produced.", usererr.ActionChooseFile)
	}

	return start(abs, build)
}

// start hands the path to the system, after the caller has established that it
// is one Lathe is willing to open.
func start(abs string, build func(string) *exec.Cmd) error {
	if _, err := os.Stat(abs); err != nil {
		// Moved or deleted since the job finished, which is worth saying
		// plainly rather than launching something at a path that is not there.
		return usererr.New(usererr.CodeCorruptInput,
			fmt.Sprintf("%s isn't where Lathe left it. It may have been moved or deleted.",
				filepath.Base(abs)),
			usererr.ActionChooseFile)
	}

	cmd := build(abs)
	if err := cmd.Start(); err != nil {
		return usererr.Wrap(err, usererr.CodeComponentMissing,
			"Lathe couldn't ask this computer to open that file.",
			usererr.ActionCopyDetails)
	}

	// Reaped in the background rather than waited for. A document viewer
	// outlives this call by design, and on Unix a child nobody waits on stays
	// a zombie for as long as Lathe runs.
	go func() { _ = cmd.Wait() }()
	return nil
}

// produced reports whether this path is one of the outputs of a job in this
// session.
func (a *App) produced(abs string) bool {
	for _, j := range a.queue.List() {
		for _, out := range j.Outputs {
			if sameFile(out, abs) {
				return true
			}
		}
	}
	return false
}

// sameFile compares two paths the way the filesystem underneath them does.
func sameFile(a, b string) bool {
	x, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	y := filepath.Clean(b)
	if runtime.GOOS == "windows" {
		// Windows paths are case-insensitive, so comparing them byte for byte
		// would reject a file the user is looking at.
		return strings.EqualFold(x, y)
	}
	return x == y
}
