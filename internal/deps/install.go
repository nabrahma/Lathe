package deps

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz"

	lexec "github.com/nabrahma/lathe/internal/exec"
	"github.com/nabrahma/lathe/internal/usererr"
	"github.com/nabrahma/lathe/internal/version"
)

// downloadTimeout is generous: LibreOffice is several hundred megabytes and
// people are not always on fast connections.
const downloadTimeout = 2 * time.Hour

// probeTimeout bounds the "does this binary run" check.
const probeTimeout = 20 * time.Second

// Ensure downloads, verifies and installs a component.
func (m *manager) Ensure(ctx context.Context, id string, progress func(Progress)) error {
	c, ok := m.components[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownComponent, id)
	}
	if progress == nil {
		progress = func(Progress) {}
	}

	// One installer per component at a time, across every window.
	lock := m.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	if m.probeComponent(ctx, c).usable {
		return nil
	}

	if c.SystemOnly {
		// Nothing to download: this component is detected, not installed by
		// Lathe. Say what to install rather than failing silently.
		hint := c.Hint()
		if hint == "" {
			hint = "Install it, then restart Lathe."
		}
		return usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s isn't installed on this computer. %s", c.DisplayName, hint),
			usererr.ActionCopyDetails)
	}

	src, ok := c.Sources[platformKey()]
	if !ok {
		src, ok = c.Sources["any"]
	}
	if !ok {
		return usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s isn't available for this kind of computer yet.", c.DisplayName),
			usererr.ActionCopyDetails)
	}

	if err := checkSpace(m.root, c.DownloadBytes+c.InstalledBytes); err != nil {
		return err
	}

	staging := m.dirOf(id) + ".downloading"
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return usererr.Wrap(err, usererr.CodeNotWritable,
			"Lathe couldn't create a folder for the download.", usererr.ActionRetry)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	archive := filepath.Join(staging, "download"+archiveExt(src.URL))
	if err := download(ctx, src, archive, c, progress); err != nil {
		return err
	}

	progress(Progress{ComponentID: id, Stage: "Checking the download", Fraction: -1})
	if err := verifyChecksum(archive, src.SHA256); err != nil {
		// A file that does not match is deleted, never used.
		_ = os.Remove(archive)
		return err
	}

	progress(Progress{ComponentID: id, Stage: "Installing", Fraction: -1})
	unpacked := filepath.Join(staging, "unpacked")
	if err := extract(archive, unpacked, src.StripPrefix); err != nil {
		return err
	}

	// Atomic install: a partially extracted component must never be visible as
	// installed, so the final directory appears in one move.
	final := m.dirOf(id)
	_ = os.RemoveAll(final)
	if err := os.Rename(unpacked, final); err != nil {
		return usererr.Wrap(err, usererr.CodeNotWritable,
			fmt.Sprintf("%s downloaded but couldn't be installed.", c.DisplayName),
			usererr.ActionRetry)
	}

	m.forget(id)
	if p := m.probeComponent(ctx, c); !p.usable {
		_ = os.RemoveAll(final)
		return usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s installed but doesn't run on this computer.", c.DisplayName),
			usererr.ActionRetry, usererr.ActionCopyDetails)
	}

	progress(Progress{ComponentID: id, Stage: "Ready", Fraction: 1})
	return nil
}

