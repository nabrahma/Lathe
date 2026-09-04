//go:build !windows

package app

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// The path reaching these has already been checked against the outputs of
// this session's own jobs, in App.launch, so it is one of a set the backend
// produced rather than anything the web view chose. gosec's taint analysis
// cannot follow that check across the call, hence the annotations below: the
// mitigation is the allowlist, not the comment.

func openCommand(path string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		//nolint:gosec // allowlisted against this session's job outputs in App.launch
		return exec.Command("open", path)
	}
	//nolint:gosec // allowlisted against this session's job outputs in App.launch
	return exec.Command("xdg-open", path)
}

func revealCommand(path string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		//nolint:gosec // allowlisted against this session's job outputs in App.launch
		return exec.Command("open", "-R", path)
	}
	// Linux has no cross-desktop way to ask for one file to be highlighted:
	// the file managers that support it each spell it differently, and the
	// portal interface for it is not carried everywhere. Opening the folder
	// the file sits in is the honest approximation, and it is what the button
	// promises anyway.
	//nolint:gosec // allowlisted against this session's job outputs in App.launch
	return exec.Command("xdg-open", filepath.Dir(path))
}
