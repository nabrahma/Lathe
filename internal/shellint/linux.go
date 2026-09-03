//go:build linux

package shellint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The freedesktop convention: a .desktop entry in the user's own applications
// directory, registered as a handler for the types Lathe can open.
const desktopFile = "lathe.desktop"

type linuxIntegrator struct{}

func (l *linuxIntegrator) Status() Status {
	path, err := desktopPath()
	if err != nil {
		return Status{Supported: false, Detail: err.Error()}
	}
	if _, err := os.Stat(path); err != nil {
		return Status{Supported: true, Installed: false}
	}
	return Status{Supported: true, Installed: true, Detail: path}
}

func (l *linuxIntegrator) Install(executable string) error {
	path, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the applications folder: %w", err)
	}

	entry := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Lathe",
		"Comment=Convert, compress and read files — offline",
		"Exec=" + executable + " %F",
		"Icon=lathe",
		"Terminal=false",
		"Categories=Utility;Graphics;Office;",
		"MimeType=" + strings.Join(mimeTypes, ";") + ";",
		"", // trailing newline
	}, "\n")

	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		return fmt.Errorf("write the menu entry: %w", err)
	}
	refreshDesktopDatabase(filepath.Dir(path))
	return nil
}

func (l *linuxIntegrator) Remove() error {
	path, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove the menu entry: %w", err)
	}
	refreshDesktopDatabase(filepath.Dir(path))
	return nil
}

// mimeTypes are the types worth claiming as "open with Lathe". Claiming
// everything would be rude; these are the ones Lathe genuinely handles.
var mimeTypes = []string{
	"application/pdf",
	"image/jpeg", "image/png", "image/webp", "image/tiff", "image/bmp", "image/gif",
	"image/heic", "image/avif",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.oasis.opendocument.text",
	"video/mp4", "video/x-matroska", "video/quicktime", "video/webm",
	"audio/mpeg", "audio/x-wav", "audio/flac",
}

func desktopPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate the home folder: %w", err)
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "applications", desktopFile), nil
}

// refreshDesktopDatabase tells the desktop environment to notice the change.
// It is best-effort: the entry still works after a logout without it.
func refreshDesktopDatabase(dir string) {
	path, err := exec.LookPath("update-desktop-database")
	if err != nil {
		return
	}
	_ = exec.Command(path, dir).Run() //nolint:gosec // fixed binary, fixed argument
}

// windowsIntegrator is unreachable on Linux but keeps New's switch total.
type windowsIntegrator struct{ unsupported }
