# yb

`yb` builds Yocto images from **kas-format YAML**, the way we need it: inside our
own `yocto-kas` container, with the shared host cache, HAB signing keys, and ssh
key wired in automatically. It is a single static Go binary — no Python, no
`kas-container` shell wrapper.

- **Reads existing kas files.** `machine`, `distro`, `target`, `repos`/`layers`,
  `local_conf_header`, and `header.includes` are parsed and deep-merged exactly
  as kas does. Repo refs use `commit:` or `branch:`.
- **Our orchestration in `yb.yaml`.** The container image, cache dir, ssh key, and
  extra bind mounts live in a separate file, so the kas files stay portable.
- **Runs bitbake in the container.** yb checks out repos on the host, writes
  `local.conf`/`bblayers.conf`, then runs `bitbake` inside the image.

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
image:    yocto-kas:zeus
cache:    /srv/yocto-cache
ssh_key:  ~/.ssh/iri
mounts:
  - /srv/old-hab-keys/irisentinel:ro
```

Then:

```sh
yb build                 # checkout + conf + bitbake the target(s) from the kas file
yb build core-image-base # build a specific target
yb build --dry-run       # print the plan (repos, confs, docker command) — change nothing
yb shell                 # bitbake build shell inside the container
```

Flags: `-C <dir>` project dir, `-f <kasfile>`, `-image <name>`, `-machine <m>`.

## Commands

```
build [targets...]   checkout repos, generate conf, run bitbake in the container
                     (--dry-run, --no-checkout)
shell                open a bitbake build shell in the container
version
```
