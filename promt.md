# Prompt — design & build the `checkout`, `checkin`, and `sync` actions

> Paste this into a fresh session that has **superpowers**. Start with the
> **brainstorming** skill to lock the design, then **writing-plans**, then implement
> with **test-driven-development**. Do the design *before* touching code.

---

## Context

`netcheckout` is a Go CLI that checks work directories out of / in to a network drive
(a locally-mounted SMB/NFS share) using `rsync`, leaving a marker file behind so others
can see who holds a folder. **`GOALS.md` is the source of truth** for the design — read
it first, especially §5 (marker), §7 (commands), §8 (checkout flow), §9 (check-in flow),
§10 (locking), and §13 (open questions).

Current state:

- **Shipped:** config schema + loader, identity resolution, `list`, `init`, `status`
  (rsync dry-run diff of local vs remote), and the profile-management TUI.
- **Not built:** `checkout` (milestone M3) and `checkin` (M4). No code writes or removes
  a marker yet.
- The **e2e lifecycle suite** (on the `e2e-test` branch, `zarf/e2e`) is currently **red at
  the missing `checkout` command** — landing these actions should bring it green.

## Build on what exists — reuse, don't reinvent

| Piece                                   | Location                                             | Role                                                                               |
|-----------------------------------------|------------------------------------------------------|------------------------------------------------------------------------------------|
| `Syncer.Sync(ctx, Job) (Result, error)` | `internal/rsync/sync.go`                             | The real rsync transfer engine. **All three actions drive this.**                  |
| `Args` / `Differ`                       | `internal/rsync/rsync.go`                            | rsync arg building + dry-run diff.                                                 |
| `status.Compute(ctx, Differ, profile)`  | `internal/status/status.go`                          | local↔remote diff that powers `status`.                                            |
| `config.Load(path)`                     | `internal/config/config.go`                          | config, profiles, identity, `subpaths`.                                            |
| `sanity.Check(...)`                     | `internal/sanity`                                    | reads the marker per profile — **inert today** because nothing writes markers yet. |
| Marker                                  | `remote_root/.netcheckout.json` (GOALS §5) | atomic write, cooperative lock. Writing/removing it is core to this work.          |

Commands are cobra commands in `app/cmd/` (e.g. `list.go`, `status.go`) wired under the
root command; the TUI lives in `app/tui/`.

## The three actions to define

### 1. `checkout <profile> [relpath]` — acquire  (GOALS §8, M3)
Pull `remote_root/<relpath>` → `local_root/<relpath>` (remote is left untouched) and write
the marker as a cooperative lock. `relpath` only scopes *which files are pulled* — the lock is
always the **whole profile**. Conflict rule (§10): a marker is "yours" only when *this machine*
wrote it (`checked_out_by` **and** `host` match); any other existing marker — including your own
identity on a different host — is **refused**, and the remedy is to delete the lock by hand
(there is no automatic cross-machine recovery). `--force` overrides with a loud warning. Roll
back cleanly on any failure — never leave a marker behind after a failed transfer.

**The marker is per-profile, not per-relpath.** A profile has exactly **one** marker,
written at its *remote root* — `remote_root/.netcheckout.json` — even when the profile
declares `subpaths` or you check out a nested `relpath`. That one marker is the lock for
the whole profile: `relpath` only scopes *which files get copied*, never where the marker
lives or what it locks, so a profile is held as a unit (you can't have `./jan` locked by
you while `./march` stays free).

**What the marker records.** Written on checkout and updated as relpaths are added or
checked back in, the marker captures the *set* of relpaths currently pulled down, plus
last-operation timestamps — when the profile was first checked out, and when it was last
synced. It extends the GOALS §5 schema, e.g.:

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

> This **supersedes** the original GOALS.md, which put the marker at
> `remote_root/<relpath>/.netcheckout.json` (§5) and described "many independent
> checkouts — one per relpath" (§2). **Now reconciled in GOALS.md:** §2 and §5 use the
> per-profile rule, the expanded marker schema (relpath list + `last_sync_at`) is folded into
> §5, and the nested-checkout open question (§13 Q5) is closed.

