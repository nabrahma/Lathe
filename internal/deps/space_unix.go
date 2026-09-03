//go:build !windows

package deps

import "golang.org/x/sys/unix"

// freeSpace reports the bytes available to this user on the filesystem holding
// dir. Blocks available to an unprivileged user, not total free blocks: the
// difference is the reserved root percentage, which Lathe cannot use.
func freeSpace(dir string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, err
	}
	// Bsize is int64 on Linux and uint32 on macOS, so this conversion is
	// redundant on the platform the linter runs on and load-bearing on the
	// other.
	return int64(stat.Bavail) * int64(stat.Bsize), nil //nolint:unconvert // keeps macOS compiling
}
