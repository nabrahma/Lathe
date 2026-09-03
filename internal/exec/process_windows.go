package exec

import (
	"fmt"
	osexec "os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tracker holds the Job Object the child was assigned to. Windows has no
// process groups in the Unix sense, so a Job Object is the only reliable way
// to reach grandchildren, which matters because both FFmpeg and LibreOffice
// spawn them.
type tracker struct {
	job windows.Handle
}

func (t tracker) release() {
	if t.job != 0 {
		// KILL_ON_JOB_CLOSE means closing the last handle also terminates
		// anything still running in the job.
		_ = windows.CloseHandle(t.job)
	}
}

func configureProcAttr(cmd *osexec.Cmd) {
	// A new process group keeps console signals from propagating to Lathe
	// itself; the Job Object does the actual tree cleanup.
	cmd.SysProcAttr = &windows.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func trackTree(cmd *osexec.Cmd) (tracker, error) {
	if cmd.Process == nil {
		return tracker{}, nil
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return tracker{}, fmt.Errorf("create job object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return tracker{}, fmt.Errorf("configure job object: %w", err)
	}

	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE
	proc, err := windows.OpenProcess(access, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return tracker{}, fmt.Errorf("open process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return tracker{}, fmt.Errorf("assign to job object: %w", err)
	}
	return tracker{job: job}, nil
}

// terminateTree kills every process in the job. Windows offers no graceful
// equivalent of SIGTERM for a non-console child, so grace is spent waiting for
// a voluntary exit before the job is torn down.
func terminateTree(cmd *osexec.Cmd, t tracker, grace time.Duration) error {
	if cmd.Process == nil {
		return nil
	}

	if grace > 0 {
		_ = windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid))

		deadline := time.After(grace)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
	wait:
		for {
			select {
			case <-tick.C:
				if cmd.ProcessState != nil {
					return nil
				}
			case <-deadline:
				break wait
			}
		}
	}

	if t.job != 0 {
		if err := windows.TerminateJobObject(t.job, 1); err == nil {
			return nil
		}
	}
	return cmd.Process.Kill()
}
