//go:build !windows

package exec

import (
	osexec "os/exec"
	"syscall"
	"time"
)

// tracker is a no-op on Unix: the process group established in
// configureProcAttr is all the handle termination needs.
type tracker struct{}

func (tracker) release() {}

func configureProcAttr(cmd *osexec.Cmd) {
	// Its own process group, so a signal to -pid reaches every descendant
	// rather than just the child we spawned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func trackTree(*osexec.Cmd) (tracker, error) { return tracker{}, nil }

// terminateTree signals the whole process group, escalating SIGTERM to SIGKILL
// after grace so a process that ignores SIGTERM still dies.
func terminateTree(cmd *osexec.Cmd, _ tracker, grace time.Duration) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := -cmd.Process.Pid

	if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
		// The group may already be gone, or we may not have been able to set
		// it; fall back to the direct child.
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	if grace <= 0 {
		return syscall.Kill(pgid, syscall.SIGKILL)
	}

	deadline := time.After(grace)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if syscall.Kill(pgid, 0) != nil {
				return nil
			}
		case <-deadline:
			if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil {
				return cmd.Process.Kill()
			}
			return nil
		}
	}
}
