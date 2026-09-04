package deps

import (
	"context"
	"os"
	"testing"
	"time"
)

// Installing a component that raises the Windows permission prompt cannot be
// tested automatically, here or in CI: the prompt is drawn on the secure
// desktop, no program can answer it, and an unanswered one is refused after
// about two minutes. So this is a manual check, run by a person who is looking
// at the screen, and it is skipped otherwise.
//
//	LATHE_LIVE_INSTALL=ghostscript go test ./internal/deps -run TestLiveInstall -v
//
// It uses the real network and the real component directory, because the point
// is to prove the whole path works and a mock would prove nothing.
func TestLiveInstall(t *testing.T) {
	id := os.Getenv("LATHE_LIVE_INSTALL")
	if id == "" {
		t.Skip("set LATHE_LIVE_INSTALL=<component id> and watch for the permission prompt")
	}

	m, err := New("")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	start := time.Now()
	if err := m.Ensure(ctx, id, func(p Progress) {
		if p.BytesTotal == 0 {
			t.Logf("%s", p.Stage)
		}
	}); err != nil {
		t.Fatalf("install %s: %v", id, err)
	}
	t.Logf("installed in %s", time.Since(start).Round(time.Second))

	if !m.Available(id) {
		t.Fatal("the component installed but does not run")
	}
	for _, c := range m.Components() {
		if c.ID != id {
			continue
		}
		for _, b := range c.Binaries {
			p, err := m.BinaryPath(id, b)
			if err != nil {
				t.Fatalf("binary %s: %v", b, err)
			}
			t.Logf("binary %s -> %s", b, p)
		}
	}
}
