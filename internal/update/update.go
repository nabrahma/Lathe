// Package update performs the opt-in check for a newer release.
//
// This and internal/deps are the only packages permitted to touch the network,
// and this one is the smaller promise: it is off unless the user turns it on,
// it sends nothing but a version string, and it never downloads or installs
// anything by itself. What it does is documented in docs/PRIVACY.md, and the
// claim is checkable by watching the traffic.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nabrahma/lathe/internal/version"
)

// endpoint is the GitHub releases API for this repository. It is a plain GET
// with no query string, no cookies and no identifying header beyond the user
// agent, which carries the version and nothing else.
const endpoint = "https://api.github.com/repos/nabrahma/Lathe/releases/latest"

// checkTimeout keeps a slow or unreachable server from making the app feel
// stuck. A failed check is a non-event.
const checkTimeout = 8 * time.Second

// Release describes a newer version, when one exists.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes,omitempty"`
}

// ErrUpToDate reports that the running version is the latest.
var ErrUpToDate = errors.New("already up to date")

// Check asks whether a newer release exists.
//
// The caller must only call this when the user has opted in; the package does
// not check the setting itself, so the decision stays visible at the call
// site rather than buried here.
func Check(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	// The only thing sent about this machine is the app's own version.
	req.Header.Set("User-Agent", strings.ToLower(version.Name)+"/"+version.Version)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach the update server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the update server replied with status %d", resp.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		Draft   bool   `json:"draft"`
		Pre     bool   `json:"prerelease"`
	}
	// A cap, because an unbounded read from a remote server is an unbounded
	// allocation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("could not read the reply: %w", err)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("could not understand the reply: %w", err)
	}
	if payload.Draft || payload.Pre {
		return nil, ErrUpToDate
	}

	latest := strings.TrimPrefix(payload.TagName, "v")
	if !IsNewer(latest, version.Version) {
		return nil, ErrUpToDate
	}
	return &Release{Version: latest, URL: payload.HTMLURL, Notes: payload.Body}, nil
}

// IsNewer compares two semantic versions. An unparseable running version, a
// development build for instance, is never treated as out of date, so a developer is
// not nagged about their own working copy.
func IsNewer(candidate, current string) bool {
	a, okA := parse(candidate)
	b, okB := parse(current)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// parse reads "1.2.3" into its three numbers, tolerating a leading v and a
// trailing pre-release or build suffix.
func parse(v string) ([3]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return [3]int{}, false
	}

	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
