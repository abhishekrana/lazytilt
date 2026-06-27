// Package discovery finds Tilt instances running locally so lazytilt can switch
// between them. The per-platform process scan lives in the discovery_<goos>.go
// files:
//
//   - Linux:  scans /proc for `tilt up` processes, reading each one's --port /
//     TILT_PORT and working directory.
//   - macOS:  lists processes with `ps` and resolves each match's working
//     directory with `lsof` (there is no /proc).
//   - other:  returns nothing (callers fall back to the -host/-port flags).
package discovery

import (
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Instance is a discovered Tilt instance.
type Instance struct {
	Host     string
	Port     int
	Label    string // human-friendly name (working-dir basename)
	Dir      string // working directory (used to pair a launcher with its tilt)
	PID      int
	Starting bool // a launcher is bringing this stack up; `tilt up` hasn't bound a port yet
}

// Discover returns the set of locally discovered Tilt stacks: running instances
// (a live `tilt up`, with a port) plus stacks that are still starting (a launcher
// such as `task up` whose dir holds a Tiltfile, before `tilt up` execs).
// scanProcesses is platform-specific; mergeInstances is the pure dedup.
func Discover() []Instance {
	return mergeInstances(scanProcesses())
}

// mergeInstances dedups and orders a raw scan: running instances first (by port,
// first-seen-per-port wins), then still-starting stacks (by dir). A launcher is
// dropped once its directory has a live tilt — it has become that instance — and
// duplicate launchers for one directory collapse to one row.
func mergeInstances(scanned []Instance) []Instance {
	seenPort := map[int]bool{}
	var running, starting []Instance
	for _, in := range scanned {
		if in.Starting {
			starting = append(starting, in)
			continue
		}
		if in.Port == 0 || seenPort[in.Port] {
			continue
		}
		seenPort[in.Port] = true
		running = append(running, in)
	}
	sort.Slice(running, func(i, j int) bool { return running[i].Port < running[j].Port })

	runningDirs := map[string]bool{}
	for _, in := range running {
		if in.Dir != "" {
			runningDirs[in.Dir] = true
		}
	}
	seenDir := map[string]bool{}
	var pending []Instance
	for _, in := range starting {
		if in.Dir == "" || runningDirs[in.Dir] || seenDir[in.Dir] {
			continue
		}
		seenDir[in.Dir] = true
		pending = append(pending, in)
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].Dir < pending[j].Dir })

	out := append(running, pending...)
	disambiguate(out)
	return out
}

// disambiguate appends the port to any labels shared by more than one instance,
// so the top-bar switcher never shows two identical names.
func disambiguate(insts []Instance) {
	counts := map[string]int{}
	for _, i := range insts {
		counts[i.Label]++
	}
	for i := range insts {
		if insts[i].Label == "" {
			insts[i].Label = strconv.Itoa(insts[i].Port)
		} else if counts[insts[i].Label] > 1 && insts[i].Port != 0 {
			// Only running instances carry a port to disambiguate with; two
			// still-starting stacks that happen to share a label stay as-is.
			insts[i].Label += ":" + strconv.Itoa(insts[i].Port)
		}
	}
}

// isTiltUp reports whether an argv is a `tilt up` invocation.
func isTiltUp(args []string) bool {
	if len(args) < 2 {
		return false
	}
	if filepath.Base(args[0]) != "tilt" {
		return false
	}
	return slices.Contains(args[1:], "up")
}

// taskRunners are the build/task runners that commonly wrap `tilt up` (e.g.
// `task up`, `make dev`). When one runs in a directory that holds a Tiltfile, we
// treat it as a stack still starting — this catches the whole pre-`tilt up` phase
// (setup tasks, image pulls, builds), not just a step whose name mentions tilt.
var taskRunners = map[string]bool{"task": true, "make": true, "just": true, "mage": true}

// isTiltLauncher reports whether an argv looks like a command bringing a tilt
// stack up, so the overview can surface a stack that's still starting (pulling
// images, building) before `tilt up` execs and binds its port. It matches a known
// task runner (any target — the Tiltfile-in-cwd gate in the scanner scopes it to
// real stack dirs), or any other command that explicitly references tilt
// (e.g. `./tilt.sh`). Deliberately conservative: the tilt/lazytilt binaries
// are excluded (tilt itself is matched by isTiltUp), and a bare Tiltfile path
// argument — an editor or pager opening the file — does not count, so
// `vim Tiltfile` isn't mistaken for a launch.
func isTiltLauncher(args []string) bool {
	if len(args) == 0 {
		return false
	}
	base := filepath.Base(args[0])
	if base == "tilt" || strings.Contains(base, "lazytilt") {
		return false
	}
	if taskRunners[base] {
		return true
	}
	for _, a := range args {
		if filepath.Base(a) == "Tiltfile" {
			continue
		}
		if strings.Contains(strings.ToLower(a), "tilt") {
			return true
		}
	}
	return false
}

// parseTiltProcesses turns `ps -o pid=,args=` output into instances, using cwd to
// resolve each process's working directory (its label) and env to read TILT_PORT
// when the port isn't on the command line. hasTiltfile reports whether a directory
// holds a Tiltfile, gating launcher detection. The I/O is injected so the parser
// is pure and unit-testable on any OS; the macOS scanner supplies the real lookups.
func parseTiltProcesses(psOutput string, cwd func(pid int) string, env func(pid int) int, hasTiltfile func(dir string) bool) []Instance {
	var insts []Instance
	for line := range strings.SplitSeq(psOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, rest, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		args := strings.Fields(rest)
		dir := ""
		if cwd != nil {
			dir = cwd(pid)
		}
		switch {
		case isTiltUp(args):
			port := portFromArgs(args)
			if port == 0 && env != nil {
				port = env(pid)
			}
			if port == 0 {
				port = 10350 // Tilt's default
			}
			insts = append(insts, Instance{Host: "localhost", Port: port, Label: labelFromPath(dir), Dir: dir, PID: pid})
		case isTiltLauncher(args) && dir != "" && hasTiltfile != nil && hasTiltfile(dir):
			insts = append(insts, Instance{Host: "localhost", Label: labelFromPath(dir), Dir: dir, PID: pid, Starting: true})
		}
	}
	return insts
}

// genericDirs are path components that don't usefully name a project, so we skip
// them when deriving a label from the working directory.
var genericDirs = map[string]bool{
	"local": true, "overlays": true, "overlay": true, "base": true, "dc": true,
	"k8s": true, "kubernetes": true, "kustomize": true, "manifests": true,
	"deploy": true, "deployments": true, "env": true, "environments": true,
	"config": true, "dev": true, "development": true, "staging": true, "stage": true,
	"prod": true, "production": true, "default": true, "tilt": true,
}

// labelFromPath picks the deepest path component that actually names something,
// e.g. ".../myapp/k8s/overlays/local" -> "myapp".
func labelFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := parts[i]; p != "" && !genericDirs[p] {
			return p
		}
	}
	return filepath.Base(path)
}

func portFromArgs(args []string) int {
	for i, a := range args {
		switch {
		case a == "--port" || a == "-port":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					return p
				}
			}
		case strings.HasPrefix(a, "--port="):
			if p, err := strconv.Atoi(strings.TrimPrefix(a, "--port=")); err == nil {
				return p
			}
		}
	}
	return 0
}
