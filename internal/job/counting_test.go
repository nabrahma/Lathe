package job_test

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/nabrahma/lathe/internal/engine"
)

var (
	concurrent     atomic.Int32
	peakConcurrent atomic.Int32
)

// countingEngine records the highest number of simultaneous executions, which
// is how the concurrency limit is verified.
type countingEngine struct{}

func (c *countingEngine) ID() string      { return "fake" }
func (c *countingEngine) Available() bool { return true }

func (c *countingEngine) Execute(ctx context.Context, req engine.Request, _ func(engine.Progress)) (*engine.Response, error) {
	now := concurrent.Add(1)
	for {
		peak := peakConcurrent.Load()
		if now <= peak || peakConcurrent.CompareAndSwap(peak, now) {
			break
		}
	}
	defer concurrent.Add(-1)

	select {
	case <-time.After(80 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	out := req.Workspace.Path("result.pdf")
	if err := os.WriteFile(out, []byte("%PDF-1.7\ndone\n%%EOF\n"), 0o644); err != nil {
		return nil, err
	}
	return &engine.Response{Outputs: []string{out}}, nil
}