### 2. `checkin <profile>` — release  ("checking"; GOALS §9, M4)
Finish and release the **whole profile**: reconcile exactly like `sync` (§3) — push your local
work back, pull any remote-only changes, and **stop on a same-file conflict** — then, on a clean
reconcile, remove the marker and clear the local baseline. Confirm the marker exists and is
yours (this machine) before touching anything (refuse on mismatch unless `--force`). **`checkin`
takes no `relpath`:** the marker is profile-wide (§1), so a checkout is released as a unit and
**partial check-in is not supported** — you always push back everything you currently hold.
`--clean` (default off) additionally removes the local working copy after a successful release.
The old `--delete` question falls out of the reconcile: deletions propagate, but only
baseline-scoped (files you pulled and then removed), never a blunt mirror.

### 3. `sync <profile> [relpath]` — reconcile in place, stop on true conflicts  (**NEW — not in GOALS**)
The interim operation for a checkout you're still holding: **push your local work back to the
remote, pull in the remote changes you didn't touch, and stop only when the *same file* changed on
both sides** so you resolve it by hand. The lock is untouched throughout.

Decided semantics:

- **Lock required — fail fast.** Sync **only if the marker exists and is yours**. No marker, or
  one owned by someone else → stop immediately (non-zero exit) *before* any diff or transfer.
- **A clean sync reconciles both directions** (no conflict ⇒ no stopping):
  - changed **only on the remote** (a file added, or edited while you left your copy alone) →
    **pull it down**;
  - changed **only locally** (added, edited, or **deleted**) → **push it up**, propagating the
    delete to the remote;
  - then refresh the marker's `last_sync_at` (ownership + relpath set left alone).
- **Conflict = the *same path* changed on *both* sides** (edited locally *and* on the remote).
  List the conflicting paths and **stop without writing either side** — never auto-merge or clobber.

**Why a baseline — the intuition.** At sync time you hold two versions: your local copy and the
current remote. Comparing them *directly* can't drive the decision, because **a difference doesn't
say who caused it** — if a file differs, did you edit it (push), did the remote (pull), or both
(conflict)? To answer that you need a third reference: the folder **as it was at checkout**, when
local and remote were identical. That snapshot is the common ancestor, and `sync` is a **three-way
merge** against it — exactly like `git merge`: base = the checkout snapshot, ours = local, theirs =
remote. Measuring each side against the *base* (rather than against each other) is what makes every
case decidable. Without that base you cannot:

- separate a **one-sided change from a both-sided conflict** — a plain `rsync -n` diff is
  symmetric, and an mtime/`--update` heuristic sees only "which side is newer," so a file edited on
  both sides (local happens to be newer) reads as a clean local edit and silently clobbers the
  remote; and
- tell a **delete from an addition** — "present on the remote, absent locally" is either *you
  deleted it* (propagate the delete) or *the remote added it* (pull it), and **timestamps can't
  disambiguate**: a deleted file has no timestamp, and a new remote file's timestamp says nothing
  about why it's missing locally.

So capture a **checkout baseline**: at minimum a `path → size+mtime` snapshot of what was pulled
(checksums instead of mtime if network-share mtimes prove unreliable). Every case then falls out of
comparing both sides against the base:

| in base? | remote now | local now  | meaning          | action                     |
|----------|------------|------------|------------------|----------------------------|
| yes      | unchanged  | edited     | local-only edit  | push                       |
| yes      | edited     | unchanged  | remote-only edit | pull                       |
| yes      | edited     | edited     | **both changed** | **conflict → stop**        |
| no       | present    | absent     | remote addition  | pull                       |
| no       | absent     | present    | local addition   | push                       |
| yes      | present    | **gone**   | local delete     | push delete (`--delete`)   |
| yes      | **gone**   | present    | remote delete    | **pull the delete** (mirror it locally) |

> **The clincher — rows `remote addition` and `local delete`.** In *both* the file is present on the
> remote and absent locally, so from a plain local-vs-remote view they look **identical** — yet one
> must be **pulled down** and the other **deleted from the remote**, opposite actions. The only
> thing that separates them is the **`in base?`** column: *not* in the baseline → the remote added
> it (pull); *in* it → you deleted it (push the delete). Timestamps can't help — a missing file has
> no timestamp. After a clean sync, refresh the baseline to the reconciled state so it stays the
> ancestor for next time (the content-level analog of `last_sync_at`).

This promotes the baseline from an optional upgrade to a **requirement**, closing GOALS open
Q4/§6 (keep local state) with **yes**. It lives in a **local state file** (per profile, e.g.
`~/.local/state/netcheckout/<profile>.json`), not on the remote: the baseline is inherently
per-machine, and keeping it local leaves the shared marker small and human-readable (§5) while
avoiding a large manifest written to the slow share on every sync. The local baseline is the
*authoritative* content snapshot; the marker's `relpaths[]` is only the human-readable "what
they hold".

