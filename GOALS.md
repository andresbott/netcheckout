# netcheckout — Goals & Features

> A CLI utility to **check out** and **check in** work directories over network drives
> (e.g. a locally-mounted Samba/NAS share), using `rsync` to copy files between a
> remote root and a local working copy, and leaving a marker behind so others can see
> a folder is checked out and by whom.

Status: **initial draft / design spec.** Sections marked _(proposed — confirm)_ were
chosen as sensible defaults and should be confirmed before implementation begins.

---

## 1. Motivation

Working directly on files over a network mount (SMB/NFS) is slow and fragile. The
common workaround — manually `rsync`-ing a folder to local disk, remembering to copy it
back, and hoping nobody else edits it meanwhile — is error-prone.

`netcheckout` makes that workflow explicit and safe:

- **Checkout**: pull a remote folder down to a fast local working copy.
- **Check in**: push your changes back to the remote when you're done.
- **Marker**: while checked out, a small file is left on the remote announcing *who*
  has it, so a second person (or a second machine) doesn't clobber your work.

Think of it like `git checkout` for plain folders on a shared drive: you work on your
own local copy, and a marker tells everyone else you're holding it.

---

## 2. Core concepts

| Concept | Meaning |
|---|---|
| **Identity** | A short string in the config identifying *you* (e.g. `andres@thinkpad`). Stamped into every marker so others know who holds a checkout. |
| **Profile** | A named pair of roots: one **local root** and one **remote root**. |
| **Root** | A base directory. The **remote root** lives on the mounted network share; the **local root** is on fast local disk. |
| **Subfolder / relpath** | A path *relative to both roots* selecting what to check out (e.g. `./2025/jan`). `./` (or omitted) means the whole root. A profile MAY pre-declare a set of these as `subpaths` (see §4) to scope it. |
| **Checkout** | **Copy** `remote_root/<relpath>` → `local_root/<relpath>` (remote keeps its files) and place a marker on the remote. |
| **Check in** | Sync `local_root/<relpath>` → `remote_root/<relpath>` and remove the marker. |
| **Marker** | A small file left inside the remote subfolder recording who checked it out, from which host, and when. Acts as the lock. |

A single profile can have **many independent checkouts** at once — one per relpath
(e.g. `./jan` checked out while `./march` is not).

---

## 3. Key decisions _(proposed — confirm)_

