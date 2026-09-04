// Package deps manages the components Lathe does not ship in its installer.
//
// The tier model is the reason the installer is small: PDF and image work is
// compiled in and always available, while video and Office support announce
// themselves honestly and download once. This package is one of only two
// allowed to touch the network, and the only one that writes executables to a
// user's machine, so every download is checksum-verified against a manifest
// compiled into the binary, and a mismatch is deleted rather than used.
package deps

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/nabrahma/lathe/internal/task"
)

// Tier is re-exported from task so callers need only one import.
type Tier = task.Tier

// The component tiers.
const (
	TierCore    = task.TierCore
	TierBundled = task.TierBundled
	TierMedia   = task.TierMedia
	TierOffice  = task.TierOffice
	TierEnhance = task.TierEnhance
)

// Component is an external dependency Lathe manages.
type Component struct {
	ID   string `json:"id"`
	Tier Tier   `json:"tier"`
	// DisplayName is what the user reads: "Video support", not "ffmpeg".
	DisplayName string `json:"displayName"`
	// Explanation is one sentence for the download prompt.
	Explanation string `json:"explanation"`
	// DownloadBytes is the archive size; InstalledBytes is the space it needs
	// once unpacked. Both are shown before the user commits.
	DownloadBytes  int64  `json:"downloadBytes"`
	InstalledBytes int64  `json:"installedBytes"`
	Version        string `json:"version"`

	// Binaries are the executables this component provides, without a platform
	// extension. Availability is proved by running one, not by a file check.
	Binaries []string `json:"binaries"`
	// VersionArgs invoke the binary harmlessly to prove it works.
	VersionArgs []string `json:"-"`
	// WindowsExt overrides the ".exe" assumed on Windows. LibreOffice needs
	// it: soffice.exe is a GUI launcher that returns immediately without
	// waiting or printing, while soffice.com is the console front-end that
	// actually reports a version and blocks until the conversion finishes.
	WindowsExt string `json:"-"`
	// WindowsNames renames binaries on Windows, for publishers who ship a
	// different executable name there. Ghostscript needs it: the console
	// build is gswin64c.exe, not gs.exe.
	WindowsNames map[string]string `json:"-"`

	// Sources are the per-platform download locations, keyed by GOOS/GOARCH,
	// or the single key "any" for a platform-independent file.
	Sources map[string]Source `json:"-"`

	// SearchPaths are the usual install locations, searched after PATH.
	SearchPaths []string `json:"-"`
	// InstallHint is per-GOOS advice shown when a component cannot be
	// downloaded on this platform. It names the command to run, not a concept
	// to look up.
	InstallHint map[string]string `json:"-"`
}

// SourceFor returns where this component comes from on the machine it is
// running on, and whether it can be fetched here at all.
//
// Whether Lathe can install something is a property of the platform, not of
// the component: the same Ghostscript that has to be found on a Linux machine,
// because Artifex publishes only source there, is a signed installer on
// Windows that Lathe can fetch and run.
func (c Component) SourceFor() (Source, bool) {
	if src, ok := c.Sources[platformKey()]; ok {
		return src, true
	}
	src, ok := c.Sources["any"]
	return src, ok
}

// winExt is the extension to append on Windows, and nothing elsewhere.
func (c Component) winExt() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if c.WindowsExt != "" {
		return c.WindowsExt
	}
	return ".exe"
}

// binName is the executable name to look for on this platform.
func (c Component) binName(binary string) string {
	if runtime.GOOS == "windows" {
		if alt, ok := c.WindowsNames[binary]; ok {
			binary = alt
		}
	}
	return binary + c.winExt()
}

// Hint returns the install advice for the running platform.
func (c Component) Hint() string {
	if c.InstallHint == nil {
		return ""
	}
	return c.InstallHint[runtime.GOOS]
}

// Source is where one platform's build of a component comes from.
type Source struct {
	URL string
	// SHA256 is mandatory. A download that does not match is deleted, not
	// used: this is the supply-chain defence, since the file becomes an
	// executable on someone else's machine.
	SHA256 string
	// StripPrefix drops a leading directory from the archive, since upstream
	// archives usually wrap everything in a versioned folder. "*/" means drop
	// whatever the first component happens to be called.
	StripPrefix string
	// Version is the build actually published at URL. It can differ per
	// platform, because the three FFmpeg distributors do not release together.
	Version string

	// InstallerArgs runs the downloaded file as the publisher's own installer
	// rather than unpacking it as an archive, for the projects that ship one
	// and nothing else. The literal "{{dir}}" is replaced with the directory
	// the component must end up in.
	//
	// Both installers used this way are NSIS, whose /D switch has two rules
	// worth knowing before editing anything here: it must be the final
	// argument, and it must not be quoted even when the path contains a space.
	// runInstaller is what keeps the second rule.
	InstallerArgs []string

	// Elevates records that running the installer raises a Windows permission
	// prompt, because its manifest asks for administrator rights. It exists so
	// the interface can warn someone before the prompt appears rather than
	// after, which is the difference between an expected step and an alarming
	// one.
	Elevates bool
	// Rolling marks a URL whose contents change when upstream publishes a new
	// release, which invalidates SHA256. Refresh one with:
	//   curl -sL <url> | sha256sum
	Rolling bool
}