Resolved sub-decisions: a **remote-side deletion** of a file you didn't touch is **pulled**
(mirrored locally) — a deletion is just another remote-only change. `--force` overrides
**conflicts** in favor of local, but **never** the lock-ownership check. With no `relpath`,
`sync` reconciles **all** relpaths currently held (the whole checkout); it never refuses for a
missing relpath.

**Boundary:** `checkout` acquires the lock and does the initial full pull; `checkin` does the final
push and releases; `sync` is the in-between reconcile that changes **neither** the lock's ownership
nor its existence.

## Open questions this work closes  *(resolved 2026-07-11)*

From GOALS §13, folded back into GOALS.md:

- **Q2 marker filename** → `.netcheckout.json`, one per profile at the remote root.
- **Q4 / §6 keep local state** → **yes, required** — the checkout baseline lives in a local
  state file (see §3).
- **Q5 nested checkouts** → settled: the profile is the atomic lock unit; a parent/child
  `relpath` only scopes which files copy, never the lock.
- **Q6 check-in `--delete`** → subsumed: `checkin` reconciles like `sync`, so deletes propagate
  baseline-scoped (only files you pulled and removed).
- **Q7 `--clean`** → opt-in flag, off by default.

`sync` is new, so it gets its own section in GOALS (§9.5), alongside the reconciled per-profile
marker (§2/§5) and the now-required local baseline (§6).

## E2E coverage the suite must gain

The `zarf/e2e` lifecycle suite already drives checkout→checkin; extend it with a **sync stage** and
the reconcile/conflict scenarios that define `sync`:

1. **Push local edit (clean):** checkout → edit a local file → `sync` → the remote reflects the
   edit; marker intact, still owned by you, relpath set unchanged, `last_sync_at` bumped.
2. **Pull remote-only add (clean, *not* a conflict):** checkout → add a new file on the remote that
   never existed locally → `sync` → the file is pulled down and the run succeeds.
3. **Local delete vs. remote add — disambiguated:** in one run, (a) delete a file locally that came
   from the checkout and (b) add a different, brand-new file on the remote → `sync` → (a) is removed
   on the remote and (b) is pulled down locally. The baseline is what tells these two "absent on one
   side" cases apart; assert neither is mishandled (the deleted file is not resurrected, the
   remote-added file is not deleted).
4. **True conflict → stop (the key case):** checkout → edit file `F` locally → edit the **same**
   file `F` out-of-band on the remote → `sync` → `F` is reported as a conflict, exit non-zero, and
   **nothing is written on either side**; the marker is untouched.
5. **No lock → fail fast:** `sync` with no marker, and against a marker owned by *someone else* →
   both exit non-zero **before** any transfer; the remote is byte-for-byte untouched.
6. **Override (only if `--force`-overrides-conflicts is adopted):** repeat (4) with `--force` →
   local wins, the push proceeds, `last_sync_at` bumped.
7. **`--dry-run sync`:** prints the reconcile plan (and any would-be conflicts) and mutates nothing.

Scenarios 3 and 4 are the acceptance tests — design detection so they're deterministic (seed the
baseline and control mtimes/checksums explicitly rather than leaning on wall-clock ordering).

## Constraints / working agreement

- Go; single static binary; shells out to `rsync`. Match the existing package layout and
  style.
- **TDD** — write the test first. Unit tests sit beside each package; the `zarf/e2e`
  lifecycle suite is the integration check.
- Every mutating command supports `--dry-run` and `--force`; respect `--config` and
  `-v/--verbose` (GOALS §7).
- **Work directly on the active feature branch (currently `e2e-test`). Do NOT create a git
  worktree** — this repo's feature work happens on the branch itself.
- New files get the AGPL-3.0 license header.

## Definition of done

- `checkout`, `checkin`, and `sync` implemented as cobra commands under `app/cmd/`, wired
  into the root command (and the TUI where it makes sense).
- Marker write/read/remove implemented; `internal/sanity` is no longer inert.
- Unit tests plus the `zarf/e2e` lifecycle suite pass — including the new `sync` stage, the
  delete-vs-add disambiguation, and the true-conflict scenario (see E2E coverage) — via
  `make verify` (or `/verify`), green before anything is claimed done.
- `GOALS.md` updated to match what shipped: `sync` documented (three-way reconcile + lock-required
  + conflict-stop, plus the checkout baseline that Q4 now requires), closed open questions recorded.
