package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoSession is returned by Load when no session file exists on disk.
var ErrNoSession = errors.New("no active session")

// SessionStore persists a Session to disk.
type SessionStore interface {
	Save(s *Session) error
	Load() (*Session, error) // returns ErrNoSession if none exists
	Delete() error
}

// diskStore is the concrete SessionStore that writes to the XDG data directory.
type diskStore struct {
	path string // full path to session.json
}

// NewSessionStore returns a SessionStore backed by the XDG data directory.
// Path: $XDG_DATA_HOME/handoff/session.json or ~/.local/share/handoff/session.json
func NewSessionStore() (SessionStore, error) {
	dir, err := dataDir()
	if err != nil {
		return nil, fmt.Errorf("resolving data directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}
	return &diskStore{path: filepath.Join(dir, "session.json")}, nil
}

// dataDir returns the handoff-specific XDG data directory.
func dataDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "handoff"), nil
}

// Save marshals s to JSON and writes it atomically via a temp file + os.Rename.
func (d *diskStore) Save(s *Session) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to persist session state: %w", err)
	}

	// Write to a temp file in the same directory so os.Rename is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(d.path), "session-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to persist session state: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any error path.
	defer func() {
		if err != nil {
			os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to persist session state: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("failed to persist session state: %w", err)
	}

	if err = os.Rename(tmpName, d.path); err != nil {
		return fmt.Errorf("failed to persist session state: %w", err)
	}
	return nil
}

// Load reads and unmarshals the session file.
// Returns ErrNoSession if the file does not exist.
func (d *diskStore) Load() (*Session, error) {
	data, err := os.ReadFile(d.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoSession
		}
		return nil, fmt.Errorf("failed to read session state: %w", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse session state: %w", err)
	}
	return &s, nil
}

// Delete removes the session file from disk.
func (d *diskStore) Delete() error {
	if err := os.Remove(d.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to delete session state: %w", err)
	}
	return nil
}
