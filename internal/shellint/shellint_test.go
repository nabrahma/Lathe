package shellint_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/nabrahma/lathe/internal/shellint"
)

// The context-menu entry writes to the current user's own registry hive or
// applications directory, so this round-trip is safe to run: it touches
// nothing another user or the system depends on, and cleans up after itself.
//
// It is skipped unless LATHE_TEST_SHELLINT is set, because a test that
// modifies a developer's context menu without being asked is exactly the
// behaviour the package exists to avoid.

func TestInstallAndRemoveRoundTrip(t *testing.T) {
	if os.Getenv("LATHE_TEST_SHELLINT") == "" {
		t.Skip("set LATHE_TEST_SHELLINT=1 to let this test modify your context menu")
	}

	integrator := shellint.New()
	if !integrator.Status().Supported {
		t.Skipf("context-menu integration is not supported on %s", runtime.GOOS)
	}

	if before := integrator.Status(); before.Installed {
		t.Skip("an entry is already installed; refusing to disturb it")
	}

	executable, err := shellint.Executable()
	if err != nil {
		t.Fatal(err)
	}

	if err := integrator.Install(executable); err != nil {
		t.Fatalf("install: %v", err)
	}
	t.Cleanup(func() { _ = integrator.Remove() })

	installed := integrator.Status()
	if !installed.Installed {
		t.Fatal("Status reported no entry immediately after installing one")
	}
	// The recorded command must point at this binary, or the menu entry would
	// launch something else entirely.
	if !strings.Contains(installed.Detail, trimExt(executable)) {
		t.Errorf("registered command %q does not reference %q", installed.Detail, executable)
	}

	if err := integrator.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if after := integrator.Status(); after.Installed {
		t.Error("the entry survived removal; an uninstall must leave nothing behind")
	}
}

func TestRemoveIsSafeWhenNothingIsInstalled(t *testing.T) {
	integrator := shellint.New()
	if !integrator.Status().Supported {
		t.Skipf("not supported on %s", runtime.GOOS)
	}
	if integrator.Status().Installed {
		t.Skip("an entry is installed; this test only covers the absent case")
	}

	// Removing something that is not there is a no-op, not an error: the app
	// calls this whenever the toggle is turned off, whatever the real state.
	if err := integrator.Remove(); err != nil {
		t.Errorf("removing an absent entry should be harmless, got %v", err)
	}
}

func TestExecutableResolvesToARealFile(t *testing.T) {
	path, err := shellint.Executable()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("resolved path does not exist: %v", err)
	}
	if info.IsDir() {
		t.Error("resolved path is a directory")
	}
}

func trimExt(path string) string {
	if i := strings.LastIndex(path, "."); i > 0 {
		return path[:i]
	}
	return path
}
