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
- **Our orchestration in `yb.yaml`.** Version, cache dir, ssh key, and extra bind
  mounts live in a separate file, so the kas files stay portable.

See [docs/design/2026-07-06-yb.md](docs/design/2026-07-06-yb.md) for the design.

## Build

```sh
make build            # -> ./yb
make install          # -> $GOPATH/bin/yb
```

## Use

In a project that has kas files (e.g. `irisentinel.yml`), add a `yb.yaml`:

```yaml
kas_file: irisentinel.yml
version:  zeus              # yb builds yb-yocto:zeus (Ubuntu 18.04 + python2)
cache:    /srv/yocto-cache
ssh_key:  ~/.ssh/iri
mounts:
  - /srv/old-hab-keys/irisentinel:ro
# image: my/prebuilt:tag   # optional — skip image building, use this instead
```

Known versions: `zeus`, `dunfell`, `gatesgarth`, `hardknott`, `honister`,
`kirkstone`, `langdale`, `mickledore`, `nanbield`, `scarthgap` (extend the table
in `internal/image/image.go`). The first build builds the image (cached
thereafter; `--rebuild` forces it).

Then:

```sh
yb build                 # checkout + conf + bitbake the target(s) from the kas file
yb build core-image-base # build a specific target
yb build --dry-run       # print the plan (repos, confs, docker command) — change nothing
yb shell                 # bitbake build shell inside the container
```

Flags: `-C <dir>` project dir, `-f <kasfile>`, `-version <v>`, `-image <name>`
(use a prebuilt image), `-machine <m>`, `--rebuild` (rebuild the version image).

## Commands

```
build [targets...]   checkout repos, generate conf, run bitbake in the container
                     (--dry-run, --no-checkout)
shell                open a bitbake build shell in the container
version
```
