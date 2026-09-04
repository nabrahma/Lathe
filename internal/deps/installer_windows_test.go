//go:build windows

package deps

import (
	"fmt"
	"syscall"
	"testing"
	"unsafe"
)

// SHELLEXECUTEINFOW is validated by its own cbSize, so a field added or
// reordered here fails the call rather than being ignored. The sizes are what
// the 64-bit ABI expects.
func TestShellExecuteInfoMatchesTheWindowsLayout(t *testing.T) {
	var info shellExecuteInfoW
	if got, want := unsafe.Sizeof(info), uintptr(112); got != want {
		t.Errorf("SHELLEXECUTEINFOW is %d bytes, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(info.hProcess), uintptr(104); got != want {
		t.Errorf("hProcess sits at %d, want %d", got, want)
	}
}

// Declining the permission prompt has to be told apart from a real failure,
// because the two deserve different things said to the user.
func TestDeclinedPermissionIsRecognised(t *testing.T) {
	// os/exec wraps the errno, so the check has to unwrap rather than match on
	// the message, which is localised on a non-English Windows.
	wrapped := fmt.Errorf("fork/exec setup.exe: %w", errorCancelled)
	if !isPermissionDeclined(wrapped) {
		t.Error("a wrapped cancelled error was not recognised as a declined prompt")
	}
	if !isPermissionDeclined(syscall.Errno(1223)) {
		t.Error("the Windows cancelled error was not recognised as a declined prompt")
	}
	if isPermissionDeclined(syscall.Errno(5)) {
		t.Error("access denied was mistaken for a declined prompt")
	}
}