// Status is what the settings screen shows for one component.
type Status struct {
	Component Component `json:"component"`
	Installed bool      `json:"installed"`
	// Usable is true only when the binary actually ran. A truncated download
	// passes a file-exists check and then fails mysteriously later.
	Usable    bool   `json:"usable"`
	Path      string `json:"path,omitempty"`
	DiskBytes int64  `json:"diskBytes,omitempty"`
	Problem   string `json:"problem,omitempty"`

	// Downloadable is whether Lathe can fetch this component on this machine,
	// which decides between offering a button and offering advice.
	Downloadable bool `json:"downloadable"`
	// Elevates is whether installing it raises a Windows permission prompt, so
	// the interface can say so before the prompt appears.
	Elevates bool `json:"elevates"`
}

// Progress reports a download or install in flight.
type Progress struct {
	ComponentID string  `json:"componentId"`
	Stage       string  `json:"stage"`
	Fraction    float64 `json:"fraction"`
	BytesDone   int64   `json:"bytesDone"`
	BytesTotal  int64   `json:"bytesTotal"`
}

// Manager installs, verifies and locates components.
type Manager interface {
	// Components lists everything Lathe knows how to install.
	Components() []Component
	// Status reports what is installed and what is missing.
	Status(ctx context.Context) []Status
	// Available reports whether a component is installed and actually runs.
	Available(id string) bool
	// TierAvailable reports whether everything a tier needs is usable.
	TierAvailable(t Tier) bool
	// TierName is what a tier is called in a prompt: "Video and photo
	// support", never "tier 2".
	TierName(t Tier) string
	// TierDownloadMB is the download a tier still needs, for the prompt that
	// asks the user to commit to it.
	TierDownloadMB(t Tier) int
	// BinaryPath returns the absolute path to one of a component's binaries.
	BinaryPath(componentID, binary string) (string, error)
	// Ensure downloads and installs a component. It is idempotent and safe to
	// call concurrently for the same component.
	Ensure(ctx context.Context, id string, progress func(Progress)) error
	// Remove uninstalls a component to reclaim disk space.
	Remove(id string) error
	// Verify re-checks an installed component.
	Verify(id string) error
	// DiskUsage reports space used per component.
	DiskUsage() map[string]int64
}

// ErrUnknownComponent reports an id that is not in the manifest.
var ErrUnknownComponent = errors.New("unknown component")

// ErrNotInstalled reports a component that has not been downloaded.
var ErrNotInstalled = errors.New("component is not installed")

// ErrNoSource reports that a component has no build for this platform.
var ErrNoSource = errors.New("no build of this component for this platform")

// manager is the default Manager. It caches probe results because engines ask
// about availability on every job.
type manager struct {
	root       string
	components map[string]Component
	order      []string

	mu     sync.RWMutex
	probes map[string]probe
	locks  sync.Map // component id -> *sync.Mutex
}

type probe struct {
	usable bool
	path   string
	err    string
}

// New returns a Manager storing components under root. An empty root uses the
// per-user application data directory.
func New(root string) (Manager, error) {
	if root == "" {
		var err error
		if root, err = DefaultRoot(); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create component folder: %w", err)
	}

	m := &manager{
		root:       root,
		components: make(map[string]Component),
		probes:     make(map[string]probe),
	}
	for _, c := range Manifest() {
		m.components[c.ID] = c
		m.order = append(m.order, c.ID)
	}
	return m, nil
}

// DefaultRoot is where components live: inside the app's own data directory,
// never scattered across the system.
func DefaultRoot() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("locate application data folder: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "Lathe", "components"), nil
}

func (m *manager) Components() []Component {
	out := make([]Component, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.components[id])
	}
	return out
}

func (m *manager) Status(ctx context.Context) []Status {
	out := make([]Status, 0, len(m.order))
	for _, id := range m.order {
		c := m.components[id]
		s := Status{Component: c}

		dir := m.dirOf(id)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			s.Installed = true
			s.DiskBytes = dirSize(dir)
		}

		p := m.probeComponent(ctx, c)
		s.Usable, s.Path = p.usable, p.path
		if p.usable {
			// A system component found on PATH is installed, even though
			// Lathe did not put it there.
			s.Installed = true
		}

		// Problem is read by a person, so it carries advice or nothing at all.
		// The probe's own error is engineer-facing and never shown: "component
		// is not installed" beside a Download button tells the user nothing
		// they cannot already see.
		src, downloadable := c.SourceFor()
		s.Downloadable = downloadable
		s.Elevates = downloadable && src.Elevates

		switch {
		case p.usable:
			s.Problem = ""
		case !downloadable:
			// Nothing to offer but advice, so the advice had better be exact.
			s.Problem = c.Hint()
		case s.Installed:
			s.Problem = "This component is installed but does not run. Removing and downloading it again usually fixes it."
		default:
			s.Problem = ""
		}
		out = append(out, s)
	}
	return out
}

