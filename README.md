# netcheckout

A CLI utility to **check out** and **check in** work directories over network drives
(e.g. a locally-mounted Samba/NAS share), using `rsync` to copy files between a remote
root and a local working copy, and leaving a marker behind so others can see a folder is
checked out and by whom.

See [`GOALS.md`](./GOALS.md) for the full design.

> Status: bootstrap — only the `version` command is implemented so far.

## Install

### Homebrew

A macOS cask is published into this repository on every tagged release. Because the repo
isn't named `homebrew-*`, tap it with an explicit URL, then install:

```bash
brew tap andresbott/tap https://github.com/andresbott/netcheckout
brew install --cask andresbott/tap/netcheckout
```

`rsync` is pulled in as a dependency, and `brew upgrade` will track future releases.

Or grab a prebuilt archive from the
[releases page](https://github.com/andresbott/netcheckout/releases).

## Build

```bash
make build        # goreleaser snapshot build for the current OS/arch → ./dist
# or plain go:
go build ./...
```

## Run

```bash
go run main.go version
```

## Develop

```bash
make verify       # test → license-check → lint → benchmark → coverage
make help         # list all targets
```

Requires Go 1.26+. The full toolchain also needs `golangci-lint`, `goreleaser`, and
`go-licence-detector`; at runtime `rsync` must be on `PATH`.
