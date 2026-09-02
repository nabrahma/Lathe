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
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
