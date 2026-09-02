// Package job schedules pipeline runs and reports their progress.
//
// Concurrency is deliberately conservative. Media conversion saturates every
// core it is given, and an app that makes the machine unusable while it works
// gets blamed for it.
package job

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
	"github.com/nabrahma/lathe/internal/usererr"
)

// State is where a job is in its life.
type State string

// The job states. A job ends in exactly one of Completed, Failed or Cancelled.
const (
	Queued    State = "queued"
	Preparing State = "preparing"
	Running   State = "running"
	Verifying State = "verifying"
	Completed State = "completed"
	Failed    State = "failed"
	Cancelled State = "cancelled"
)

// Terminal reports whether no further change is possible.
func (s State) Terminal() bool {
	return s == Completed || s == Failed || s == Cancelled
}

// Job is one queued or running unit of work, as the interface sees it.
type Job struct {
	ID     string   `json:"id"`
	TaskID string   `json:"taskId"`
	Name   string   `json:"name"`
	Inputs []string `json:"inputs"`

	Options   map[string]any `json:"options"`
	OutputDir string         `json:"outputDir"`

	State State `json:"state"`
	// Progress is 0..1, or -1 when the engine cannot report a real figure. A
	// bar that jumps to 90% and sits there teaches users to distrust it.
	Progress float64 `json:"progress"`
	// Stage is the user-facing label: "Compressing page 4 of 20".
	Stage string `json:"stage"`

	Outputs []string       `json:"outputs,omitempty"`
	Notes   []string       `json:"notes,omitempty"`
	Error   *usererr.Error `json:"error,omitempty"`

	InputBytes  int64 `json:"inputBytes,omitempty"`
	OutputBytes int64 `json:"outputBytes,omitempty"`

	QueuedAt   time.Time  `json:"queuedAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Duration is how long the job ran, or has been running.
func (j *Job) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	if j.FinishedAt == nil {
		return time.Since(*j.StartedAt)
	}
	return j.FinishedAt.Sub(*j.StartedAt)
}

// Event announces a job change to subscribers.
type Event struct {
	Job *Job `json:"job"`
}

// Queue runs jobs with a bounded number in flight.
type Queue struct {
	runner *pipeline.Runner
	limit  chan struct{}

	mu      sync.RWMutex
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
	order   []string

	subMu sync.Mutex
	subs  map[int]chan Event
	nextS int

	seq  atomic.Uint64
	wg   sync.WaitGroup
	done chan struct{}
	once sync.Once
}

// DefaultConcurrency is the number of jobs allowed to run at once.
func DefaultConcurrency() int {
	n := runtime.NumCPU() / 2
	if n > 2 {
		n = 2
	}
	if n < 1 {
		n = 1
	}
	return n
}

// NewQueue returns a queue running at most concurrency jobs at a time. A
// concurrency below one is corrected to DefaultConcurrency.
func NewQueue(runner *pipeline.Runner, concurrency int) *Queue {
	if concurrency < 1 {
		concurrency = DefaultConcurrency()
	}
	return &Queue{
		runner:  runner,
		limit:   make(chan struct{}, concurrency),
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
		subs:    make(map[int]chan Event),
		done:    make(chan struct{}),
	}
}

// Submit queues a job and returns it immediately in the Queued state.
func (q *Queue) Submit(t task.Task, inputs []string, opts map[string]any, outputDir string) (*Job, error) {
	select {
	case <-q.done:
		return nil, fmt.Errorf("the queue is shutting down")
	default:
	}

	if opts == nil {
		opts = map[string]any{}
	}
	// Task defaults fill any gap, so a job submitted with no options at all
	// still behaves the way the task screen would.
	merged := t.Defaults()
	for k, v := range opts {
		merged[k] = v
	}

	j := &Job{
		ID:        fmt.Sprintf("job-%d", q.seq.Add(1)),
		TaskID:    t.ID,
		Name:      t.Name,
		Inputs:    append([]string(nil), inputs...),
		Options:   merged,
		OutputDir: outputDir,
		State:     Queued,
		Progress:  -1,
		QueuedAt:  time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	q.mu.Lock()
	q.jobs[j.ID] = j
	q.cancels[j.ID] = cancel
	q.order = append(q.order, j.ID)
	q.mu.Unlock()

	q.publish(j)

	q.wg.Add(1)
	go q.run(ctx, t, j.ID)
	return q.snapshot(j.ID), nil
}

func (q *Queue) run(ctx context.Context, t task.Task, id string) {
	defer q.wg.Done()
	defer func() {
		q.mu.Lock()
		delete(q.cancels, id)
		q.mu.Unlock()
	}()

	// Wait for a slot, but stay cancellable while queued.
	select {
	case q.limit <- struct{}{}:
		defer func() { <-q.limit }()
	case <-ctx.Done():
		q.finishCancelled(id)
		return
	}

	if ctx.Err() != nil {
		q.finishCancelled(id)
		return
	}

	started := time.Now()
	q.update(id, func(j *Job) {
		j.State = Preparing
		j.StartedAt = &started
	})

	res, err := q.runner.Run(ctx, pipeline.Request{
		Task:      t,
		Inputs:    q.inputsOf(id),
		Options:   q.optionsOf(id),
		OutputDir: q.outputDirOf(id),
		Progress: func(p engine.Progress) {
			q.update(id, func(j *Job) {
				if j.State == Preparing || j.State == Queued {
					j.State = Running
				}
				if p.Stage == string(pipeline.StageVerify) {
					j.State = Verifying
				}
				j.Progress = p.Fraction
				if p.Stage != "" {
					j.Stage = p.Stage
				}
			})
		},
	})

	finished := time.Now()
	switch {
	case ctx.Err() != nil:
		q.finishCancelled(id)
	case err != nil:
		q.update(id, func(j *Job) {
			j.State = Failed
			j.Error = usererr.Translate(err)
			j.Stage = ""
			j.FinishedAt = &finished
		})
	default:
		q.update(id, func(j *Job) {
			j.State = Completed
			j.Progress = 1
			j.Stage = ""
			j.Outputs = res.Outputs
			j.Notes = res.Notes
			j.InputBytes = res.InputBytes
			j.OutputBytes = res.OutputBytes
			j.FinishedAt = &finished
		})
	}
}

func (q *Queue) finishCancelled(id string) {
	now := time.Now()
	q.update(id, func(j *Job) {
		if j.State.Terminal() {
			return
		}
		j.State = Cancelled
		j.Stage = ""
		j.Progress = -1
		j.FinishedAt = &now
	})
}

// Cancel stops a job. Cancelling an already-finished job is a no-op, not an
// error, because the UI cannot know the job finished between render and click.
func (q *Queue) Cancel(id string) error {
	q.mu.RLock()
	cancel, running := q.cancels[id]
	_, known := q.jobs[id]
	q.mu.RUnlock()

	if !known {
		return fmt.Errorf("no such job %q", id)
	}
	if running {
		cancel()
	}
	return nil
}

// CancelAll stops every unfinished job, which is what quitting does.
func (q *Queue) CancelAll() {
	q.mu.RLock()
	cancels := make([]context.CancelFunc, 0, len(q.cancels))
	for _, c := range q.cancels {
		cancels = append(cancels, c)
	}
	q.mu.RUnlock()

	for _, c := range cancels {
		c()
	}
}

// Get returns a copy of a job.
func (q *Queue) Get(id string) (*Job, bool) {
	j := q.snapshot(id)
	return j, j != nil
}

// List returns every job, newest first.
func (q *Queue) List() []*Job {
	q.mu.RLock()
	ids := append([]string(nil), q.order...)
	q.mu.RUnlock()

	out := make([]*Job, 0, len(ids))
	for _, id := range ids {
		if j := q.snapshot(id); j != nil {
			out = append(out, j)
		}
	}
	sort.SliceStable(out, func(i, k int) bool { return out[i].QueuedAt.After(out[k].QueuedAt) })
	return out
}

// Active reports how many jobs have not finished.
func (q *Queue) Active() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	n := 0
	for _, j := range q.jobs {
		if !j.State.Terminal() {
			n++
		}
	}
	return n
}

// Subscribe returns a channel of job events and a function to stop listening.
// Events are dropped rather than blocking a slow subscriber: the UI can always
// re-read current state, and a stalled render must never stall a conversion.
func (q *Queue) Subscribe() (<-chan Event, func()) {
	q.subMu.Lock()
	defer q.subMu.Unlock()

	id := q.nextS
	q.nextS++
	ch := make(chan Event, 64)
	q.subs[id] = ch

	return ch, func() {
		q.subMu.Lock()
		defer q.subMu.Unlock()
		if c, ok := q.subs[id]; ok {
			delete(q.subs, id)
			close(c)
		}
	}
}

// Shutdown cancels everything in flight and waits for it to unwind, so quitting
// leaves no orphaned engine processes behind.
func (q *Queue) Shutdown(ctx context.Context) error {
	q.once.Do(func() { close(q.done) })
	q.CancelAll()

	finished := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(finished)
	}()

	select {
	case <-finished:
	case <-ctx.Done():
		return ctx.Err()
	}

	q.subMu.Lock()
	for id, ch := range q.subs {
		delete(q.subs, id)
		close(ch)
	}
	q.subMu.Unlock()
	return nil
}

func (q *Queue) update(id string, mutate func(*Job)) {
	q.mu.Lock()
	j, ok := q.jobs[id]
	if ok {
		mutate(j)
	}
	q.mu.Unlock()

	if ok {
		q.publish(j)
	}
}

func (q *Queue) publish(j *Job) {
	snap := q.snapshot(j.ID)
	if snap == nil {
		return
	}

	q.subMu.Lock()
	defer q.subMu.Unlock()
	for _, ch := range q.subs {
		select {
		case ch <- Event{Job: snap}:
		default:
		}
	}
}

// snapshot returns a deep-enough copy that callers cannot observe a job
// mutating underneath them.
func (q *Queue) snapshot(id string) *Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	j, ok := q.jobs[id]
	if !ok {
		return nil
	}
	clone := *j
	clone.Inputs = append([]string(nil), j.Inputs...)
	clone.Outputs = append([]string(nil), j.Outputs...)
	clone.Notes = append([]string(nil), j.Notes...)
	clone.Options = make(map[string]any, len(j.Options))
	for k, v := range j.Options {
		clone.Options[k] = v
	}
	return &clone
}

func (q *Queue) inputsOf(id string) []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return append([]string(nil), q.jobs[id].Inputs...)
}

func (q *Queue) optionsOf(id string) engine.Options {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make(engine.Options, len(q.jobs[id].Options))
	for k, v := range q.jobs[id].Options {
		out[k] = v
	}
	return out
}

func (q *Queue) outputDirOf(id string) string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.jobs[id].OutputDir
}
