# CLAUDE.md

Guidance for working in this repo. Read before making changes.

## What this is

`lazytilt` is a terminal UI (Go + Bubble Tea) for [Tilt](https://tilt.dev), modeled on lazygit/lazydocker. It
hot-switches between multiple locally-running Tilt instances without a restart and renders Kubernetes and docker-compose
instances through one consistent UI.

Module path: `github.com/abhishekrana/lazytilt`.

## Build / test / run

Day-to-day work goes through Taskfile (`task --list` to see all); `./bootstrap.sh` installs the `task` CLI itself.

- `task build` — compile to `bin/lazytilt`
- `task run` — build + run the TUI (forward flags after `--`, e.g. `task run -- --port 10351 --theme solarized-dark`)
- `task test` / `task test-live` — `go test ./...` / the gated live smoke test below
- `task vet` — `go vet ./...`
- `task fmt` — `go fmt` + `prettier --write` on markdown (120 cols)
- `task check` — fmt + vet + test + build (use before pushing)
- `task tidy` — `go mod tidy`

The equivalent without task:

```sh
go build ./... && go vet ./... && go test ./...   # before every commit
go run .                                           # launch the TUI
go run . --port 10351 --theme solarized-dark       # fallback instance + theme
```

- Tests are **hermetic** by default (decode against `internal/tilt/testdata/*.json`, render the Bubble Tea model and
  assert on the frame string).
- The live smoke test hits real running Tilts and is gated:
  `LAZYTILT_LIVE=1 go test ./internal/ui -run TestLiveSmoke -v`.
- Color tests force a truecolor profile via `lipgloss.SetColorProfile(termenv.TrueColor)` because lipgloss strips color
  under the default (Ascii) profile in tests.

## Architecture

```
main.go                 flag parse + tea.NewProgram
internal/tilt/          one Tilt instance: client + decode + actions (no UI)
  types.go              hand-rolled View/UIResource/LogList structs (camelCase JSON)
  client.go             GET /api/view (+ X-Tilt-Token), ParseView
  status.go             updateStatus/runtimeStatus -> combined Status; backend + runtime line
  logs.go               span->manifest log assembly; AllLines = interleaved, source-tagged (All-Resources view)
  actions.go            shell out: tilt trigger|enable|disable <res> --port <port>; tilt snapshot create
internal/discovery/     find `tilt up` processes -> []Instance; Linux /proc, macOS ps/lsof (discovery_<goos>.go)
internal/ui/            Bubble Tea: app.go (model/Update/View), sidebar, logpane, overview, theme, messages
  overview.go           cross-instance ‹1› dashboard (the landing screen) + top-bar health badges; esc/digit drills in
```

Data flow: a 1s tick fetches `GET /api/view` for **every** discovered instance, caching each by port in `Model.views`
(so the top-bar badges and the ‹1› overview show cross-instance health without switching); the active instance's
response also drives the focused pane and log viewport. The websocket is intentionally **not** used (polling is simpler
and was the deliberate choice). Actions shell out to the `tilt` CLI scoped by `--port`. Discovery re-runs every tick
(the /proc scan is only a few ms) so start/stop is reflected within ~1s, pruning cached views for instances that
disappear.

## Conventions & gotchas (important)

- **Never commit real names.** Test fixtures, examples, and docs use mock names (`api`, `worker`, `db`, `web`,
  `metrics`, instances `app-one`/`app-two`). Do not paste resource/pod/container names, paths, or logs captured from a
  real local Tilt into the repo.
- **Don't import `github.com/tilt-dev/tilt`.** It drags in ~22 k8s.io modules. We hand-roll the minimal decode structs
  in `internal/tilt/types.go`. Tilt's JSON is **camelCase**.
- **No painted backgrounds.** Theming is foreground-only; we rely on the terminal's own background so the screen stays
  even (painting a bg fights Tilt's log ANSI and looked patchy). See `internal/ui/theme.go`.
- **Sanitize log output.** `sanitizeLogLine` strips carriage returns (curl/progress output) and other C0 controls before
  rendering — verbatim `\r` jumps the cursor to column 0 and corrupts the layout.
- **Tab numbering is `‹1›` = overview, `‹2›…‹9›` = instances.** This invariant spans three places that must agree: the
  top bar (`renderTopBar`), the overview header tags (`renderOvHeader`, `i+2`), and the digit-key handlers (`'2'` ⇒
  instance index 0). The overview itself is reached/left with `1`; out-of-range digits are no-ops.
- **Sidebar index 0 is the synthetic "All Resources" row.** Its logs are the combined stream of every resource (plus
  global Tilt output); resources live at selection index `i+1`. `selectedResource()` returns `ok=false` on the All row,
  so resource-scoped actions (trigger/disable/save) are no-ops there; `onAllLogs()` reports the row. Anything mapping a
  resource to a selection index (e.g. `selectByName`) must add the +1 offset.
- **Sidebar grouping is label-driven.** When any resource has a label, `sidebarGroups()` arranges resources into label
  groups (sorted by label, resources alphabetical within; `(no label)` last); with no labels it's a flat group in Tilt
  order. `visible()` flattens the groups, so `selected` still indexes only the All row + resources — the rendered group
  headers are **not** selectable (selection skips them). Keep `visible()` and `renderSidebar` deriving from the same
  `sidebarGroups()` so order and headers never drift.
- **Keep it simple.** Minimal panels, no collapsible/expandable chrome; prefer the smallest change.
- **Commits:** do not add `Co-Authored-By` lines.

## Verifying UI changes

Tests render the model and assert on `View()`. For a real visual check, run the binary in a pty with a window size set
(it shows `loading…` until it gets dimensions), e.g. drive it with Python's `pty` and `TIOCSWINSZ`. Set
`COLORTERM=truecolor` to see theme colors.

## Releasing

Releases are cut by **pushing a SemVer tag**; everything else is automated.

- **Versioning:** `v`-prefixed SemVer tags (`vMAJOR.MINOR.PATCH`). Still pre-1.0 (`v0.x`), so breaking changes are
  allowed between minors.
- **Pipeline:** `.github/workflows/ci.yml` runs vet/test/build on every push + PR. `.github/workflows/release.yml` runs
  [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`) on any `v*` tag — it cross-builds linux/darwin (amd64 +
  arm64) archives, writes `checksums.txt`, and publishes a GitHub Release with auto-generated notes. No Windows builds.
- **Version stamping:** GoReleaser injects the tag via `-ldflags "-X main.version=…"`; `main.resolveVersion` falls back
  to the module build info for `go install`-from-tag. Check with `lazytilt --version`.

To cut a release:

```sh
task check                              # main must be green and pushed
goreleaser check                        # validate .goreleaser.yaml
goreleaser release --snapshot --clean --skip=publish   # optional local dry-run (builds all archives into dist/)
git tag -a v0.2.0 -m "lazytilt v0.2.0"  # annotated tag
git push origin v0.2.0                  # triggers release.yml
```

Then watch the run under the repo's **Actions** tab (or `gh run watch`). Never reuse or move a published tag — bump to
the next patch instead. `dist/` (GoReleaser output) is git-ignored.
