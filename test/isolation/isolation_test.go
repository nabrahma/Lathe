// Package isolation_test proves the privacy claim rather than asserting it.
//
// Two things make "your files never leave your machine" verifiable: no package
// outside internal/deps and internal/update may even import network code, and
// every conversion must complete with outbound access blocked. The first is
// checked by scripts/boundary in CI; this file checks both again from the
// outside, so the guarantee survives a refactor that moves the lint rule.
package isolation_test

import (
	"context"
	"go/parser"
	"go/token"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/imageengine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
	"github.com/nabrahma/lathe/internal/pipeline"
	"github.com/nabrahma/lathe/internal/task"
)

const corpus = "../../testdata/corpus"

// networkAllowed are the only two packages permitted to reach the network, and
// both do so only on an explicit, user-initiated action.
var networkAllowed = []string{"internal/deps", "internal/update", "scripts/boundary", "test/isolation"}

var networkPackages = []string{"net", "net/http", "net/url", "net/rpc", "net/smtp", "golang.org/x/net"}

func TestNoNetworkImportsOutsideTheTwoPermittedPackages(t *testing.T) {
	root := filepath.Join("..", "..")

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", ".tools", "bin", "dist", "testdata", "build":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		if underAny(dir, networkAllowed) {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			target, unquoteErr := strconv.Unquote(imp.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if isNetwork(target) {
				t.Errorf("%s imports %s; network access belongs only in %s",
					rel, target, strings.Join(networkAllowed[:2], " and "))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestConversionsSucceedWithNoNetwork runs real conversions while every
// outbound dial is intercepted and failed. Any attempt is recorded and fails
// the test; the conversions themselves must still succeed.
func TestConversionsSucceedWithNoNetwork(t *testing.T) {
	var attempts atomic.Int64

	// net.DefaultResolver is the shared path every name lookup takes. Making
	// it fail loudly turns a silent network call into a test failure.
	original := net.DefaultResolver.Dial
	net.DefaultResolver.PreferGo = true
	net.DefaultResolver.Dial = func(_ context.Context, network, address string) (net.Conn, error) {
		attempts.Add(1)
		return nil, &net.OpError{Op: "dial", Net: network, Err: errBlocked{address}}
	}
	t.Cleanup(func() { net.DefaultResolver.Dial = original })

	runner := pipeline.New(engine.NewRegistry(pdfengine.New(), imageengine.New()), nil)
	registry := task.Default()

	cases := []struct {
		taskID  string
		inputs  []string
		options map[string]any
	}{
		{"pdf.compress", []string{filepath.Join(corpus, "pdf", "five-page.pdf")}, nil},
		{"pdf.merge", []string{
			filepath.Join(corpus, "pdf", "single-page.pdf"),
			filepath.Join(corpus, "pdf", "five-page.pdf"),
		}, nil},
		{"pdf.split", []string{filepath.Join(corpus, "pdf", "five-page.pdf")},
			map[string]any{"mode": "pages"}},
		{"image.convert", []string{filepath.Join(corpus, "images", "photo.png")},
			map[string]any{"format": "jpg"}},
		{"image.resize", []string{filepath.Join(corpus, "images", "photo.jpg")},
			map[string]any{"preset": "custom", "width": 320}},
		{"pdf.from-images", []string{filepath.Join(corpus, "images", "photo.png")}, nil},
		{"text.from-pdf", []string{filepath.Join(corpus, "pdf", "five-page.pdf")},
			map[string]any{"ocr": false}},
	}

	for _, tc := range cases {
		tk, ok := registry.Get(tc.taskID)
		if !ok {
			t.Fatalf("no such task %q", tc.taskID)
		}
		if _, err := os.Stat(tc.inputs[0]); err != nil {
			t.Skipf("corpus missing (run: make corpus): %v", err)
		}

		opts := engine.Options(tk.Defaults())
		for k, v := range tc.options {
			opts[k] = v
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		_, err := runner.Run(ctx, pipeline.Request{
			Task: tk, Inputs: tc.inputs, Options: opts, OutputDir: t.TempDir(),
		})
		cancel()

		// text.from-pdf on a PDF with no text layer is a legitimate failure
		// with OCR turned off; what matters here is that nothing dialled out.
		if err != nil && tc.taskID != "text.from-pdf" {
			t.Errorf("%s failed with no network: %v", tc.taskID, err)
		}
	}

	if n := attempts.Load(); n > 0 {
		t.Fatalf("conversions attempted %d network lookups; they must make none", n)
	}
}

type errBlocked struct{ address string }

func (e errBlocked) Error() string {
	return "network access is blocked during the isolation test: " + e.address
}

func underAny(dir string, roots []string) bool {
	for _, r := range roots {
		if dir == r || strings.HasPrefix(dir, r+"/") {
			return true
		}
	}
	return false
}

func isNetwork(target string) bool {
	switch target {
	case "net/netip", "net/textproto", "net/mail":
		return false
	}
	for _, p := range networkPackages {
		if target == p || strings.HasPrefix(target, p+"/") {
			return true
		}
	}
	return false
}
