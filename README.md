# lazytilt

A terminal UI for [Tilt](https://tilt.dev), in the spirit of lazygit / lazydocker.

lazytilt mirrors the Tilt web UI in your terminal and lets you **switch between multiple Tilt instances running on the
same machine without restarting** — handy when you have several `tilt up` sessions going at once. Kubernetes and
docker-compose instances render through one consistent UI.

It opens on a cross-instance **overview** (`‹1›`) summarizing the health of every discovered instance; `esc` or a digit
key drills into a single instance, and `1` brings the overview back.

## Install

**Prebuilt binaries** (Linux and macOS; amd64 and arm64) are attached to every
[release](https://github.com/abhishekrana/lazytilt/releases) — download the archive for your platform, extract, and put
`lazytilt` on your `PATH`. For example:

```sh
# Linux amd64 — adjust the version/platform to match the release
curl -sSL https://github.com/abhishekrana/lazytilt/releases/latest/download/lazytilt_0.1.0_linux_amd64.tar.gz \
  | tar -xz
sudo mv lazytilt /usr/local/bin/
```

**With Go** (1.25+) you can install the latest release straight from source:

```sh
go install github.com/abhishekrana/lazytilt@latest
```

**From a checkout** (for development):

```sh
go build -o lazytilt . && ./lazytilt
```

`lazytilt --version` prints the build version.

## Run

lazytilt auto-discovers running `tilt up` processes (Linux via `/proc`, macOS via `ps`/`lsof`) and reads each one's
`--port` / `TILT_PORT`. If none are found it falls back to `--host`/`--port` (defaults `localhost:10350`).

The sidebar's first entry, **All Resources**, streams every resource's logs interleaved with global Tilt output, each
line tagged with its source — handy for watching everything at once. It's the default selection when you open an
instance; pick a single resource to narrow the logs to it.

When resources carry Tiltfile **labels**, the sidebar groups them under label headers (each with a per-group status
rollup), sorted by label name with resources alphabetical inside each group. The headers are dividers only — selection
still moves resource-to-resource. With no labels it's a flat list in Tilt's order.

Selecting a resource shows a **detail strip** above its logs — the last build error and warnings (when present), the
workload kind, how long ago it last built, endpoints, and labels — each line appearing only when it carries something.

A **helm release** that bundles several workloads under one Tilt resource lists them as nested child rows; select a
workload to narrow the logs to that workload's pods. (Tilt exposes no per-workload runtime status for a bundle, so a
child row shows just the pod name, and the release header shows its workload count.)

Themes: `--theme solarized-light` (default), `solarized-dark`, or `dark`; press `T` in-app to cycle.

## Keys

| Key             | Action                                                           |
| --------------- | ---------------------------------------------------------------- |
| `↑`/`k` `↓`/`j` | move selection                                                   |
| `⏎` / `tab`     | focus the log pane / toggle pane                                 |
| `esc`           | back to the resource list (from the overview: into the instance) |
| `1`             | overview — cross-instance health (the landing screen)            |
| `F`             | overview: show only failing instances                            |
| `2`…`9`         | jump directly to that Tilt instance                              |
| `[` `]`         | previous / next Tilt instance (no restart)                       |
| `r`             | trigger (rebuild) the selected resource (asks y/n)               |
| `R`             | trigger all resources in the instance (asks y/n)                 |
| `d`             | enable / disable the selected resource (asks y/n)                |
| `/`             | search logs (highlights matches)                                 |
| `f`             | toggle log follow/tail                                           |
| `L`             | cycle log level (all / errors / warnings)                        |
| `c`             | clear the log search filter                                      |
| `o`             | open the selected resource's logs in `$EDITOR` (else vim)        |
| `s`             | save the selected resource's logs to a temp file                 |
| `S`             | snapshot the active instance (`tilt snapshot`)                   |
| `T`             | cycle color theme                                                |
| `g` / `G`       | jump to top / bottom of logs                                     |
| `?`             | help                                                             |
| `q` / `ctrl+c`  | quit                                                             |

## How it works

- **State** is streamed from Tilt's `/ws/view` websocket (the same feed the web UI uses): an initial snapshot then
  incremental deltas, so the UI does work only when something actually changes. The handshake is authenticated with a
  CSRF token from `/api/websocket_token` (fetched using the session token at `~/.tilt-dev/token` when present).
- **Actions** (trigger / enable / disable) shell out to the `tilt` CLI scoped with `--port`, so they target the right
  instance; the resulting status change just streams back.
- **Discovery** scans local `tilt up` processes for their port and working directory.

Status colors: green = ok, red = error, orange = building, yellow = pending, grey = disabled/idle.

Developed and tested against **Tilt v0.37.4**. lazytilt only reads Tilt's View JSON, so nearby versions should work, but
that's the version the fixtures and live tests track.
