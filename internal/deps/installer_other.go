//go:build !windows

package deps

import (
	"context"
	"os/exec"
	"time"
)

// Only Windows has installer-based components today: everywhere else the
// projects that ship a setup program on Windows are a package manager away
// instead, and running a package manager needs a root password Lathe has no
// business asking for. These keep the package building on every platform, and
// give an installer-based component elsewhere an obvious place to land.

func runInstaller(ctx context.Context, path string, args []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).Run()
}

// isPermissionDeclined has no meaning off Windows, where nothing Lathe runs
// raises a permission prompt of its own.
func isPermissionDeclined(error) bool { return false }
