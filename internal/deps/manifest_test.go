package deps

import (
	"strings"
	"testing"
)

// The manifest decides what gets downloaded onto somebody's machine and, for
// two components, what gets executed there. These are the rules that make that
// safe, asserted rather than remembered.

func TestEverySourceIsPinnedToAChecksum(t *testing.T) {
	for _, c := range Manifest() {
		for platform, src := range c.Sources {
			if strings.TrimSpace(src.SHA256) == "" {
				t.Errorf("%s/%s has no checksum, so its download could not be verified",
					c.ID, platform)
			}
			if !strings.HasPrefix(src.URL, "https://") {
				t.Errorf("%s/%s is fetched over %q rather than https",
					c.ID, platform, src.URL)
			}
		}
	}
}

// NSIS reads /D as the literal rest of the command line, so anything after it
// is swallowed into the path. A switch appended below it would not be a
// visible bug: the installer would simply put the component somewhere strange.
func TestTheDestinationIsTheFinalInstallerArgument(t *testing.T) {
	for _, c := range Manifest() {
		for platform, src := range c.Sources {
			if len(src.InstallerArgs) == 0 {
				continue
			}

			last := src.InstallerArgs[len(src.InstallerArgs)-1]
			if !strings.Contains(last, dirPlaceholder) {
				t.Errorf("%s/%s ends its installer arguments with %q; the destination must come last",
					c.ID, platform, last)
			}
			for _, arg := range src.InstallerArgs[:len(src.InstallerArgs)-1] {
				if strings.Contains(arg, dirPlaceholder) {
					t.Errorf("%s/%s names the destination in %q as well as last", c.ID, platform, arg)
				}
			}
		}
	}
}

// A component that downloads has to say what it costs before it is started,
// because the interface offers the figure next to the button.
func TestDownloadableComponentsDeclareTheirSize(t *testing.T) {
	for _, c := range Manifest() {
		if len(c.Sources) == 0 {
			continue
		}
		if c.DownloadBytes <= 0 {
			t.Errorf("%s can be downloaded but does not say how large it is", c.ID)
		}
	}
}

// Somewhere a component cannot be downloaded, advice is all there is, so it
// had better exist on every platform that will see it.
func TestComponentsThatCannotBeDownloadedEverywhereCarryAdvice(t *testing.T) {
	for _, c := range Manifest() {
		if _, everywhere := c.Sources["any"]; everywhere {
			continue
		}
		for _, goos := range []string{"windows", "darwin", "linux"} {
			// A platform with its own source needs no advice.
			if hasSourceForOS(c, goos) {
				continue
			}
			if strings.TrimSpace(c.InstallHint[goos]) == "" {
				t.Errorf("%s can be neither downloaded nor explained on %s", c.ID, goos)
			}
		}
	}
}

func hasSourceForOS(c Component, goos string) bool {
	for platform := range c.Sources {
		if strings.HasPrefix(platform, goos+"/") {
			return true
		}
	}
	return false
}

func TestInstallerArgsSubstituteTheDestination(t *testing.T) {
	src := Source{InstallerArgs: []string{"/S", "/D=" + dirPlaceholder}}

	got := installerArgsFor(src, `C:\Users\Anna Sharma\AppData\Roaming\Lathe\gs`)
	want := []string{"/S", `/D=C:\Users\Anna Sharma\AppData\Roaming\Lathe\gs`}

	if len(got) != len(want) {
		t.Fatalf("got %d arguments, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argument %d is %q, want %q", i, got[i], want[i])
		}
	}
}
