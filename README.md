# yb

`yb` builds Yocto images from **kas-format YAML**, the way we need it: it builds
its **own build container aligned to the Yocto release** (`version: zeus` → an
18.04 image with python2; `kirkstone` → 22.04), then runs the build in it with the
shared host cache, HAB signing keys, and ssh key wired in automatically. A single
static Go binary — no Python, no `kas-container`, no prebuilt image to manage.

- **Builds the image from the version.** The release name in `yb.yaml` selects an
  Ubuntu base + host packages (`internal/image`); yb builds `yb-yocto:<version>`
  on demand from an embedded Dockerfile. No kas is installed — yb does checkout,
  conf, and bitbake itself.
- **Reads existing kas files.** `machine`, `distro`, `target`, `repos`/`layers`,
  `local_conf_header`, and `header.includes` are parsed and deep-merged exactly
  as kas does. Repo refs use `commit:` or `branch:`.
- **Our orchestration in a `yb:` block.** Version, `dl`/`sstate` dirs, ssh key,
  and extra bind mounts live in a `yb:` block in the kas file; kas ignores it.

See [docs/design/2026-07-06-yb.md](docs/design/2026-07-06-yb.md) for the design.

## Install

Grab a prebuilt static binary from the latest release (no Go toolchain needed):

```sh
sudo curl -fsSL https://github.com/haonguy3n/yb/releases/latest/download/yb-linux-amd64 \
  -o /usr/local/bin/yb && sudo chmod +x /usr/local/bin/yb
yb version
```

(`yb-linux-arm64` is also published; each release carries `SHA256SUMS`.)

Or build from source:

```sh
make build            # -> ./yb
make install          # -> $GOPATH/bin/yb
```

Releases are cut by pushing a `v*` tag — the `release` workflow builds and
attaches the binaries.

## Use

Add a `yb:` block to your top-level kas file (e.g. `irisentinel.yml`). kas
ignores it; yb reads it:

```yaml
# irisentinel.yml
header:
  includes: [base.yml]
machine: irisentinel9x9
distro:  iritech-imx6ul
target:  [iritech-hab-firmware]

yb:
  version: zeus              # yb builds yb-yocto:zeus (Ubuntu 18.04 + python2)
  dl:      /srv/yocto-cache/downloads          # optional DL_DIR (safe to share)
  sstate:  /srv/yocto-cache/sstate-irisentinel # optional SSTATE_DIR (per-project keeps it isolated)
  ssh_key: ~/.ssh/iri
  mounts:
    - /srv/old-hab-keys/irisentinel:ro
  # image: my/prebuilt:tag   # optional — skip image building, use this instead
```

When set, `dl`/`sstate` become `DL_DIR`/`SSTATE_DIR`; when unset, yb omits them
and Yocto uses its own defaults. Sharing `dl` across projects deduplicates
source downloads; a per-project `sstate` keeps each release's shared-state cache
isolated. yb creates and mounts only the paths you set.

Known versions: `zeus`, `dunfell`, `gatesgarth`, `hardknott`, `honister`,
`kirkstone`, `langdale`, `mickledore`, `nanbield`, `scarthgap` (extend the table
in `internal/image/image.go`). The first build builds the image; it is cached
thereafter.

**Omit `version` (and `image`) to build natively on the host** — no container,
bitbake runs directly. Use this when the host already has the Yocto build deps
(e.g. a set-up CI runner); layer paths are the real project paths instead of
`/work`. Set a `version` when you want the release-aligned container (e.g. zeus
on a modern host).

Run yb **from the project directory**:

```sh
yb build                 # auto-detects the kas file carrying the yb: block
yb build irisentinel.yml            # or name the file (kas-style positional)
yb build a.yml b.yml     # overlay files, kas-style (also a.yml:b.yml)
yb build iritech-hab-firmware       # build a specific target
yb build --force         # force git checkout/pull to the pinned commit/branch
yb shell                 # bitbake build shell inside the container
```

Everything else — version, dl, sstate, ssh_key, mounts — comes from the `yb:` block.
A positional naming an existing `*.yml` file is a kas entry file; other
positionals are bitbake targets.

## Commands

```
build [file.yml ...] [targets...]   checkout repos, gen conf, run bitbake (--force)
shell [file.yml ...]                open a bitbake build shell in the container
version
```