// download fetches an archive, resuming a partial file where the server allows
// it. Users on unreliable connections do not restart a 400 MB download from
// zero; they give up.
func download(ctx context.Context, src Source, dst string, c Component, progress func(Progress)) error {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	var resumeFrom int64
	if info, err := os.Stat(dst); err == nil {
		resumeFrom = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeUnknown,
			"That download address isn't valid.", usererr.ActionCopyDetails)
	}
	req.Header.Set("User-Agent", strings.ToLower(version.Name)+"/"+version.Version)
	if resumeFrom > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(resumeFrom, 10))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeUnknown,
			"The download couldn't start. Check your connection and try again.",
			usererr.ActionRetry)
	}
	defer func() { _ = resp.Body.Close() }()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusOK:
		// The server ignored the range request, so start over.
		resumeFrom = 0
		flags |= os.O_TRUNC
	case http.StatusPartialContent:
		flags |= os.O_APPEND
	default:
		return usererr.New(usererr.CodeUnknown,
			fmt.Sprintf("%s couldn't be downloaded right now. Try again in a few minutes.", c.DisplayName),
			usererr.ActionRetry, usererr.ActionCopyDetails)
	}

	total := resp.ContentLength + resumeFrom
	if total <= 0 {
		total = c.DownloadBytes
	}

	f, err := os.OpenFile(dst, flags, 0o644)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeNotWritable,
			"Lathe couldn't save the download.", usererr.ActionRetry)
	}
	defer func() { _ = f.Close() }()

	counter := &progressWriter{
		done:  resumeFrom,
		total: total,
		report: func(done, tot int64) {
			frac := -1.0
			if tot > 0 {
				frac = float64(done) / float64(tot)
			}
			progress(Progress{
				ComponentID: c.ID,
				Stage:       fmt.Sprintf("Downloading %s", c.DisplayName),
				Fraction:    frac, BytesDone: done, BytesTotal: tot,
			})
		},
	}

	if _, err := io.Copy(io.MultiWriter(f, counter), resp.Body); err != nil {
		if ctx.Err() != nil {
			// The partial file is deliberately kept, so a retry resumes.
			return usererr.New(usererr.CodeCancelled, "The download was stopped.", usererr.ActionRetry)
		}
		return usererr.Wrap(err, usererr.CodeUnknown,
			"The download was interrupted. Check your connection and try again.",
			usererr.ActionRetry)
	}
	return f.Sync()
}

// verifyChecksum is mandatory, not advisory: this file becomes an executable
// on the user's machine.
func verifyChecksum(path, want string) error {
	if want == "" {
		return usererr.New(usererr.CodeUnknown,
			"That component has no published checksum, so Lathe won't install it.",
			usererr.ActionCopyDetails)
	}

	f, err := os.Open(path)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeUnknown,
			"The download couldn't be read back.", usererr.ActionRetry)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return usererr.Wrap(err, usererr.CodeUnknown,
			"The download couldn't be read back.", usererr.ActionRetry)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return usererr.New(usererr.CodeUnknown,
			"The download didn't match its published fingerprint, so Lathe deleted it. This usually means the download was corrupted.",
			usererr.ActionRetry)
	}
	return nil
}

// probeComponent proves a component works by running one of its binaries.
// Checking that a file exists is not enough: a truncated download passes that
// and fails later, in the middle of someone's job.
func (m *manager) probeComponent(ctx context.Context, c Component) probe {
	m.mu.RLock()
	cached, ok := m.probes[c.ID]
	m.mu.RUnlock()
	if ok {
		return cached
	}

	result := probe{}
	if len(c.Binaries) == 0 {
		result.usable = true
	} else {
		bin, err := m.BinaryPath(c.ID, c.Binaries[0])
		if err != nil {
			result.err = err.Error()
		} else {
			result.path = bin
			runCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()

			args := c.VersionArgs
			if args == nil {
				args = []string{"-version"}
			}
			if _, runErr := lexec.New().Run(runCtx, bin, args, lexec.Options{Timeout: probeTimeout}); runErr != nil {
				// Some tools report a version and then exit non-zero; the
				// point is only that the binary loaded and ran.
				var exitErr *lexec.ExitError
				result.usable = errors.As(runErr, &exitErr)
				if !result.usable {
					result.err = runErr.Error()
				}
			} else {
				result.usable = true
			}
		}
	}

	m.mu.Lock()
	m.probes[c.ID] = result
	m.mu.Unlock()
	return result
}

// checkSpace refuses a download rather than filling someone's disk.
func checkSpace(dir string, need int64) error {
	free, err := freeSpace(dir)
	if err != nil {
		return nil //nolint:nilerr // if free space is unknowable, do not block the user
	}
	// A margin, because the archive and the unpacked tree coexist briefly.
	if free < need+(256<<20) {
		return usererr.New(usererr.CodeDiskFull,
			fmt.Sprintf("There isn't enough free space: about %s is needed and %s is free.",
				humanBytes(need), humanBytes(free)),
			usererr.ActionFreeSpace)
	}
	return nil
}