These four choices shape the whole design. Defaults below are proposed; see
[Open questions](#12-open-questions) to revisit.

1. **Language: Go.** Single static binary, no runtime deps on target machines, natural
   fit for a CLI that shells out to `rsync` and parses JSON.
2. **Checkout mode: Copy + marker.** Checkout **copies** files from the remote down to
   local; the remote **keeps its files** as the canonical copy. A marker is written on
   the remote to announce the checkout and act as a cooperative lock. Check-in copies
   your working copy back and removes the marker. Nothing is ever moved or deleted off
   the remote, so there is no local-only / data-loss window.
3. **Selection model: 2 roots + relative path, with optional declared scope.** A profile
   is one `local_root` + one `remote_root`. You pick what to check out by passing a
   relative path at runtime; a profile MAY additionally declare a `subpaths` list (see §4)
   that scopes it to just those relpaths. When no `subpaths` are declared the whole root is
   in scope, so pre-declaration stays optional.
4. **Lock conflicts: block others, allow your own.** If a marker already exists and
   belongs to *someone else's* identity, the checkout is refused. If it's *your own*
   (e.g. a stale lock from another machine), it's allowed. `--force` overrides either.

---

## 4. Configuration

**Location**: YAML file at `os.UserConfigDir()/netcheckout/config.yaml`:

| OS | Path |
|---|---|
| Linux | `~/.config/netcheckout/config.yaml` |
| macOS | `~/Library/Application Support/netcheckout/config.yaml` |
| Windows | `%AppData%\netcheckout\config.yaml` |

Overridable via `--config <path>` or `$NETCHECKOUT_CONFIG`.

**Schema:**

```yaml
# Who you are. Stamped into markers. If omitted, defaults to "$USER@$HOSTNAME".
identity: andres@thinkpad

# Named profiles. Each is one local root + one remote root.
profiles:
  photos:
    local_root:  /home/bott/pics
    remote_root: /mnt/smb/fotos/2025
  work:
    local_root:  /home/bott/work
    remote_root: /mnt/nas/work
    # Optional: scope this profile to only these relpaths under BOTH roots.
    # Relative paths, may be nested; omit entirely to mean the whole root.
    subpaths:
      - reports
      - notes/2024
```

**Rules:**

- Roots must be absolute paths.
- The remote root is expected to be an already-mounted network path; `netcheckout` does
  **not** mount shares itself. It will refuse to operate if the remote root is missing
  or (heuristically) not mounted.
- `~` and environment variables in root paths are expanded.
- `subpaths`, when present, are relative paths under *both* roots; a leading `./` is
  allowed and normalized. They must not be absolute or escape the root (`..`). An empty or
  omitted list means the whole root.

---

## 5. Marker file

Placed on the remote at `remote_root/<relpath>/.netcheckout.json` _(name proposed)_.
Human-readable JSON so anyone browsing the share understands it:

```json
{
  "checked_out_by": "andres@thinkpad",
  "profile": "photos",
  "relpath": "./2025/jan",
  "host": "thinkpad",
  "checked_out_at": "2026-07-02T10:48:00Z",
  "tool_version": "0.1.0"
}
```

- The remote keeps its files; the marker simply sits alongside them announcing the checkout.
- The marker is the source of truth for the lock. It is written atomically
  (write temp + rename) and removed on successful check-in.

---

## 6. Local state _(proposed)_

A lightweight local state file (`~/.local/state/netcheckout/state.json`) records active
checkouts (profile, relpath, timestamp) so `status` can report what *this machine* holds
without scanning every remote. Remote markers remain the authoritative lock; local state
is a convenience/cache and is reconciled against markers on `status`.

---

## 7. Commands / features

| Command | Description |
|---|---|
| `netcheckout` | Launch the interactive TUI to manage profiles (add/edit/delete). Prints the plain-text profile list instead when stdout isn't a terminal (e.g. piped). |
| `netcheckout list` | Print configured profiles and their roots as plain text. |
| `netcheckout status <profile>` | Run `rsync` dry-run diffs in both directions and report whether the profile's local and remote roots differ, listing the changes needed to bring them in sync. |
| `netcheckout checkout <profile> [relpath]` | Copy `remote→local` (remote unchanged), write marker. |
| `netcheckout checkin  <profile> [relpath]` | Push `local→remote`, remove marker. |
| `netcheckout init` | Write a starter config file if none exists. |

**Global flags:**

- `--dry-run` — show the exact `rsync` plan and marker actions without executing.
- `--force` — override an existing lock (see conflict rules).
- `--config <path>` — use an alternate config file.
- `-v/--verbose` — surface `rsync` progress and detail.

---

## 8. Checkout flow

For `checkout <profile> <relpath>`:

1. Load config; resolve `identity`, `local_root`, `remote_root`.
2. Verify the remote root is present/mounted; refuse otherwise.
3. Check for an existing marker at `remote_root/<relpath>/.netcheckout.json`:
   - Owned by someone else → **refuse** (unless `--force`).
   - Owned by you → proceed (recovering your own lock).
4. `rsync -a` `remote_root/<relpath>/` → `local_root/<relpath>/` (remote unchanged).
5. Verify transfer succeeded (rsync exit status; optional checksum pass).
6. Write the marker atomically on the remote.
7. Record the checkout in local state.

If any step fails, roll back and leave the remote untouched (no marker written).

---

## 9. Check-in flow

For `checkin <profile> <relpath>`:

1. Load config; resolve roots and identity.
2. Verify the remote root is present/mounted.
3. Confirm the marker exists and is yours (warn/refuse on mismatch unless `--force`).
4. `rsync -a` `local_root/<relpath>/` → `remote_root/<relpath>/` (optionally `--delete`
   so the remote mirrors your working copy — see open questions).
5. Verify success.
6. Remove the marker.
7. Update local state (clear this checkout).
8. _(Optional, proposed off by default)_ `--clean` removes the local working copy after
   a successful check-in.

---

## 10. Locking & conflict rules

- The marker is a **cooperative lock**, not an OS-level lock — it only protects against
  other `netcheckout` users, not someone editing the share by hand.
- Ownership is decided by comparing the marker's `checked_out_by` to your `identity`.
- `--force` overrides, but always prints a loud warning naming the current holder.
- **Nested paths** _(to decide)_: checking out `./2025` while `./2025/jan` is already
  checked out is a conflict; the exact rule is an [open question](#12-open-questions).

---

## 11. Safety & error handling

- **Checkout never modifies the remote except to write the marker** — the remote copy is always preserved.
- Refuse to run if a root is missing or the remote appears unmounted.
- `rsync` failures abort the operation without writing a "checked out" marker or
  deleting anything; partial transfers can be safely retried.
- `--dry-run` is available on every mutating command.
- Clear, actionable error messages (e.g. "remote root /mnt/smb/fotos/2025 is not
  mounted", "folder is checked out by alice@nas since 2026-07-01").

---

## 12. Non-goals (YAGNI)

- No central server or daemon — it's a local CLI over an existing mount.
- No merge/conflict resolution — checkout is exclusive, not multi-writer.
- No authentication or encryption — relies on the underlying share's own auth.
- No GUI.
- No sub-file / partial locking — the lock unit is a checked-out subfolder.
- Linux-first; `rsync` is assumed to be installed and on `PATH`.

---

## 13. Open questions

1. **Language** — confirm Go.
2. **Marker filename** — `.netcheckout.json` vs `.CHECKED_OUT` vs other.
3. ~~**Config location**~~ — **Resolved:** YAML at the OS config dir (`os.UserConfigDir()/netcheckout/config.yaml`; see §4).
4. **Local state** — keep a local state cache, or rely solely on remote markers?
5. **Nested checkouts** — rule when a parent/child of an already-checked-out path is requested.
6. **Check-in `--delete`** — should check-in mirror local deletions to the remote (rsync `--delete`), or only add/update?
7. **`--clean` on check-in** — should the local copy be removed after check-in, and by default?
8. **Subpath discrepancy scan** — when a profile declares `subpaths`, a folder added
   locally (or newly appearing remotely) that is not listed is silently ignored by scoped
   actions. A dedicated scan should walk both roots, compare against the declared
   `subpaths`, and flag the discrepancies (present-but-unlisted, and listed-but-missing).
   Reporting surface (CLI vs TUI) is undecided.
9. **Marker/lock reconciliation for `status`** — the original plan for `status`
   was to reconcile local state against remote markers (§5/§6): show active
   checkouts and flag conflicts/stale locks. That's deferred in favor of a
   simpler `status` that only diffs local vs remote content via `rsync`
   dry-run. Whether marker reconciliation becomes part of `status` later, or
   its own command, is still open.

---

## 14. Roadmap / milestones

- **M1 — Foundations:** config schema + loader, identity resolution, `list`, `init`.
- **M2 — Read-only status:** `status` (rsync dry-run diff of local vs remote — done),
  and a **subpath discrepancy scan** that flags on-disk folders not covered by a
  profile's declared `subpaths` (see open question 8). Marker format and local-state
  reconciliation (see open question 9) remain open.
- **M3 — Checkout:** `checkout` (copy + marker/lock), conflict rules, `--dry-run`, `--force`.
- **M4 — Check-in:** `checkin` with marker removal + verification; `--clean` option.
- **M5 — Polish:** verbose/progress output, thorough error messages, tests.
