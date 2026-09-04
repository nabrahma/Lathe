//go:build windows

package deps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Running an installer that asks for administrator rights cannot be done with
// os/exec. CreateProcess, which is what it calls, refuses such a program
// outright with ERROR_ELEVATION_REQUIRED and never offers the user a choice;
// only ShellExecuteEx with the "runas" verb raises the permission prompt that
// lets someone consent. That is the whole reason this file exists rather than
// a two-line exec.Command.
//
// Passing the arguments as one string is a second, quieter benefit. NSIS reads
// /D as the literal remainder of the command line and rejects quotes around
// it, while os/exec would quote any path containing a space, which is every
// account whose owner has a space in their name.

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

const (
	// seeMaskNoCloseProcess asks for the process handle back, without which
	// there is no way to wait for the installer or read its exit code.
	seeMaskNoCloseProcess = 0x00000040
	// seeMaskNoAsync keeps the call valid for a caller that is not pumping a
	// message loop, which a background goroutine is not.
	seeMaskNoAsync = 0x00000100

	swShowNormal = 1

	waitTimeout = 0x00000102

	// errorCancelled is what Windows returns when the permission prompt is
	// declined or closed.
	errorCancelled = syscall.Errno(1223)
)

// shellExecuteInfoW mirrors SHELLEXECUTEINFOW. The field order and the
// pointer-sized handles are load-bearing: cbSize is checked by the API, and a
// mismatch fails the call rather than corrupting anything.
type shellExecuteInfoW struct {
	cbSize         uint32
	fMask          uint32
	hwnd           uintptr
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       uintptr
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      uintptr
	dwHotKey       uint32
	hIconOrMonitor uintptr
	hProcess       syscall.Handle
}

func runInstaller(ctx context.Context, path string, args []string, timeout time.Duration) error {
	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	params, err := syscall.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return err
	}

	info := shellExecuteInfoW{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		// Shown rather than hidden. These installers are silent, so nothing
		// should appear; if one does put up a dialog anyway, a visible window
		// is a question the user can answer instead of an invisible hang.
		nShow: swShowNormal,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ret, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		// callErr is always non-nil here and carries the real reason, which is
		// ERROR_CANCELLED when the prompt was declined.
		return callErr
	}
	if info.hProcess == 0 {
		return errors.New("the installer started but could not be waited for")
	}
	defer func() { _ = syscall.CloseHandle(info.hProcess) }()

	if err := waitForProcess(ctx, info.hProcess, timeout); err != nil {
		return err
	}

	var code uint32
	if err := syscall.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the installer stopped with code %d", code)
	}
	return nil
}

// waitForProcess blocks until the installer finishes, in slices, so that
// cancelling the job in the interface is noticed rather than ignored for the
// twenty minutes the outer timeout allows.
func waitForProcess(ctx context.Context, h syscall.Handle, timeout time.Duration) error {
	const slice = 250 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("the installer did not finish in time")
		}

		event, err := syscall.WaitForSingleObject(h, uint32(slice.Milliseconds()))
		if err != nil {
			return err
		}
		if event != waitTimeout {
			return nil
		}
	}
}

// isPermissionDeclined reports whether the installer never started because the
// permission prompt was refused, which is a choice rather than a failure.
func isPermissionDeclined(err error) bool {
	return errors.Is(err, errorCancelled)
}
