// Package fsatomic guarantees that a job either produces a complete output
// file or leaves the filesystem exactly as it found it.
//
// Two rules hold everywhere in Lathe: an input file is opened read-only, and
// an output file appears at its destination only once it is complete. This
// package is how the second rule is kept.
package fsatomic

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultPerm is the mode for files Lathe creates on behalf of a user.
const DefaultPerm os.FileMode = 0o644

// tempPrefix marks Lathe's in-progress files so a crash leaves something
// recognisable to clean up rather than mysterious debris in someone's folder.
const tempPrefix = ".lathe-tmp-"

// WriteFile writes through a temp file in the destination's own directory,
// flushes it to disk, and renames it into place.
//
// The temp file must be a sibling of dst: os.Rename is atomic only within one
// filesystem, and the system temp directory is frequently on another, where Go
// silently degrades the rename to copy-then-delete.
func WriteFile(dst string, write func(w io.Writer) error, perm os.FileMode) (err error) {
	if perm == 0 {
		perm = DefaultPerm
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return fmt.Errorf("create temp file beside %s: %w", filepath.Base(dst), err)
	}
	tmpName := tmp.Name()

	// Any failure past this point must leave nothing behind.
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = write(tmp); err != nil {
		return err
	}
	// Without the sync, a power loss after rename can leave a correctly named
	// file with no contents.
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("flush %s: %w", filepath.Base(dst), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(dst), err)
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}
	if err = os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("publish %s: %w", filepath.Base(dst), err)
	}
	return nil
}

// Publish moves an already-written file (typically produced by an external
// engine inside a Workspace) to its destination atomically, falling back to a
// copy when the two paths are on different filesystems.
func Publish(src, dst string, perm os.FileMode) error {
	if perm == 0 {
		perm = DefaultPerm
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := os.Rename(src, dst); err == nil {
		return os.Chmod(dst, perm)
	}

	// Cross-device: copy through a sibling temp file so the destination still
	// appears atomically.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open result: %w", err)
	}
	defer func() { _ = in.Close() }()

	if err := WriteFile(dst, func(w io.Writer) error {
		_, copyErr := io.Copy(w, in)
		return copyErr
	}, perm); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

// Workspace is a scratch directory for one job. Engines write their
// intermediate and final files here, and only completed results are published
// to the user's chosen destination.
type Workspace struct {
	dir    string
	closed bool
	mu     sync.Mutex
}

// TempWorkspace creates a scratch directory under the Lathe temp root. purpose
// appears in the directory name so a leftover directory is self-explaining.
func TempWorkspace(purpose string) (*Workspace, error) {
	root, err := tempRoot()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, sanitize(purpose)+"-")
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	return &Workspace{dir: dir}, nil
}

// Dir is the workspace root.
func (w *Workspace) Dir() string { return w.dir }

// Path returns a path for a named file inside the workspace. The name is
// sanitised, so a hostile input filename cannot escape the workspace.
func (w *Workspace) Path(name string) string {
	return filepath.Join(w.dir, sanitize(name))
}

// UniqueName returns a workspace path that no file occupies yet. Batch tasks
// hit this constantly: photo.png, photo.jpg and photo.bmp converted together
// all want to be photo.png, and silently overwriting would return one result
// where the user asked for three.
func (w *Workspace) UniqueName(name string) string {
	name = sanitize(name)
	ext := filepath.Ext(name)
	return UniquePath(w.dir, strings.TrimSuffix(name, ext), ext)
}

// Sub creates and returns a subdirectory, for multi-step plans that need to
// keep stages apart.
func (w *Workspace) Sub(name string) (string, error) {
	p := w.Path(name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", fmt.Errorf("create workspace subdirectory: %w", err)
	}
	return p, nil
}

// Close removes the workspace and everything in it. It is safe to call twice.
func (w *Workspace) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return os.RemoveAll(w.dir)
}

// CleanOrphans removes workspaces left behind by a previous run that crashed.
// Anything older than maxAge is assumed dead; a conservative age avoids
// deleting a workspace belonging to a second running instance.
func CleanOrphans(maxAge time.Duration) (removed int, err error) {
	root, err := tempRoot()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		info, statErr := e.Info()
		if statErr != nil || info.ModTime().After(cutoff) {
			continue
		}
		if os.RemoveAll(filepath.Join(root, e.Name())) == nil {
			removed++
		}
	}
	return removed, nil
}

// UniquePath resolves a collision by suffixing rather than overwriting:
// report.pdf becomes report (1).pdf. Silently replacing a user's file is the
// one thing this package exists to prevent.
func UniquePath(dir, base, ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	base = sanitize(base)

	candidate := filepath.Join(dir, base+ext)
	if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate
	}
	for i := 1; i < 10000; i++ {
		candidate = filepath.Join(dir, base+" ("+strconv.Itoa(i)+")"+ext)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	// Astronomically unlikely; a timestamp is still better than clobbering.
	return filepath.Join(dir, base+"-"+strconv.FormatInt(time.Now().UnixNano(), 36)+ext)
}

// CheckWritable reports, before a job starts, whether results can actually be
// written to dir, so the failure arrives up front rather than after a
// five-minute conversion.
func CheckWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("output folder is not available: %w", err)
	}
	if !info.IsDir() {
		return errors.New("the output path is a file, not a folder")
	}

	probe, err := os.CreateTemp(dir, tempPrefix+"probe-*")
	if err != nil {
		return fmt.Errorf("output folder is not writable: %w", err)
	}
	name := probe.Name()
	_ = probe.Close()
	return os.Remove(name)
}

func tempRoot() (string, error) {
	root := filepath.Join(os.TempDir(), "lathe")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create temp root: %w", err)
	}
	return root, nil
}

// sanitize reduces a name to something safe to join onto a directory: no
// separators, no traversal, no reserved characters, and never empty.
func sanitize(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, r == 0x7f:
			return -1
		case strings.ContainsRune(`:*?"<>|`, r):
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")

	if name == "" {
		return "file"
	}
	// Windows refuses these names regardless of extension.
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "_" + name
	}
	// Leave room for a " (12)" suffix and an extension within common limits.
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
