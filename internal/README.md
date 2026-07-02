# internal/

Domain logic for netcheckout, not importable by other modules.

Planned packages (see [`../GOALS.md`](../GOALS.md)):

- **config** — config schema + loader, identity resolution.
- **marker** — read/write the remote `.netcheckout.json` marker (the cooperative lock).
- **rsync** — wrapper around the `rsync` binary for copy/sync operations.
- **checkout** — checkout / check-in orchestration and conflict rules.
- **state** — local state cache of active checkouts.

Empty for now — this file only keeps the directory tracked in git.
