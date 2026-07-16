// Package threewayrsync performs three-way sync between a local and a remote endpoint
// using rsync as the sole fingerprinting and transfer engine. An endpoint is a local
// path (e.g. a mounted share), an ssh target, or an rsync daemon module. Base
// ("last-synced") state is persisted through a pluggable Store; change detection is
// rsync's size+mtime quick-check (no content hashing). Options.Scope limits a sync to
// chosen subdirectories while preserving base knowledge about the rest of the tree. It
// is decoupled from any specific application.
//
// Remote deletes (ssh and daemon alike) run through rsync --delete-missing-args, which
// requires rsync >= 3.1 on both ends (note: Apple's openrsync on macOS may lack it; a
// mac syncing against a mounted SMB share is unaffected, since filesystem deletes use
// os.Remove).
//
// Known limitations:
//   - Change detection is size + 1-second mtime. Two files that differ in content but
//     match both are declared in-sync and recorded in the base without warning.
//     Options.Checksum makes transfers content-accurate but does not change detection.
//   - Regular files only: symlinks, empty directories, permissions, and ownership are
//     neither synced nor deleted.
//   - Deletes on a remote endpoint (ssh or daemon) are not re-verified against the
//     enumerated state; a remote file edited between enumeration and deletion is lost.
//     Local deletes are guarded and skip (reporting a conflict) instead. A remote file
//     replaced by a non-empty directory is not deleted (no --force) and resurfaces as a
//     discrepancy next run.
//   - A Syncer is not safe for concurrent use. Cross-process exclusion is provided by
//     the Store when it implements Locker (FileStore does, via flock).
package threewayrsync

// ConflictPolicy decides what Sync does with paths that changed on both sides.
type ConflictPolicy int

const (
	// Abort refuses the whole sync when any conflict exists, changing nothing.
	Abort ConflictPolicy = iota
	// Skip applies every clean operation and leaves conflicts untouched.
	Skip
	// PreferLocal resolves conflicts by pushing the local copy over the remote.
	PreferLocal
	// PreferRemote resolves conflicts by pulling the remote copy over the local.
	PreferRemote
)

// Options control a single sync.
type Options struct {
	Checksum bool     // pass rsync --checksum on transfers (content-accurate transfers)
	Exclude  []string // rsync --exclude patterns
	Conflict ConflictPolicy
	OnEvent  func(Event) // per-path progress; runs on the sync goroutine — must not block

	// AcceptEmpty permits syncing against a local endpoint that is an empty directory
	// even though the base records files from a previous sync. Without it, Sync and Diff
	// fail with *EmptyEndpointError: an empty-but-listable directory is exactly what an
	// unmounted mount point or a mistyped path looks like, and syncing against one would
	// delete the entire other side.
	AcceptEmpty bool

	// MaxDeleteFraction caps how much of the base a single Sync may delete. When the
	// plan's deletions (both sides combined) exceed this fraction of the files in base
	// (the in-scope part, when Scope is set), Sync fails with *TooManyDeletesError before
	// touching anything. 0 means the default of 0.5; values >= 1 disable the guard.
	MaxDeleteFraction float64

	// Scope limits the sync to the given slash-relative subdirectories (e.g. "docs/api").
	// Empty means the whole tree. Enumeration, transfers, and deletes all stay inside the
	// scope; base entries outside it are preserved untouched, so alternating scoped and
	// full syncs is safe. Entries must be plain relative directory paths — no "..", no
	// leading "/", no rsync wildcards.
	Scope []string
}

// Event is one applied change, emitted live as Sync carries it out.
type Event struct {
	Op   string // "pull" | "push" | "delete-local" | "delete-remote"
	Path string
}
