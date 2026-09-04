package deps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nabrahma/lathe/internal/usererr"
)

// Some projects publish an installer and nothing else. Tesseract and
// Ghostscript both do on Windows: there is no portable archive to unpack, only
// a setup program. Refusing to run it means telling people to go and find the
// download themselves, which is the wall this exists to remove.
//
// The exchange is that an installer is opaque where an archive is not. Lathe
// still pins the exact file by SHA-256, so what runs is the byte-for-byte file
// the checksum was taken from, and it still installs into Lathe's own folder
// rather than the system, so removing the component is deleting a directory.

// installerTimeout is generous. These installers unpack a few hundred
// megabytes, and on a slow disk that is minutes rather than seconds. It also
// has to cover the time somebody spends looking at the Windows permission
// prompt before deciding.
const installerTimeout = 20 * time.Minute

// dirPlaceholder is what an InstallerArgs entry uses to mean the destination.
const dirPlaceholder = "{{dir}}"

// runPublisherInstaller installs a component by running the publisher's own
// setup program.
func (m *manager) runPublisherInstaller(
	ctx context.Context, c Component, src Source, installer, dir string,
) error {
	// The installer writes straight into the final directory rather than into
	// staging to be moved afterwards. An NSIS installer records where it put
	// itself, and moving the tree behind its back leaves an uninstall entry
	// pointing at nothing. That costs the atomicity the archive path has, so
	// every failure below clears the directory out again.
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return usererr.Wrap(err, usererr.CodeNotWritable,
			fmt.Sprintf("Lathe couldn't create a folder to install %s into.", c.DisplayName),
			usererr.ActionRetry)
	}

	if err := runInstaller(ctx, installer, installerArgsFor(src, dir), installerTimeout); err != nil {
		_ = os.RemoveAll(dir)
		return installFailed(c, err)
	}
	return nil
}

// installerArgsFor fills the destination into the publisher's own switches.
func installerArgsFor(src Source, dir string) []string {
	args := make([]string, len(src.InstallerArgs))
	for i, arg := range src.InstallerArgs {
		args[i] = strings.ReplaceAll(arg, dirPlaceholder, dir)
	}
	return args
}

// installFailed turns an installer's exit into something worth reading.
//
// Declining the Windows permission prompt is the common case by a wide margin,
// and it is a decision rather than a fault, so it gets its own wording: no
// apology, no error code, and the way to change your mind.
func installFailed(c Component, err error) error {
	if isPermissionDeclined(err) {
		return usererr.New(usererr.CodeComponentMissing,
			fmt.Sprintf("%s wasn't installed, because Windows was not given permission. "+
				"Choose Download again and select Yes when Windows asks.", c.DisplayName),
			usererr.ActionRetry)
	}
	return usererr.Wrap(err, usererr.CodeComponentMissing,
		fmt.Sprintf("%s downloaded, but its installer didn't finish.", c.DisplayName),
		usererr.ActionRetry, usererr.ActionCopyDetails)
}
