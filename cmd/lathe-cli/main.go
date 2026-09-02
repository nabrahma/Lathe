// Command lathe is the headless interface to the same task registry the
// desktop app drives, so anything Lathe can do is scriptable.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nabrahma/lathe/internal/cli"
	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engines"
	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
)

func main() {
	// os.Exit skips deferred calls, so all cleanup happens inside run.
	os.Exit(run())
}

func run() int {
	// Ctrl-C must cancel the job cleanly rather than orphaning an engine.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Clear workspaces left by a previous run that was killed.
	_, _ = fsatomic.CleanOrphans(defaultOrphanAge)

	manager, err := deps.New("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "lathe:", err)
		return 1
	}

	app := &cli.App{
		Registry: task.Default(),
		Runner:   pipeline.New(engines.Default(manager), manager),
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}
	return app.Run(ctx, os.Args[1:])
}
