// Package engine defines the boundary between a task and the tool that
// actually performs it.
//
// An adapter's job is threefold: translate a task request into the tool's
// argument conventions, parse its progress output, and translate its errors
// into human language. Nothing above this layer knows what a tool's command
// line looks like.
package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/task"
)

// Progress is an engine's report of how far along it is.
type Progress struct {
	// Fraction is 0..1, or -1 when the engine genuinely cannot tell. A fake
	// percentage is worse than an honest indeterminate bar.
	Fraction float64
	// Stage is the user-facing description: "Compressing page 4 of 20".
	Stage string
}

// Indeterminate reports progress with only a stage label.
func Indeterminate(stage string) Progress {
	return Progress{Fraction: -1, Stage: stage}
}

// Request is one unit of work handed to an engine.
type Request struct {
	Task task.Task
	// Inputs are absolute paths, opened read-only. An engine must never write
	// to one.
	Inputs []string
	// Options are the task's option values, already validated and defaulted.
	Options Options
	// Workspace is scratch space. Every file an engine creates goes here; the
	// pipeline publishes the results afterwards.
	Workspace *fsatomic.Workspace
}

// Response is what an engine produced.
type Response struct {
	// Outputs are paths inside the workspace, in the order they should be
	// presented.
	Outputs []string
	// Notes are honest caveats worth surfacing, such as a fidelity warning
	// on a best-effort conversion.
	Notes []string
}

// Engine performs the work for one family of tasks.
type Engine interface {
	// ID matches the Engine field of the tasks it serves.
	ID() string
	// Available reports whether the engine can run right now. An engine whose
	// component has not been downloaded is unavailable, not broken.
	Available() bool
	// Execute performs the request. It must honour ctx cancellation promptly
	// and must not modify any input file.
	Execute(ctx context.Context, req Request, progress func(Progress)) (*Response, error)
}

// Registry resolves an engine name to an implementation.
type Registry struct {
	mu      sync.RWMutex
	engines map[string]Engine
}

// NewRegistry returns a registry holding the given engines.
func NewRegistry(engines ...Engine) *Registry {
	r := &Registry{engines: make(map[string]Engine, len(engines))}
	for _, e := range engines {
		r.Register(e)
	}
	return r
}

// Register adds or replaces an engine.
func (r *Registry) Register(e Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines[e.ID()] = e
}

// Get returns the engine with this ID.
func (r *Registry) Get(id string) (Engine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.engines[id]
	if !ok {
		return nil, fmt.Errorf("no engine registered for %q", id)
	}
	return e, nil
}

// IDs lists the registered engines, sorted.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.engines))
	for id := range r.engines {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
