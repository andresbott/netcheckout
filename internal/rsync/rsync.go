// Package rsync wraps the rsync binary with a small, purpose-built API for
// netcheckout: a structured dry-run Diff and a real Sync (optionally deleting)
// between a local folder and a remote folder reached either through a mounted
// path or over ssh. It is deliberately decoupled from the config package.
package rsync

import (
	"fmt"
	"strings"
)

// Direction is the direction of a sync between the two endpoints of a Job.
type Direction int

const (
	// Pull copies Remote → Local (a netcheckout checkout).
	Pull Direction = iota
	// Push copies Local → Remote (a netcheckout check in).
	Push
)

// SSH describes reaching an endpoint over ssh. A zero Port and an empty
// IdentityFile mean "use the ssh default".
type SSH struct {
	User         string
	Host         string
	Port         int
	IdentityFile string
}

// Endpoint is one side of a sync: an absolute path, optionally reached over ssh.
// A nil SSH means a local filesystem path (for example a mounted share).
type Endpoint struct {
	Path string
	SSH  *SSH
}

// Options control how a sync behaves. The zero value is mount-safe: recursive
// add/update preserving modification times only — no deletion and no
// perms/owner/group, which SMB and NFS mounts frequently cannot honor.
type Options struct {
	Delete        bool // rsync --delete
	PreservePerms bool // rsync --perms
	PreserveOwner bool // rsync --owner
	PreserveGroup bool // rsync --group
	Checksum      bool // rsync --checksum
}

// Job is a single source→destination sync, expressed as a local and a remote
// endpoint plus the direction between them.
type Job struct {
	Local     Endpoint
	Remote    Endpoint
	Direction Direction
	Options   Options
}

// ChangeType classifies a single itemized change.
type ChangeType int

const (
	Created  ChangeType = iota // created at the destination
	Modified                   // contents or attributes updated
	Deleted                    // removed from the destination (only with Options.Delete)
)

// Change is one path a sync creates, updates, or deletes.
type Change struct {
	Path string
	Type ChangeType
}

// Diff is the outcome of a dry run: the changes a Sync of the same Job would make.
type Diff struct {
	Changes []Change
	InSync  bool
}

// Result is the outcome of a real Sync: the changes rsync reported making and its
// raw stdout.
type Result struct {
	Changes []Change
	Raw     string
}

// Error is returned when rsync exits non-zero, carrying enough detail for an
// actionable message.
type Error struct {
	Op       string // "diff" or "sync"
	Args     []string
	Stderr   string
	ExitCode int
	Err      error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("rsync %s: exit %d", e.Op, e.ExitCode)
	if s := strings.TrimSpace(e.Stderr); s != "" {
		return msg + ": " + s
	}
	// No exit status and no stderr (e.g. rsync failed to start, such as a missing
	// binary); surface the wrapped cause instead of a bare "exit 0".
	if e.ExitCode == 0 && e.Err != nil {
		return fmt.Sprintf("rsync %s: %v", e.Op, e.Err)
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }
