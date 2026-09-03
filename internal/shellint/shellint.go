// Package shellint adds and removes the "Convert with Lathe" entry in the
// operating system's file context menu.
//
// It is opt-in, offered once, and completely removable. Silently modifying
// someone's shell is hostile, and an entry left behind after an uninstall is
// worse.
package shellint

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// MenuLabel is what the entry reads in Explorer or Finder.
const MenuLabel = "Convert with Lathe"

// Status reports whether the entry is currently installed.
type Status struct {
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	Detail    string `json:"detail,omitempty"`
}

// Integrator installs and removes the context-menu entry for one platform.
type Integrator interface {
	// Status reports the current state without changing anything.
	Status() Status
	// Install adds the entry for the current user only. It never writes to a
	// machine-wide location, so it needs no elevation.
	Install(executable string) error
	// Remove takes the entry away and leaves nothing behind.
	Remove() error
}

// New returns the integrator for the running platform.
func New() Integrator {
	switch runtime.GOOS {
	case "windows":
		return &windowsIntegrator{}
	case "linux":
		return &linuxIntegrator{}
	default:
		// macOS exposes this through a Quick Action bundled with the app,
		// which is installed by the .app itself rather than at runtime.
		return &unsupported{reason: "On macOS this is provided by a Quick Action inside the app bundle."}
	}
}

// Executable returns the path to use in the menu entry.
func Executable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the application: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, nil //nolint:nilerr // the unresolved path still works
	}
	return resolved, nil
}

type unsupported struct{ reason string }

func (u *unsupported) Status() Status {
	return Status{Supported: false, Detail: u.reason}
}

func (u *unsupported) Install(string) error { return fmt.Errorf("%s", u.reason) }
func (u *unsupported) Remove() error        { return nil }
