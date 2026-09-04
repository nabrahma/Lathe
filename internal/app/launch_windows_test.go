//go:build windows

package app

import (
	"strings"
	"testing"
)

// Explorer parses this argument itself and accepts exactly one shape: the
// switch bare, the path quoted, in a single token. os/exec would quote the
// whole token, switch included, and Explorer answers that by opening Documents
// instead of reporting a problem, so nothing about the failure is visible.
func TestRevealQuotesThePathButNotTheSwitch(t *testing.T) {
	cmd := revealCommand(`C:\Users\Anna Sharma\Documents\merged.pdf`)

	if cmd.SysProcAttr == nil {
		t.Fatal("reveal was left to os/exec quoting, which Explorer does not accept")
	}

	got := cmd.SysProcAttr.CmdLine
	want := `explorer.exe /select,"C:\Users\Anna Sharma\Documents\merged.pdf"`
	if got != want {
		t.Errorf("command line is\n  %s\nwant\n  %s", got, want)
	}
	if strings.Contains(got, `"/select`) {
		t.Error("the switch was quoted along with the path")
	}
}

// The opener takes its argument conventionally, so ordinary quoting is right
// here and the raw command line would be the mistake.
func TestOpenPassesThePathAsAnArgument(t *testing.T) {
	path := `C:\Users\Anna Sharma\Documents\merged.pdf`
	cmd := openCommand(path)

	if cmd.SysProcAttr != nil {
		t.Error("open built a raw command line, which it does not need")
	}
	if len(cmd.Args) < 3 || cmd.Args[len(cmd.Args)-1] != path {
		t.Errorf("path is not the final argument: %q", cmd.Args)
	}
}
