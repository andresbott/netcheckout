# Sample netcheckout config

A ready-to-read example showing the config format and the local/remote-root
layout for two profiles (`photos` and `work`).

- `config.yaml` — the sample configuration (an `identity` plus two profiles).
- `photos/`, `work/` — each profile's `local/` (fast working copy) and `remote/`
  (canonical copy on the network share) roots.

Roots in `config.yaml` must be **absolute** paths. The example uses a
`/path/to/netcheckout/...` placeholder — replace it with the absolute path to
your checkout to try it against these folders:

```bash
netcheckout --config zarf/sample/config.yaml list
```

Run without `--config` and `netcheckout` reads `$NETCHECKOUT_CONFIG`, or the OS
config dir (Linux `~/.config/netcheckout/config.yaml`, macOS
`~/Library/Application Support/netcheckout/config.yaml`, Windows
`%AppData%\netcheckout\config.yaml`).
