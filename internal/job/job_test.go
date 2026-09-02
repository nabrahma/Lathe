package job_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/detect"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/job"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
)

type slowEngine struct {
	delay   time.Duration
	started chan struct{}
	once    sync.Once
}

func (s *slowEngine) ID() string      { return "fake" }
func (s *slowEngine) Available() bool { return true }

func (s *slowEngine) Execute(ctx context.Context, req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
	if s.started != nil {
		s.once.Do(func() { close(s.started) })
	}
	progress(engine.Progress{Fraction: 0.5, Stage: "halfway"})

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	out := req.Workspace.Path("result.pdf")
	if err := os.WriteFile(out, []byte("%PDF-1.7\ndone\n%%EOF\n"), 0o644); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}

func testTask() task.Task {
	return task.Task{
		ID: "test.convert", Name: "Test convert", Verb: "Convert", Engine: "fake",
		Accepts: []detect.Category{detect.CategoryPDF}, MinInputs: 1, MaxInputs: 1,
		Options: []task.Option{{ID: "quality", Type: task.OptionText, Default: "medium"}},
	}
}

func newQueue(t *testing.T, e engine.Engine, concurrency int) *job.Queue {
	t.Helper()
	q := job.NewQueue(pipeline.New(engine.NewRegistry(e), nil), concurrency)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = q.Shutdown(ctx)
	})
	return q
}

func TestAJobRunsToCompletionAndReportsItsOutputs(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	outDir := t.TempDir()
	q := newQueue(t, &slowEngine{}, 1)

	j, err := q.Submit(testTask(), []string{in}, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, q, j.ID, 10*time.Second)

	if final.State != job.Completed {
		t.Fatalf("state %s, error %v", final.State, final.Error)
	}
	if len(final.Outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(final.Outputs))
	}
	if final.Progress != 1 {
		t.Errorf("a completed job should report full progress, got %v", final.Progress)
	}
	if final.FinishedAt == nil || final.StartedAt == nil {
		t.Error("timing fields are needed for the result card")
	}
}

func TestSubmitFillsInTaskDefaults(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	q := newQueue(t, &slowEngine{}, 1)

	j, err := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if j.Options["quality"] != "medium" {
		t.Errorf("defaults were not applied: %v", j.Options)
	}
}

func TestCancellingAJobReportsCancelledAndLeavesNoOutput(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	outDir := t.TempDir()

	started := make(chan struct{})
	q := newQueue(t, &slowEngine{delay: 10 * time.Second, started: started}, 1)

	j, err := q.Submit(testTask(), []string{in}, nil, outDir)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the job never started")
	}

	if err := q.Cancel(j.ID); err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, q, j.ID, 10*time.Second)

	if final.State != job.Cancelled {
		t.Fatalf("state %s, want cancelled", final.State)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("cancelling left files behind: %v", entries)
	}
}

func TestCancellingAQueuedJobNeverStartsIt(t *testing.T) {
	dir := t.TempDir()
	first := writePDF(t, dir, "first.pdf")
	second := writePDF(t, dir, "second.pdf")

	started := make(chan struct{})
	q := newQueue(t, &slowEngine{delay: 3 * time.Second, started: started}, 1)

	blocking, err := q.Submit(testTask(), []string{first}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	<-started

	queued, err := q.Submit(testTask(), []string{second}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Cancel(queued.ID); err != nil {
		t.Fatal(err)
	}

	final := waitForTerminal(t, q, queued.ID, 10*time.Second)
	if final.State != job.Cancelled {
		t.Fatalf("a queued job cancelled before starting should be Cancelled, got %s", final.State)
	}
	_ = q.Cancel(blocking.ID)
}

func TestCancellingAFinishedJobIsNotAnError(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	q := newQueue(t, &slowEngine{}, 1)

	j, _ := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	waitForTerminal(t, q, j.ID, 10*time.Second)

	// The UI cannot know a job finished between render and click.
	if err := q.Cancel(j.ID); err != nil {
		t.Errorf("cancelling a finished job should be a no-op, got %v", err)
	}
	if err := q.Cancel("job-does-not-exist"); err == nil {
		t.Error("cancelling an unknown job should report an error")
	}
}

func TestSubscribersSeeTheJobLifecycle(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	q := newQueue(t, &slowEngine{}, 1)

	events, unsubscribe := q.Subscribe()
	defer unsubscribe()

	j, err := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	seen := map[job.State]bool{}
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Job.ID != j.ID {
				continue
			}
			seen[ev.Job.State] = true
			if ev.Job.State.Terminal() {
				if !seen[job.Queued] {
					t.Error("subscribers should see the job while it is still queued")
				}
				if !seen[job.Completed] {
					t.Errorf("never saw completion; saw %v", seen)
				}
				return
			}
		case <-deadline:
			t.Fatalf("timed out; saw %v", seen)
		}
	}
}

