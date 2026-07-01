# CLAUDE.md

Guidance for working in this repo. Read before making changes.

## What this is

`lazytilt` is a terminal UI (Go + Bubble Tea) for [Tilt](https://tilt.dev), modeled on lazygit/lazydocker. It
hot-switches between multiple locally-running Tilt instances without a restart and renders Kubernetes and docker-compose
instances through one consistent UI. Module path: `github.com/abhishekrana/lazytilt`.

## Build / test / run

Day-to-day work goes through the Taskfile (`task --list`):

- `task build` / `task run` — compile to `bin/lazytilt` / build + run the TUI (flags after `--`)
- `task test` — `go test ./...`; live smoke test: `LAZYTILT_LIVE=1 go test ./internal/ui -run TestLiveSmoke -v`
- `task vet` / `task fmt` / `task tidy` — `go vet` / `go fmt` + prettier (md, 120 cols) / `go mod tidy`
- `task check` — fmt + vet + test + build; run before pushing

Tests are **hermetic** (decode `internal/tilt/testdata/*.json`, render the model, assert on the frame string). Color
tests force truecolor via `lipgloss.SetColorProfile(termenv.TrueColor)` (lipgloss strips color under the test default).

## Architecture

```
main.go                 flag parse + tea.NewProgram; starts the Hub
internal/tilt/          one Tilt instance: client + decode + actions (no UI)
  types.go              hand-rolled View/UIResource/LogList structs (camelCase JSON)
  client.go             /api/websocket_token (CSRF) + FetchView (GET /api/view, live test only); ParseView
  ws.go                 gorilla/websocket client for /ws/view: WatchView streams the snapshot + deltas
  accumulator.go        folds incremental ws deltas (changed resources, new log segments) into full Views
  status.go             updateStatus/runtimeStatus -> combined Status
  logs.go               span->manifest log assembly; AllLines = interleaved, source-tagged (All-Resources view)
  actions.go            shell out: tilt trigger|enable|disable <res> --port <port>; tilt snapshot create
internal/discovery/     find `tilt up` processes -> []Instance; Linux /proc, macOS ps/lsof (discovery_<goos>.go)
internal/ui/            Bubble Tea: app.go (model/Update/View), sidebar, logpane, detail, overview, theme, messages
  detail.go             resource detail strip above the logs (error/warnings/kind/build+recency/endpoints/labels)
  hub.go                I/O owner: discovers instances + one /ws/view per instance -> viewMsg/instancesMsg on a channel
  logview.go            windowed log renderer: wraps + paints only the visible rows (O(visible)/frame); owns scroll+follow
  overview.go           cross-instance ‹1› dashboard (landing screen) + top-bar health badges; esc/digit drills in
```

Data flow: the `Hub` is the single I/O owner — it rediscovers instances every ~2s and holds one `/ws/view` websocket per
instance. Tilt sends a full snapshot then incremental deltas; a `ViewAccumulator` folds each stream into a complete
`*View` that the Hub pushes as `viewMsg`/`instancesMsg` onto a channel the model drains via `listenCmd`. The UI only
re-renders on a change, the log buffer is capped (`accumulator.go` `maxLogSegments`), and `logview.go` paints just the
visible window — so CPU stays low whether idle or streaming a busy log. Actions shell out to the `tilt` CLI scoped by
`--port`; the status change then streams back.

## Conventions & gotchas (important)

- **Never commit real names.** Fixtures/examples/docs use mock names (`api`, `worker`, `db`, instances
  `app-one`/`app-two`). Never paste real resource/pod/container names, paths, or logs into the repo.
- **Don't import `github.com/tilt-dev/tilt`** (drags in ~22 k8s.io modules). Hand-roll minimal decode structs in
  `types.go`; Tilt's JSON is **camelCase**. The one network dependency is `gorilla/websocket`.
- **No painted backgrounds.** Theming is foreground-only (a bg fights Tilt's log ANSI). See `theme.go`.
- **Sanitize log output.** `sanitizeLogLine` strips `\r` and other C0 controls before rendering (raw `\r` corrupts the
  layout).
- **Tab numbering: `‹1›` = overview, `‹2›…‹9›` = instances.** Must agree across `renderTopBar`, the overview header tags
  (`i+2`), and the digit handlers (`'2'` ⇒ index 0).
- **Sidebar index 0 is the synthetic "All Resources" row**; the navigable rows below it are `selectableRows()` — each
  resource, then its workloads when a helm release bundles more than one. Selection index `i+1` maps to
  `selectableRows()[i]`; keep it in lockstep with `renderSidebar`'s row order. `selectedResource()` returns the owning
  resource (the parent for a workload child; `ok=false` on All-Resources, so resource actions no-op there),
  `selectedWorkload()` reports a workload child, and `onAllLogs()` reports index 0.
- **Sidebar grouping is label-driven.** Keep `selectableRows()`/`visible()` and `renderSidebar` deriving from the same
  `sidebarGroups()` so order and headers never drift; group headers aren't selectable.
- **Helm releases bundle workloads.** `k8sResourceInfo.displayNames` lists every managed object as `<name>:<kind>`;
  `Workloads()`/`WorkloadKinds()` filter to deployable kinds. Helm _hooks_ (e.g. pre-upgrade jobs) are NOT in
  `displayNames` — Tilt doesn't track them, so lazytilt can't show them. Per-workload pod status isn't exposed either,
  so a bundle's header shows the workload count and a child row shows just the pod name (read from
  `pod:<manifest>:<pod>` log span IDs).
- **Keep it simple**, and use **no `Co-Authored-By`** lines in commits.

## Verifying UI changes

Tests render the model and assert on `View()`. For a real visual check, run the binary in a pty with a window size set
(it shows `loading…` until it gets dimensions) and `COLORTERM=truecolor`.

## Releasing

Push a `v`-prefixed SemVer tag (pre-1.0, so `v0.x`) — `.github/workflows/release.yml` runs GoReleaser to cross-build
linux/darwin archives and publish a GitHub Release. `ci.yml` runs vet/test/build on every push/PR. Never move a
published tag; bump to the next patch. Check the embedded version with `lazytilt --version`.

```sh
task check                               # main must be green and pushed first
git tag -a v0.2.0 -m "lazytilt v0.2.0"   # annotated SemVer tag
git push origin v0.2.0                    # triggers release.yml
```
