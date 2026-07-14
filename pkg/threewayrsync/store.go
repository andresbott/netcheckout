package threewayrsync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Store persists the base ("last-synced") manifest between syncs. Implementations decide
// where it lives; the library loads once at the start of a sync and saves once at the end.
// LoadBase returns (nil, false, nil) when there is no prior sync.
type Store interface {
	LoadBase() (Manifest, bool, error)
	SaveBase(Manifest) error
}

// FileStore persists the base manifest to a single JSON file, written atomically.
type FileStore struct {
	Path string
}

// LoadBase reads the manifest. A missing file is (nil, false, nil).
func (f FileStore) LoadBase() (Manifest, bool, error) {
	data, err := os.ReadFile(f.Path) //nolint:gosec // G304: path is caller-supplied application state, not attacker-controlled input.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, err
	}
	return m, true, nil
}

// SaveBase writes the manifest atomically (temp file + rename), creating the parent dir.
func (f FileStore) SaveBase(m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".threewayrsync-state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, f.Path)
}
