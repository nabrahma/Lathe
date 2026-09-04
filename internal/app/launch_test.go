package app

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Open and Reveal will only touch a file this session produced, so the
// comparison behind that decides whether the buttons work at all. Too strict
// and a legitimate result is refused; too loose and the allowlist is
// decoration.
func TestSameFileMatchesTheWayTheFilesystemDoes(t *testing.T) {
	dir := t.TempDir()
	stored := filepath.Join(dir, "merged.pdf")

	if !sameFile(stored, stored) {
		t.Error("a path did not match itself")
	}
	if !sameFile(stored, filepath.Clean(stored)) {
		t.Error("cleaning a path stopped it matching")
	}
	if sameFile(stored, filepath.Join(dir, "other.pdf")) {
		t.Error("two different files were treated as one")
	}

	// A traversal that lands somewhere else must not be accepted just because
	// it is spelled with the allowed directory in it.
	escape := filepath.Join(dir, "..", "elsewhere.pdf")
	if sameFile(stored, escape) {
		t.Error("a path climbing out of the folder was accepted")
	}

	if runtime.GOOS == "windows" {
		// Windows paths are case-insensitive, so a result the user is looking
		// at must not be refused over the spelling of a drive letter.
		shouted := filepath.Join(dir, "MERGED.PDF")
		if !sameFile(stored, shouted) {
			t.Error("the same file in different case was refused on Windows")
		}
	}
}
