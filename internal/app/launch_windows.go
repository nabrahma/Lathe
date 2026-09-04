//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// The path reaching these has already been checked against the outputs of
// this session's own jobs, in App.launch, so it is one of a set the backend
// produced rather than anything the web view chose. gosec's taint analysis
// cannot follow that check across the call, hence the annotations below: the
// mitigation is the allowlist, not the comment.

// openCommand hands the file to whatever application is registered for it.
// rundll32 takes its argument conventionally, so ordinary quoting is correct.
func openCommand(path string) *exec.Cmd {
	//nolint:gosec // allowlisted against this session's job outputs in App.launch
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", path)
}

// revealCommand opens the containing folder with the file already selected.
//
// The command line is built by hand because Explorer parses this argument
// itself and wants exactly one shape: the switch bare, the path quoted, in a
// single token. os/exec would quote the whole token, switch included, and
// Explorer answers that by opening Documents rather than reporting a problem,
// which is the kind of failure nobody files a bug about.
func revealCommand(path string) *exec.Cmd {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `explorer.exe /select,"` + path + `"`,
	}
	return cmd
}
