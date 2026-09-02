package exec_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/exec"
)

// The tests drive a helper mode of this same test binary rather than depending
// on shell utilities, which differ across the three target platforms.
const helperEnv = "LATHE_EXEC_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "":
		os.Exit(m.Run())
	case "echo-args":
		for _, a := range os.Args[1:] {
			if strings.HasPrefix(a, "-test.") {
				continue
			}
			fmt.Println(a)
		}
	case "sleep":
		time.Sleep(30 * time.Second)
	case "spew":
		line := strings.Repeat("x", 1024) + "\n"
		for i := 0; i < 64*1024; i++ {
			fmt.Print(line)
		}
	case "spawn":
		// Spawn a grandchild that heartbeats to a file, then block. Killing
		// only the direct child would leave the grandchild writing.
		cmd := osexec.Command(os.Args[0]) //nolint:gosec // test helper
		cmd.Env = append(os.Environ(), helperEnv+"=heartbeat")
		if err := cmd.Start(); err != nil {
			os.Exit(1)
		}
		time.Sleep(30 * time.Second)
	case "heartbeat":
		path := os.Getenv("LATHE_HEARTBEAT")
		for i := 0; i < 600; i++ {
			_ = os.WriteFile(path, []byte(time.Now().String()), 0o600)
			time.Sleep(50 * time.Millisecond)
		}
	case "stream":
		fmt.Println("out-one")
		fmt.Fprintln(os.Stderr, "err-one")
		fmt.Print("progress=1\rprogress=2\rprogress=3\n")
	case "fail":
		fmt.Fprintln(os.Stderr, "something went wrong")
		os.Exit(3)
	}
	os.Exit(0)
}

func helper(mode string, extra ...string) (string, []string, []string) {
	env := append(exec.BaseEnv(), helperEnv+"="+mode)
	env = append(env, extra...)
	return os.Args[0], nil, env
}

func TestRunPassesArgumentsWithoutShellInterpretation(t *testing.T) {
	// Every one of these is inert only because args are passed as a slice.
	adversarial := []string{
		"plain.pdf",
		"name with spaces.pdf",
		`quo"te.pdf`,
		"single'quote.pdf",
		"; rm -rf ~",
		"$(whoami).pdf",
		"`id`.pdf",
		"a&b|c.pdf",
		"emoji-\U0001F600.pdf",
		"unicode-café-日本語.pdf",
		strings.Repeat("long", 60) + ".pdf",
	}

	bin, _, env := helper("echo-args")
	res, err := exec.New().Run(context.Background(), bin, adversarial, exec.Options{Env: env})
	if err != nil {
		t.Fatalf("run: %v (stderr: %s)", err, res.Stderr)
	}

	got := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(res.Stdout), "\r\n", "\n")), "\n")
	if len(got) != len(adversarial) {
		t.Fatalf("got %d args back, want %d: %q", len(got), len(adversarial), got)
	}
	for i, want := range adversarial {
		if got[i] != want {
			t.Errorf("arg %d: got %q, want %q", i, got[i], want)
		}
	}
}

func TestCancellationKillsTheWholeProcessTree(t *testing.T) {
	beat := filepath.Join(t.TempDir(), "heartbeat")
	bin, _, env := helper("spawn", "LATHE_HEARTBEAT="+beat)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := exec.New().Run(ctx, bin, nil, exec.Options{Env: env, GracefulKill: 200 * time.Millisecond})
		errCh <- err
	}()

	// Wait for the grandchild to prove it is alive before killing anything.
	waitFor(t, 5*time.Second, func() bool {
		_, err := os.Stat(beat)
		return err == nil
	}, "grandchild never started heartbeating")

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	// Give any survivor time to write, then assert the file stopped changing.
	time.Sleep(500 * time.Millisecond)
	before := mtime(t, beat)
	time.Sleep(700 * time.Millisecond)
	if after := mtime(t, beat); !after.Equal(before) {
		t.Fatal("grandchild survived cancellation: heartbeat file is still being written")
	}
}

func TestTimeoutKillsAndReportsErrTimeout(t *testing.T) {
	bin, _, env := helper("sleep")
	start := time.Now()
	_, err := exec.New().Run(context.Background(), bin, nil, exec.Options{
		Env:          env,
		Timeout:      300 * time.Millisecond,
		GracefulKill: 100 * time.Millisecond,
	})
	if !errors.Is(err, exec.ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("timeout took %s to take effect", elapsed)
	}
}

func TestOutputIsCappedRatherThanUnbounded(t *testing.T) {
	bin, _, env := helper("spew")
	res, err := exec.New().Run(context.Background(), bin, nil, exec.Options{Env: env, MaxOutputMB: 1})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := len(res.Stdout); got > 1<<20 {
		t.Fatalf("captured %d bytes, want at most %d", got, 1<<20)
	}
}

func TestStreamingSplitsCarriageReturnProgress(t *testing.T) {
	bin, _, env := helper("stream")
	var lines []string
	_, err := exec.New().RunStreaming(context.Background(), bin, nil, exec.Options{Env: env},
		func(_ exec.Stream, line string) { lines = append(lines, line) })
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{"out-one", "err-one", "progress=1", "progress=2", "progress=3"}
	for _, w := range want {
		if !contains(lines, w) {
			t.Errorf("missing streamed line %q; got %q", w, lines)
		}
	}
}

func TestNonZeroExitReturnsExitError(t *testing.T) {
	bin, _, env := helper("fail")
	res, err := exec.New().Run(context.Background(), bin, nil, exec.Options{Env: env})

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("got %v, want *exec.ExitError", err)
	}
	if ee.ExitCode != 3 {
		t.Errorf("exit code %d, want 3", ee.ExitCode)
	}
	if !strings.Contains(ee.Error(), "something went wrong") {
		t.Errorf("error text lost stderr: %q", ee.Error())
	}
	if res.ExitCode != 3 {
		t.Errorf("result exit code %d, want 3", res.ExitCode)
	}
}

func TestMissingBinaryIsDistinguishable(t *testing.T) {
	_, err := exec.New().Run(context.Background(), "lathe-no-such-binary-9c1f", nil, exec.Options{})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}

	dir := t.TempDir()
	if _, err := exec.New().Run(context.Background(), dir, nil, exec.Options{}); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("directory as binary: got %v, want ErrNotFound", err)
	}
}

func TestBaseEnvDoesNotLeakTheParentEnvironment(t *testing.T) {
	t.Setenv("LATHE_SHOULD_NOT_LEAK", "1")
	for _, kv := range exec.BaseEnv() {
		if strings.HasPrefix(kv, "LATHE_SHOULD_NOT_LEAK=") {
			t.Fatal("BaseEnv passed through an unrelated variable")
		}
	}
	if !contains(exec.BaseEnv(), "LC_ALL=C") {
		t.Error("BaseEnv should pin the locale so tool output is parseable")
	}
}

func waitFor(t *testing.T, limit time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal(msg)
}

func mtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
