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
	Host  string
	Port  int
	Label string // human-friendly name (working-dir basename)
	PID   int
}

// Discover returns the set of locally running Tilt instances, deduplicated by
// port (first seen wins) and sorted by port. scanProcesses is platform-specific.
func Discover() []Instance {
	seen := map[int]bool{}
	var out []Instance
	for _, in := range scanProcesses() {
		if in.Port == 0 || seen[in.Port] {
			continue
		}
		seen[in.Port] = true
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
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
		} else if counts[insts[i].Label] > 1 {
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

// parseTiltProcesses turns `ps -o pid=,args=` output into instances, using cwd to
// resolve each process's working-directory label and env to read TILT_PORT when
// the port isn't on the command line. The I/O is injected so the parser is pure
// and unit-testable on any OS; the macOS scanner supplies the real lookups.
func parseTiltProcesses(psOutput string, cwd func(pid int) string, env func(pid int) int) []Instance {
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
		if !isTiltUp(args) {
			continue
		}
		port := portFromArgs(args)
		if port == 0 && env != nil {
			port = env(pid)
		}
		if port == 0 {
			port = 10350 // Tilt's default
		}
		label := ""
		if cwd != nil {
			label = labelFromPath(cwd(pid))
		}
		insts = append(insts, Instance{Host: "localhost", Port: port, Label: label, PID: pid})
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