func (m *manager) Available(id string) bool {
	c, ok := m.components[id]
	if !ok {
		return false
	}
	return m.probeComponent(context.Background(), c).usable
}

func (m *manager) TierAvailable(t Tier) bool {
	// Core work is compiled into the binary and always available.
	if t == TierCore {
		return true
	}
	for _, id := range m.order {
		c := m.components[id]
		if c.Tier == t && !m.Available(c.ID) {
			return false
		}
	}
	return true
}

// TierName describes a tier using the display names of the components it is
// still missing, so the prompt says what the user is actually getting.
func (m *manager) TierName(t Tier) string {
	var names []string
	for _, id := range m.order {
		c := m.components[id]
		if c.Tier == t && !m.Available(c.ID) {
			names = append(names, c.DisplayName)
		}
	}
	switch len(names) {
	case 0:
		return t.String()
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// TierDownloadMB is the size of what is still missing from a tier, rounded up
// to whole megabytes.
func (m *manager) TierDownloadMB(t Tier) int {
	var total int64
	for _, id := range m.order {
		c := m.components[id]
		if c.Tier == t && !m.Available(c.ID) {
			total += c.DownloadBytes
		}
	}
	return int((total + (1 << 20) - 1) >> 20)
}

func (m *manager) BinaryPath(componentID, binary string) (string, error) {
	c, ok := m.components[componentID]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownComponent, componentID)
	}
	// A managed copy always wins, so a user who installed one through Lathe
	// is not surprised by a different version found on PATH.
	dir := m.dirOf(componentID)
	if _, err := os.Stat(dir); err == nil {
		if found, err := findBinary(dir, c.binName(binary)); err == nil {
			return found, nil
		}
	}

	// Then an existing system installation. Someone who already has FFmpeg
	// should not be asked to download a second copy of it.
	if found, ok := findOnSystem(c.binName(binary), c.SearchPaths); ok {
		return found, nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotInstalled, c.DisplayName)
}

// findOnSystem looks for an already-installed binary: PATH first, then the
// locations each platform's installers actually use.
func findOnSystem(name string, searchPaths []string) (string, bool) {
	if p, err := osexec.LookPath(name); err == nil {
		return p, true
	}

	for _, dir := range searchPaths {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		// Some installers put the binary in a versioned subdirectory rather
		// than the root: Ghostscript on Windows lands in gs\gs10.07.1in.
		if p, ok := findNearby(dir, name, 3); ok {
			return p, true
		}
	}
	return "", false
}

// findNearby looks for name under dir, no deeper than maxDepth. The search
// paths are specific product directories rather than Program Files itself, so
// this stays small.
func findNearby(dir, name string, maxDepth int) (string, bool) {
	var found string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable subtrees
		}
		if found != "" {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(dir, p)
		if relErr == nil && strings.Count(rel, string(os.PathSeparator)) > maxDepth {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), name) {
			found = p
		}
		return nil
	})
	return found, found != ""
}

func (m *manager) DiskUsage() map[string]int64 {
	out := make(map[string]int64, len(m.order))
	for _, id := range m.order {
		if size := dirSize(m.dirOf(id)); size > 0 {
			out[id] = size
		}
	}
	return out
}

func (m *manager) Remove(id string) error {
	if _, ok := m.components[id]; !ok {
		return fmt.Errorf("%w: %s", ErrUnknownComponent, id)
	}
	m.forget(id)
	return os.RemoveAll(m.dirOf(id))
}

func (m *manager) Verify(id string) error {
	c, ok := m.components[id]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownComponent, id)
	}
	m.forget(id)

	p := m.probeComponent(context.Background(), c)
	if !p.usable {
		return fmt.Errorf("%s is installed but does not run: %s", c.DisplayName, p.err)
	}
	return nil
}

func (m *manager) dirOf(id string) string { return filepath.Join(m.root, id) }

func (m *manager) forget(id string) {
	m.mu.Lock()
	delete(m.probes, id)
	m.mu.Unlock()
}

// lockFor returns the per-component mutex, so two windows asking for the same
// component do not race each other into the same directory.
func (m *manager) lockFor(id string) *sync.Mutex {
	actual, _ := m.locks.LoadOrStore(id, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// dirSize sums a directory tree, used for the settings screen.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry just does not count
		}
		if info, statErr := d.Info(); statErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// findBinary locates an executable inside an installed component. Upstream
// archives disagree about layout, so the whole tree is searched once.
func findBinary(dir, want string) (string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable subtrees
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), want) {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("%s was not found in the installed files", want)
	}

	// Prefer the shallowest match: an archive can carry the same name in a
	// nested tools directory.
	sort.Slice(found, func(i, j int) bool {
		return strings.Count(found[i], string(os.PathSeparator)) < strings.Count(found[j], string(os.PathSeparator))
	})
	return found[0], nil
}