func TestASlowSubscriberDoesNotStallTheQueue(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	q := newQueue(t, &slowEngine{}, 1)

	// Subscribe and never read: a stalled UI must not stall a conversion.
	_, unsubscribe := q.Subscribe()
	defer unsubscribe()

	j, err := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if final := waitForTerminal(t, q, j.ID, 10*time.Second); final.State != job.Completed {
		t.Fatalf("state %s, error %v", final.State, final.Error)
	}
}

func TestConcurrencyIsBounded(t *testing.T) {
	dir := t.TempDir()
	q := newQueue(t, &countingEngine{}, 1)

	var ids []string
	for i := 0; i < 4; i++ {
		in := writePDF(t, dir, filepath.Base(filepath.Join(dir, "in"))+string(rune('a'+i))+".pdf")
		j, err := q.Submit(testTask(), []string{in}, nil, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, j.ID)
	}
	for _, id := range ids {
		waitForTerminal(t, q, id, 20*time.Second)
	}
	if peak := peakConcurrent.Load(); peak > 1 {
		t.Errorf("ran %d jobs at once with a limit of 1", peak)
	}
}

func TestFailureIsReportedAsAUserFacingError(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.pdf")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	q := newQueue(t, &slowEngine{}, 1)

	j, _ := q.Submit(testTask(), []string{empty}, nil, t.TempDir())
	final := waitForTerminal(t, q, j.ID, 10*time.Second)

	if final.State != job.Failed {
		t.Fatalf("state %s, want failed", final.State)
	}
	if final.Error == nil || final.Error.Code != usererr.CodeEmptyInput {
		t.Fatalf("error %+v, want a CodeEmptyInput user error", final.Error)
	}
	if final.Error.Detail == final.Error.Message {
		t.Error("the copyable detail should differ from the message shown")
	}
}

func TestShutdownCancelsEverythingInFlight(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	started := make(chan struct{})
	q := job.NewQueue(pipeline.New(engine.NewRegistry(&slowEngine{delay: 30 * time.Second, started: started}), nil), 1)

	j, err := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := q.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if final, _ := q.Get(j.ID); final.State != job.Cancelled {
		t.Errorf("state after shutdown %s, want cancelled", final.State)
	}
	if q.Active() != 0 {
		t.Errorf("%d jobs still active after shutdown", q.Active())
	}
}

func TestListReturnsSnapshotsNotLiveJobs(t *testing.T) {
	in := writePDF(t, t.TempDir(), "in.pdf")
	q := newQueue(t, &slowEngine{}, 1)

	j, _ := q.Submit(testTask(), []string{in}, nil, t.TempDir())
	waitForTerminal(t, q, j.ID, 10*time.Second)

	list := q.List()
	if len(list) == 0 {
		t.Fatal("expected at least one job")
	}
	list[0].State = "tampered"
	if again, _ := q.Get(j.ID); again.State == "tampered" {
		t.Error("List handed out a live pointer into queue state")
	}
}

func waitForTerminal(t *testing.T, q *job.Queue, id string, limit time.Duration) *job.Job {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		j, ok := q.Get(id)
		if ok && j.State.Terminal() {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s never reached a terminal state", id)
	return nil
}

func writePDF(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("%PDF-1.7\n1 0 obj\n<< >>\nendobj\n%%EOF\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
