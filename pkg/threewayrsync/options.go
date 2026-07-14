// Package threewayrsync performs three-way sync between a local and a remote endpoint
// using rsync as the sole fingerprinting and transfer engine. Base ("last-synced") state
// is persisted through a pluggable Store; change detection is rsync's size+mtime
// quick-check (no content hashing). It is decoupled from any specific application.
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
	Checksum bool        // pass rsync --checksum on transfers (content-accurate transfers)
	Exclude  []string    // rsync --exclude patterns
	Conflict ConflictPolicy
	OnEvent  func(Event) // per-path progress; runs on the sync goroutine — must not block
}

// Event is one applied change, emitted live as Sync carries it out.
type Event struct {
	Op   string // "pull" | "push" | "delete-local" | "delete-remote"
	Path string
}
