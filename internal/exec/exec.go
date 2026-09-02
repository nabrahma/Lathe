// Package exec is the only place in Lathe that spawns a process.
//
// Centralising this makes argument escaping, timeouts, cancellation and
// process-tree cleanup provable rather than hopeful. Nothing outside this
// package may import os/exec.
package exec

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"time"
)

// Stream identifies which pipe a line of output arrived on.
type Stream uint8

// The two output streams a running command writes to.
const (
	Stdout Stream = iota
	Stderr
)

func (s Stream) String() string {
	if s == Stderr {
		return "stderr"
	}
	return "stdout"
}

// Options controls how a command is run. The zero value is usable: it applies
// DefaultTimeout and DefaultMaxOutputMB.
type Options struct {
	// Timeout hard-kills the process tree once elapsed. Zero means
	// DefaultTimeout; a negative value means no timeout.
	Timeout time.Duration

	// WorkDir is the process working directory. Empty means the caller's.
	WorkDir string

	// Env is the complete environment for the child. It is never inherited
	// wholesale: LibreOffice alone reads a dozen variables that change its
	// behaviour unpredictably. BaseEnv builds a safe minimum.
	Env []string

	// MaxOutputMB caps captured stdout and stderr. Zero means
	// DefaultMaxOutputMB. A runaway process must not exhaust memory.
	MaxOutputMB int

	// GracefulKill is how long a process gets to exit after a termination
	// signal before it is killed outright. Zero means DefaultGracePeriod.
	GracefulKill time.Duration
}

// Defaults applied when the corresponding Options field is zero.
const (
	DefaultTimeout     = 10 * time.Minute
	DefaultMaxOutputMB = 8
	DefaultGracePeriod = 3 * time.Second
)

// Result describes a finished command. It is returned even when the command
// exited non-zero, so callers can inspect output before deciding what it means.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Duration time.Duration
}

// ErrTimeout reports that a command was killed for exceeding its timeout.
var ErrTimeout = errors.New("command timed out")

// ErrNotFound reports that the binary could not be located or is not
// executable.
var ErrNotFound = errors.New("command not found")

// ExitError reports a non-zero exit.
type ExitError struct {
	Cmd      string
	ExitCode int
	Stderr   string
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if len(msg) > 400 {
		msg = msg[:400] + "..."
	}
	if msg == "" {
		return fmt.Sprintf("%s exited with status %d", e.Cmd, e.ExitCode)
	}
	return fmt.Sprintf("%s exited with status %d: %s", e.Cmd, e.ExitCode, msg)
}

// Runner executes external commands.
type Runner interface {
	// Run executes cmd with args and waits for it to finish. args are passed
	// as a slice and never through a shell, so a filename containing shell
	// metacharacters is inert.
	Run(ctx context.Context, cmd string, args []string, opts Options) (*Result, error)

	// RunStreaming is Run plus a callback invoked for each line of output as
	// it arrives, which is how engines report progress.
	RunStreaming(ctx context.Context, cmd string, args []string, opts Options,
		onLine func(Stream, string)) (*Result, error)
}

// New returns the default Runner.
func New() Runner { return &runner{} }

type runner struct{}

func (r *runner) Run(ctx context.Context, cmd string, args []string, opts Options) (*Result, error) {
	return r.RunStreaming(ctx, cmd, args, opts, nil)
}

