# lazytilt

A terminal UI for [Tilt](https://tilt.dev), in the spirit of lazygit / lazydocker.

lazytilt mirrors the Tilt web UI in your terminal and lets you **switch between
multiple Tilt instances running on the same machine without restarting** — handy
when you have several `tilt up` sessions going at once. Kubernetes and
docker-compose instances render through one consistent UI.

## Install / run

```sh
go build -o lazytilt .
./lazytilt
```

lazytilt auto-discovers running `tilt up` processes (Linux, via `/proc`) and
reads each one's `--port` / `TILT_PORT`. If none are found it falls back to
`--host`/`--port` (defaults `localhost:10350`).

Themes: `--theme solarized-light` (default), `solarized-dark`, or `dark`; press
`T` in-app to cycle.

## Keys

| Key | Action |
| --- | --- |
| `↑`/`k` `↓`/`j` | move selection |
| `⏎` / `tab` | focus the log pane / toggle pane |
| `esc` | back to the resource list |
| `[` `]` | previous / next Tilt instance (no restart) |
| `r` | trigger (rebuild) the selected resource |
| `e` / `d` | enable / disable the selected resource |
| `/` | filter (resources or logs, depending on focus) |
| `f` | toggle log follow/tail |
| `L` | cycle log level (all / errors / warnings) |
| `c` | clear the log text filter |
| `s` | show/hide disabled resources |
| `T` | cycle color theme |
| `g` / `G` | jump to top / bottom of logs |
| `?` | help |
| `q` / `ctrl+c` | quit |

## How it works

- **State** is read by polling Tilt's `GET /api/view` (the same data the web UI
  uses), authenticated with the token at `~/.tilt-dev/token` when present.
- **Actions** (trigger / enable / disable) shell out to the `tilt` CLI scoped
  with `--port`, so they target the right instance.
- **Discovery** scans local `tilt up` processes for their port and working
  directory.

Status colors: green = ok, red = error, orange = building, yellow = pending,
grey = disabled/idle.
