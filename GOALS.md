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

- **Checkout**: claim a remote folder by placing a marker; `sync` then pulls it down to a fast local working copy.
- **Check in**: release the folder once your changes are synced back to the remote.
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
| **Checkout** | Place the profile marker on the remote and record an empty local baseline. Copies **no** files — pulling the tree down is `sync`'s job. |
| **Check in** | Verify `local_root` and `remote_root` are already in sync, then remove the marker (releases the whole profile). Refuses if anything is unsynced. |
| **Marker** | A small file at the **remote root** recording who holds the profile, from which host, when, and which relpaths are pulled. Acts as the lock. |

A profile is checked out **as a unit**: exactly **one** marker/lock per profile, at its remote
root. A `relpath` only scopes *which files are in scope* — recorded at checkout, copied by
`sync` — it never splits the lock, so you can't hold `./jan` while `./march` stays free. The
profile is held, and released, as a whole.

---

## 3. Key decisions _(proposed — confirm)_

These four choices shape the whole design. Defaults below are proposed; see
[Open questions](#12-open-questions) to revisit.

1. **Language: Go.** Single static binary, no runtime deps on target machines, natural
   fit for a CLI that shells out to `rsync` and parses JSON.
2. **Checkout mode: marker-only; `sync` moves the data.** Checkout writes a marker on the
   remote to announce the checkout and act as a cooperative lock, and records an empty local
   baseline — it copies **no** files. `sync` is the single data-moving command: it pulls the
   remote down and pushes local work back. Check-in copies nothing either — it verifies the
   profile is already in sync, then removes the marker. The remote **keeps its files** as the
   canonical copy throughout, and nothing is moved or deleted off it outside an explicit
   `sync`, so there is no local-only / data-loss window.
3. **Selection model: 2 roots + relative path, with optional declared scope.** A profile
   is one `local_root` + one `remote_root`. You pick what to check out by passing a
   relative path at runtime; a profile MAY additionally declare a `subpaths` list (see §4)
   that scopes it to just those relpaths. When no `subpaths` are declared the whole root is
   in scope, so pre-declaration stays optional.
4. **Lock conflicts: block anyone but this machine.** A marker is *yours* only when *this
   machine* wrote it — its `checked_out_by` **and** `host` both match. Any other existing
   marker — someone else's identity, **or your own identity on a different host** — refuses the
   checkout; there is no automatic cross-machine recovery, so reclaiming it means deleting the
   lock by hand. `--force` overrides with a loud warning naming the holder.

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

One marker **per profile**, at the remote root: `remote_root/.netcheckout.json`. Human-readable
JSON so anyone browsing the share understands it:

```json
{
  "checked_out_by": "andres@thinkpad",
  "profile": "photos",
  "host": "thinkpad",
  "relpaths": ["./2025/jan", "./2025/march"],
  "checked_out_at": "2026-07-02T10:48:00Z",
  "last_sync_at":   "2026-07-05T09:12:00Z",
  "tool_version": "0.1.0"
}
```

- The marker is **per-profile, not per-relpath** — one lock for the whole profile, written at its
  remote root even when the profile declares `subpaths` or a nested `relpath` is checked out.
  `relpaths` records the *set* currently pulled down (grown as relpaths are added on checkout);
  it scopes which files copy, never where the marker lives or what it locks.
- `checked_out_at` is set on first checkout; `last_sync_at` is refreshed on every `sync`.
- The remote keeps its files; the marker simply sits at the root announcing the checkout.
- The marker is the source of truth for the lock. It is written atomically (write temp + rename)
  and removed on successful check-in (which releases the whole profile).
- The authoritative record of *what content* was pulled (for `sync`'s three-way merge) is the
  local **baseline** (§6), not this marker.

---

## 6. Local state & the checkout baseline _(required — closes open question 4)_

`sync` (§9.5) is a three-way merge, which needs a **baseline**: a snapshot of each checked-out
tree *as it was at checkout*, when local and remote were identical. Comparing local vs remote
directly can't tell a one-sided change from a both-sided conflict, nor a local delete from a
remote addition; measuring both sides against the baseline makes every case decidable. So the
baseline is **not optional** — it is written at checkout and refreshed after every clean sync.

It lives in a **local state file** per profile (e.g. `~/.local/state/netcheckout/<profile>.json`):
the relpaths covered plus a `path → size+mtime` manifest of what was pulled (checksums instead of
mtime if share mtimes prove unreliable), and `last_sync_at`. Keeping it local — rather than beside
the remote marker — leaves the shared marker small and human-readable (§5) and avoids writing a
large manifest to the slow share on every sync. Because the baseline is per-machine,
`sync`/`checkin` require it locally, which dovetails with the lock rule (§3, §10): only *this
machine* can operate on its own checkout.

The state directory is resolved as `$NETCHECKOUT_STATE`, else `$XDG_STATE_HOME/netcheckout`, else
`~/.local/state/netcheckout`. Change detection against the baseline is **hybrid**: a size+mtime
fast path, confirmed by a stored content hash (SHA-256), so a network-share mtime change with
identical content is not mistaken for a real edit.

This local state also answers "what does this machine hold" without scanning remotes. Full
marker/lock reconciliation inside `status` remains a separate, still-open question (§13 Q9);
`status` today only diffs content via `rsync` dry-run.

The manifest tracks **regular files only**. A symlink is copied by `rsync` during `sync` like
anything else, but `Snapshot`/`Scan` skip it, so it is never recorded in the baseline and never
reconciled by `sync`/`checkin` — a symlink created locally is not pushed. It follows that `--clean`
(§9), which removes the entire local working copy, would silently discard any such symlink.

---

## 7. Commands / features

| Command | Description |
|---|---|
| `netcheckout` | Launch the interactive TUI to manage profiles (add/edit/delete). Prints the plain-text profile list instead when stdout isn't a terminal (e.g. piped). |
| `netcheckout list` | Print configured profiles and their roots as plain text. |
| `netcheckout status <profile>` | Run `rsync` dry-run diffs in both directions and report whether the profile's local and remote roots differ, listing the changes needed to bring them in sync. |
| `netcheckout checkout <profile> [relpath]` | Write the profile marker and record an empty baseline — no files are copied (run `sync` to pull). `relpath` scopes the recorded relpaths; the lock is the whole profile. |
| `netcheckout sync <profile> [relpath]` | Reconcile a held checkout in place (§9.5): push your changes, pull remote-only changes, stop on same-file conflicts. Lock-required and untouched. `relpath` scopes which held files reconcile. |
| `netcheckout checkin <profile>` | Finish and release the whole profile: verify local and remote are already in sync (same engine as `sync`), then remove the marker. Refuses if anything is unsynced — run `sync` first. No `relpath`; no `--force`. |
| `netcheckout init` | Write a starter config file if none exists. |

**Global flags:**

- `--dry-run` — show the exact `rsync` plan and marker actions without executing.
- `--force` — override an existing lock (see conflict rules). Not available on `checkin`, which
  requires a this-machine-owned, fully-synced profile.
- `--config <path>` — use an alternate config file.
- `-v/--verbose` — surface `rsync` progress and detail.

**TUI equivalents:** the interactive TUI exposes `force`/`clean` as pre-run toggles in the footer
(rather than flags) before launching sync/checkin actions, and renders lock/conflict failures in
the Activity box instead of an inline force prompt. Check-in still prompts for confirmation before
releasing the profile.

---

## 8. Checkout flow

For `checkout <profile> [relpath]` (`relpath` omitted = all declared `subpaths`, or the whole
root; it only scopes the recorded relpaths that `sync` later copies — the lock is always the
whole profile):

1. Load config; resolve `identity`, `local_root`, `remote_root`.
2. Verify the remote root is present/mounted; refuse otherwise.
3. Check for an existing marker at `remote_root/.netcheckout.json`:
   - Written by **this machine** (`checked_out_by` **and** `host` match) → proceed (you already
     hold it; e.g. adding a relpath).
   - Anyone else, **or your own identity on another host** → **refuse** (unless `--force`);
     reclaiming means deleting the lock by hand.
4. Refuse if the local target already holds content (an existing working copy). This guard is
   absolute — `--force` does not bypass it — and keeping the target empty makes the first
   `sync` a clean pull.
5. Record an **empty baseline** in local state (§6) for the scoped `relpaths`. No files are
   copied; `sync` pulls the remote down afterwards, and the empty baseline makes it classify
   every remote file as a fresh pull (never as a delete against a phantom snapshot).
6. Write (or update) the marker atomically on the remote.

If any step fails, roll back (remove the just-written baseline) and leave the remote untouched
(no marker written).

---

## 9. Check-in flow

For `checkin <profile>` — releases the **whole profile** (no `relpath`; partial check-in isn't
supported):

1. Load config; resolve roots and identity.
2. Verify the remote root is present/mounted.
3. Confirm the marker exists and is **yours (this machine)**. There is **no `--force`**: a
   foreign or mismatched lock always refuses (reclaim it by hand, per §3/§10).
4. **Verify the profile is already in sync.** Run the same three-way reconcile engine as `sync`
   (§9.5) as a read-only check: if it finds *any* pending work — a push, a pull, a
   baseline-scoped delete, or a same-file conflict — checkin **fails** and lists what is
   pending, leaving the marker in place. It copies nothing: moving data is `sync`'s job. Run
   `sync` until the profile is clean, then check in.
5. Remove the marker (releases the profile).
6. Clear the baseline / local state for this profile.
7. _(Optional, off by default)_ `--clean` removes the local working copy after a successful
   release.

---

## 9.5 Sync flow (reconcile a held checkout — NEW)

`sync <profile> [relpath]` is the in-between operation for a checkout you're still holding: push
your local work back, pull the remote changes you didn't touch, and stop only when the *same
file* changed on both sides. The lock's ownership and existence are untouched throughout.

1. Load config; resolve roots and identity.
2. Verify the remote root is present/mounted.
3. **Lock required — fail fast.** The marker must exist and be **yours (this machine)**; a
   missing marker, or one owned by anyone else, stops immediately (non-zero) *before* any diff or
   transfer. The local baseline (§6) must also be present — without it there is nothing to merge
   against, so `sync` refuses (re-checkout on this machine to establish one).
4. Three-way merge against the baseline (base = the tree at checkout, ours = local, theirs =
   remote), over `relpath` — or all held relpaths when omitted:

   | in base? | remote now | local now | meaning | action |
   |----------|------------|-----------|---------|--------|
   | yes | unchanged | edited    | local-only edit  | push |
   | yes | edited    | unchanged | remote-only edit | pull |
   | yes | edited    | edited    | **both changed** | **conflict → stop** |
   | no  | present   | absent    | remote addition  | pull |
   | no  | absent    | present   | local addition   | push |
   | yes | present   | gone      | local delete     | push delete (`--delete`) |
   | yes | gone      | present   | remote delete    | pull the delete (mirror it locally) |

   The `in base?` column is what separates a **remote addition** (pull) from a **local delete**
   (push the delete): both are "present on the remote, absent locally", and timestamps can't tell
   them apart.
5. **Conflict = the same path edited on both sides.** List the conflicting paths and stop without
   writing either side (`--force` resolves conflicts local-wins; it never overrides the lock
   check). With no conflicts, both directions are applied.
6. Refresh the marker's `last_sync_at` and re-snapshot the **baseline** to the reconciled state
   (so it stays the ancestor for next time). Ownership and the relpath set are left alone.

`--dry-run` prints the reconcile plan (and any would-be conflicts) and mutates nothing.

---

## 10. Locking & conflict rules

- The marker is a **cooperative lock**, not an OS-level lock — it only protects against
  other `netcheckout` users, not someone editing the share by hand.
- The lock is **per profile** (one marker at the remote root), so it covers every relpath at
  once — there are no nested or per-relpath locks. Checking out `./2025` while you already hold
  `./2025/jan` just widens what's pulled under the same lock; a *different* machine is refused
  regardless of which relpath it asks for.
- Ownership is **this-machine**: the marker's `checked_out_by` **and** `host` must both match.
  Your own identity on another host does **not** own it — reclaim by deleting the lock by hand.
- `--force` overrides, but always prints a loud warning naming the current holder.

---

## 11. Safety & error handling

- **Checkout never modifies the remote except to write the marker** — the remote copy is always preserved.
- **Checkout and check-in never move file data** — only `sync` copies files. Checkout never
  writes into the local working copy either; it only records local state (the baseline + marker).
- Refuse to run if a root is missing or the remote appears unmounted.
- `rsync` failures during `sync` abort the operation without deleting anything; partial transfers
  can be safely retried. A failed marker write on `checkout` rolls back the just-written baseline.
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

1. ~~**Language**~~ — **Resolved:** Go.
2. ~~**Marker filename**~~ — **Resolved:** `.netcheckout.json`, one per profile at the remote
   root (§5).
3. ~~**Config location**~~ — **Resolved:** YAML at the OS config dir (`os.UserConfigDir()/netcheckout/config.yaml`; see §4).
4. ~~**Local state**~~ — **Resolved:** required. The checkout baseline lives in a local state
   file (§6); `sync`'s three-way merge depends on it.
5. ~~**Nested checkouts**~~ — **Resolved:** the profile is the atomic lock unit (§2, §5, §10). A
   parent/child `relpath` only scopes which files copy; it never splits the lock.
6. ~~**Check-in `--delete`**~~ — **Resolved:** `checkin` reconciles like `sync`, so deletions
   propagate **baseline-scoped** — only files that were pulled and then removed — never a blunt
   rsync `--delete` mirror (§9).
7. ~~**`--clean` on check-in**~~ — **Resolved:** opt-in flag, off by default (§9).
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
   its own command, is still open. *(Now that `checkout`/`checkin` write markers, `status`'s
   `CheckedOut` reading is live rather than inert.)*

---

## 14. Roadmap / milestones

- **M1 — Foundations:** config schema + loader, identity resolution, `list`, `init`.
- **M2 — Read-only status:** `status` (rsync dry-run diff of local vs remote — done),
  and a **subpath discrepancy scan** that flags on-disk folders not covered by a
  profile's declared `subpaths` (see open question 8). Marker format and local-state
  reconciliation (see open question 9) remain open.
- **M3 — Checkout:** `checkout` (per-profile marker/lock + empty baseline; no file copy),
  conflict rules (this-machine ownership), the checkout **baseline** in local state, `--dry-run`,
  `--force`.
- **M4 — Check-in & sync:** `checkin` (verify-in-sync then release the whole profile, `--clean`,
  no `--force`) and `sync` (§9.5 three-way reconcile against the baseline — the single
  data-moving command, lock-required, stop on conflict). Both share the reconcile engine and the
  baseline.
- **M5 — Polish:** verbose/progress output, thorough error messages, tests.

---

## 15. Deferred follow-ups

Non-blocking refinements to already-shipped work, consciously deferred and recorded here
so they aren't lost. None affects the correctness of current behavior.

- **TUI `status` re-entry guard.** The TUI Status action (Activity box) runs
  `status.Compute` asynchronously and applies the result under a name guard, but does not
  stop a *second* run of the same profile from starting while one is already in flight
  (pressing Enter repeatedly, or leaving and reopening a profile mid-compute). Because
  `status` is a read-only `rsync` dry-run this is harmless — a later result is still a
  valid reading — but repeated Enters spawn redundant concurrent `rsync` processes. Fix:
  skip launching a new compute while `checking` is set (the Status branch of
  `updateProfile` in `app/tui/tui.go`).
- **Share the change-mark / diff formatting.** The TUI (`app/tui/profile.go`) and the CLI
  (`app/cmd/status.go`) each carry their own copy of the `+`/`-`/`M` change-mark and
  push/pull diff formatting. The duplication is intentional for now — the two render to
  different media, and the CLI file was out of scope when the TUI action was built. When
  `app/cmd/status.go` is next touched, hoist the identical `changeMark` into a shared
  helper (e.g. a `Mark()` method on `rsync.ChangeType`) used by both packages.
- **Per-subpath checkout state in the profile sanity check — RESOLVED.** The marker
  semantics landed as per-profile, not per-subpath (§5, §10): one lock at the remote root
  covers every relpath. So `sanity.Check` reporting a single aggregate "checked out" flag
  per profile is the correct, final shape — not a stopgap awaiting per-subpath upgrade.
  The per-subpath display idea below is moot and won't be pursued.
- **No staleness guard on the sanity-check cache.** The TUI runs the sanity checks in the
  background, keyed by profile name, latest result wins. A slow/stale check that resolves
  after its profile was deleted or renamed can briefly re-add (or, on same-name
  recreation, momentarily show) an out-of-date result in the Details box. It is harmless
  and self-correcting — Status is read-only and the next check overwrites it. A
  generation/epoch counter would close it but isn't justified for a display-only glitch.
