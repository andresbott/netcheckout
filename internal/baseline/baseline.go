// Package baseline stores the per-profile checkout baseline: a snapshot of each
// checked-out tree as it was at checkout, used by sync's three-way merge
// (GOALS.md §6). It lives in a local state file, never on the remote.
package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/andresbott/netcheckout/internal/marker"
)

// FileState is one path's recorded state: size, mtime, and content hash.
type FileState struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
	Hash    string    `json:"hash"`
}

// Baseline is a profile's checkout snapshot.
type Baseline struct {
	Profile    string               `json:"profile"`
	Relpaths   []string             `json:"relpaths"`
	Files      map[string]FileState `json:"files"`
	LastSyncAt time.Time            `json:"last_sync_at"`
}

// Dir returns the state directory: $NETCHECKOUT_STATE, else
// $XDG_STATE_HOME/netcheckout, else ~/.local/state/netcheckout.
func Dir() (string, error) {
	if p := os.Getenv("NETCHECKOUT_STATE"); p != "" {
		return p, nil
	}
	if p := os.Getenv("XDG_STATE_HOME"); p != "" {
		return filepath.Join(p, "netcheckout"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "netcheckout"), nil
}

func statePath(profile string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, profile+".json"), nil
}

// Load reads the baseline for profile. A missing file is (nil, false, nil).
func Load(profile string) (*Baseline, bool, error) {
	path, err := statePath(profile)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, false, err
	}
	return &b, true, nil
}

// Save writes b to the state dir atomically (temp file + rename).
func Save(b *Baseline) error {
	path, err := statePath(b.Profile)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".netcheckout-state-*.tmp")
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
	return os.Rename(tmpName, path)
}

// Remove deletes the baseline for profile. A missing file is not an error.
func Remove(profile string) error {
	path, err := statePath(profile)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// HashFile returns the sha256 hex of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Snapshot walks each relpath subtree under root and returns a size+mtime+hash
// manifest of every regular file, keyed by slash path relative to root. The
// marker file at any level is excluded. A relpath of "." (or "") means the whole
// root. A relpath whose subtree does not exist yet is skipped, not an error.
func Snapshot(root string, relpaths []string) (map[string]FileState, error) {
	if len(relpaths) == 0 {
		relpaths = []string{"."}
	}
	out := map[string]FileState{}
	for _, rp := range relpaths {
		base := filepath.Join(root, filepath.Clean(rp))
		if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Name() == marker.FileName {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil // skip symlinks and other non-regular entries; the manifest tracks regular files only
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			hash, err := HashFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = FileState{Size: info.Size(), ModTime: info.ModTime(), Hash: hash}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
