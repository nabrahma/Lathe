package app

import (
	"os"
	"path/filepath"
	"testing"
)

// Launching a viewer or a file manager puts a window on someone's screen, so
// it is a manual check rather than part of the suite.
//
//	LATHE_LAUNCH_CHECK=reveal go test ./internal/app -run TestManualLaunch -v
func TestManualLaunch(t *testing.T) {
	which := os.Getenv("LATHE_LAUNCH_CHECK")
	if which == "" {
		t.Skip("set LATHE_LAUNCH_CHECK=open|reveal to put a window on screen")
	}

	path := os.Getenv("LATHE_LAUNCH_PATH")
	if path == "" {
		dir := t.TempDir()
		path = filepath.Join(dir, "lathe launch check.txt")
		if err := os.WriteFile(path, []byte("Lathe launch check.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Logf("using a temporary file with a space in its name: %s", path)
	}

	build := revealCommand
	if which == "open" {
		build = openCommand
	}
	if err := start(path, build); err != nil {
		t.Fatalf("%s: %v", which, err)
	}
	t.Logf("%s launched; check the screen", which)
}
