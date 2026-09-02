// Package settings stores user preferences on disk.
//
// There is no server and no account, so this is the only state Lathe keeps
// between launches, and it holds nothing about the files anyone converted.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/nabrahma/lathe/internal/fsatomic"
	"github.com/nabrahma/lathe/internal/job"
)

// Settings is what the user can change.
type Settings struct {
	// Concurrency caps simultaneous jobs. Media conversion saturates every
	// core it is given, so the default is deliberately conservative.
	Concurrency int `json:"concurrency"`
	// OutputDir is where results go. Empty means beside the input, which is
	// what people expect and what needs no explaining.
	OutputDir string `json:"outputDir"`
	// CheckUpdates is opt-in and asked once. When false, Lathe makes no
	// network request at all outside an explicit component download.
	CheckUpdates bool `json:"checkUpdates"`
	// AskedAboutUpdates records that the question has been put, so it is not
	// asked twice.
	AskedAboutUpdates bool `json:"askedAboutUpdates"`
	// ShellIntegration is the opt-in "Convert with Lathe" context-menu entry.
	ShellIntegration bool `json:"shellIntegration"`
	// EnhanceBeforeOCR defaults the preprocessing toggle on OCR tasks.
	EnhanceBeforeOCR bool `json:"enhanceBeforeOcr"`
	// Language is the interface language code.
	Language string `json:"language"`
	// Window remembers size and position across launches.
	Window WindowState `json:"window"`
}

// WindowState is the remembered geometry.
type WindowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximised bool `json:"maximised"`
}

// Defaults returns the settings a first run starts with.
func Defaults() Settings {
	return Settings{
		Concurrency:      job.DefaultConcurrency(),
		CheckUpdates:     false,
		EnhanceBeforeOCR: true,
		Language:         "en",
		Window:           WindowState{Width: 1040, Height: 760, X: -1, Y: -1},
	}
}

// Store reads and writes the settings file.
type Store struct {
	path string
	mu   sync.RWMutex
	data Settings
}

// Load reads settings from disk, falling back to defaults. A corrupt or
// unreadable file is not an error the user should have to deal with: they get
// working defaults and the file is rewritten on the next save.
func Load() (*Store, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	s := &Store{path: path, data: Defaults()}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, nil //nolint:nilerr // an unreadable file must not block startup
	}
	defer func() { _ = f.Close() }()

	body, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return s, nil //nolint:nilerr // as above
	}

	loaded := Defaults()
	if err := json.Unmarshal(body, &loaded); err != nil {
		return s, nil //nolint:nilerr // as above
	}
	s.data = sanitise(loaded)
	return s, nil
}

// Path is where the settings file lives.
func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("locate settings folder: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "Lathe", "settings.json"), nil
}

// Get returns the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Save validates and persists settings atomically.
func (s *Store) Save(next Settings) error {
	next = sanitise(next)

	s.mu.Lock()
	s.data = next
	s.mu.Unlock()

	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(s.path, func(w io.Writer) error {
		_, writeErr := w.Write(append(body, '\n'))
		return writeErr
	}, 0o600)
}

// sanitise keeps a hand-edited or outdated file from producing nonsense.
func sanitise(s Settings) Settings {
	if s.Concurrency < 1 || s.Concurrency > 16 {
		s.Concurrency = job.DefaultConcurrency()
	}
	if s.Language == "" {
		s.Language = "en"
	}
	if s.Window.Width < 640 {
		s.Window.Width = 1040
	}
	if s.Window.Height < 480 {
		s.Window.Height = 760
	}
	return s
}
