package threewayrsync

import "time"

// FileState fingerprints one path: size and modification time. There is no content hash
// — change detection uses rsync's own size+mtime quick-check.
type FileState struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
}

// Equal reports whether two states match on size and mtime (rsync's quick check).
func (fs FileState) Equal(other FileState) bool {
	return fs.Size == other.Size && fs.ModTime.Equal(other.ModTime)
}

// Manifest is a tree fingerprint keyed by slash-relative path.
type Manifest map[string]FileState