// extract unpacks a zip or tar archive, refusing any entry that would escape
// the destination.
func extract(archive, dst, stripPrefix string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	lower := strings.ToLower(archive)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(archive, dst, stripPrefix)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"),
		strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tar.xz"),
		strings.HasSuffix(lower, ".tar"):
		return extractTar(archive, dst, stripPrefix)
	default:
		// A bare binary: install it under its own name.
		return os.Rename(archive, filepath.Join(dst, filepath.Base(archive)))
	}
}

func extractZip(archive, dst, stripPrefix string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return usererr.Wrap(err, usererr.CodeCorruptInput,
			"The download couldn't be unpacked; it may be damaged.", usererr.ActionRetry)
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		target, ok := safeJoin(dst, strip(f.Name, stripPrefix))
		if !ok {
			continue // an entry trying to escape the destination
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeEntry(target, rc, f.Mode())
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTar(archive, dst, stripPrefix string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f
	switch {
	case strings.HasSuffix(archive, ".gz"), strings.HasSuffix(archive, ".tgz"):
		gz, gzErr := gzip.NewReader(f)
		if gzErr != nil {
			return usererr.Wrap(gzErr, usererr.CodeCorruptInput,
				"The download couldn't be unpacked; it may be damaged.", usererr.ActionRetry)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case strings.HasSuffix(archive, ".bz2"):
		reader = bzip2.NewReader(f)
	case strings.HasSuffix(archive, ".xz"):
		xzr, xzErr := xz.NewReader(f)
		if xzErr != nil {
			return usererr.Wrap(xzErr, usererr.CodeCorruptInput,
				"The download couldn't be unpacked; it may be damaged.", usererr.ActionRetry)
		}
		reader = xzr
	}

	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return usererr.Wrap(err, usererr.CodeCorruptInput,
				"The download couldn't be unpacked; it may be damaged.", usererr.ActionRetry)
		}

		target, ok := safeJoin(dst, strip(hdr.Name, stripPrefix))
		if !ok {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeEntry(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		default:
			// Symlinks, devices and hard links are skipped: none are needed by
			// the components Lathe installs, and each is a way out of dst.
		}
	}
}

func writeEntry(target string, src io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, normalise(mode))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// normalise keeps the executable bit and nothing surprising.
func normalise(mode os.FileMode) os.FileMode {
	if mode&0o111 != 0 {
		return 0o755
	}
	if mode == 0 {
		return 0o644
	}
	return mode.Perm()
}

func strip(name, prefix string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "./")
	if prefix == "" {
		return name
	}
	if trimmed := strings.TrimPrefix(name, strings.TrimSuffix(prefix, "/")+"/"); trimmed != name {
		return trimmed
	}
	// The prefix may be a versioned directory whose exact name is unknown;
	// "*/" means "drop whatever the first component is".
	if prefix == "*/" {
		if _, rest, found := strings.Cut(name, "/"); found {
			return rest
		}
	}
	return name
}

// safeJoin refuses any archive entry that would land outside dst.
func safeJoin(dst, name string) (string, bool) {
	if name == "" || strings.Contains(name, "..") {
		return "", false
	}
	target := filepath.Join(dst, filepath.FromSlash(path.Clean("/"+name)))
	rel, err := filepath.Rel(dst, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return target, true
}

func archiveExt(url string) string {
	base := strings.ToLower(path.Base(url))
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tgz", ".zip", ".tar"} {
		if strings.HasSuffix(base, ext) {
			return ext
		}
	}
	return filepath.Ext(base)
}

func platformKey() string { return runtime.GOOS + "/" + runtime.GOARCH }

// progressWriter counts bytes and reports at most a few times a second, since
// a UI update per 32 KB chunk is pure noise.
type progressWriter struct {
	done, total int64
	last        time.Time
	report      func(done, total int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.done += int64(len(p))
	if time.Since(w.last) > 200*time.Millisecond {
		w.last = time.Now()
		w.report(w.done, w.total)
	}
	return len(p), nil
}

func humanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f %cB", float64(n)/float64(div), "kMGT"[exp])
}