func (r *runner) RunStreaming(ctx context.Context, name string, args []string, opts Options,
	onLine func(Stream, string),
) (*Result, error) {
	opts = withDefaults(opts)

	if err := lookup(name); err != nil {
		return nil, err
	}

	runCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// The context is deliberately not handed to CommandContext: its default
	// kill reaches only the direct child. Termination is handled below so the
	// whole process tree goes down together.
	cmd := osexec.Command(name, args...) //nolint:gosec // audited chokepoint; args are never shell-interpolated
	cmd.Dir = opts.WorkDir
	cmd.Env = opts.Env
	configureProcAttr(cmd)

	limit := int64(opts.MaxOutputMB) << 20
	outBuf := &capped{limit: limit}
	errBuf := &capped{limit: limit}

	var wg sync.WaitGroup
	if onLine != nil {
		outPipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		errPipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("stderr pipe: %w", err)
		}
		wg.Add(2)
		go scan(&wg, Stdout, outPipe, outBuf, onLine)
		go scan(&wg, Stderr, errPipe, errBuf, onLine)
	} else {
		cmd.Stdout = outBuf
		cmd.Stderr = errBuf
	}

	started := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}

	tr, err := trackTree(cmd)
	if err != nil {
		_ = terminateTree(cmd, tracker{}, 0)
		_ = cmd.Wait()
		return nil, fmt.Errorf("track %s: %w", name, err)
	}
	defer tr.release()

	done := make(chan error, 1)
	go func() {
		wg.Wait()
		done <- cmd.Wait()
	}()

	var killed bool
	select {
	case err = <-done:
	case <-runCtx.Done():
		killed = true
		_ = terminateTree(cmd, tr, opts.GracefulKill)
		<-done
		err = runCtx.Err()
	}

	res := &Result{
		Stdout:   outBuf.Bytes(),
		Stderr:   errBuf.Bytes(),
		Duration: time.Since(started),
	}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}

	switch {
	case killed && ctx.Err() != nil:
		return res, ctx.Err()
	case killed:
		return res, fmt.Errorf("%w after %s: %s", ErrTimeout, opts.Timeout, name)
	case err != nil:
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			return res, &ExitError{Cmd: name, ExitCode: res.ExitCode, Stderr: string(res.Stderr)}
		}
		return res, fmt.Errorf("run %s: %w", name, err)
	}
	return res, nil
}

// lookup reports whether name is runnable, distinguishing "no such binary"
// from "found but not executable" as clearly as each platform allows.
func lookup(name string) error {
	if !strings.ContainsAny(name, `/\`) {
		if _, err := osexec.LookPath(name); err != nil {
			return fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return nil
	}
	info, err := os.Stat(name)
	if err != nil || info.IsDir() {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if _, err := osexec.LookPath(name); err != nil {
		return fmt.Errorf("%w: %s is not executable", ErrNotFound, name)
	}
	return nil
}

func withDefaults(o Options) Options {
	if o.Timeout == 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxOutputMB == 0 {
		o.MaxOutputMB = DefaultMaxOutputMB
	}
	if o.GracefulKill == 0 {
		o.GracefulKill = DefaultGracePeriod
	}
	if o.Env == nil {
		o.Env = BaseEnv()
	}
	return o
}

func scan(wg *sync.WaitGroup, s Stream, r io.Reader, sink *capped, onLine func(Stream, string)) {
	defer wg.Done()
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			_, _ = sink.Write([]byte(line))
			// FFmpeg separates progress updates with CR, not LF.
			for _, part := range strings.Split(strings.Trim(line, "\r\n"), "\r") {
				if part = strings.TrimSpace(part); part != "" {
					onLine(s, part)
				}
			}
		}
		if err != nil {
			return
		}
	}
}

// capped is a writer that silently discards everything past limit bytes.
type capped struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	limit   int64
	dropped int64
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := c.limit - int64(c.buf.Len())
	switch {
	case room <= 0:
		c.dropped += int64(len(p))
	case int64(len(p)) > room:
		c.buf.Write(p[:room])
		c.dropped += int64(len(p)) - room
	default:
		c.buf.Write(p)
	}
	return len(p), nil
}

func (c *capped) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Bytes()
}

// BaseEnv returns a minimal environment for a child process: only the
// variables external tools genuinely need, with a fixed locale so their output
// is parseable regardless of the user's system language.
func BaseEnv() []string {
	keep := []string{
		"PATH", "HOME", "USERPROFILE", "SystemRoot", "SystemDrive",
		"TEMP", "TMP", "TMPDIR", "windir", "COMSPEC", "PATHEXT",
	}
	env := make([]string, 0, len(keep)+2)
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "LC_ALL=C", "LANG=C")
}
