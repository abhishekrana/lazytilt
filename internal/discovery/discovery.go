// Package discovery finds Tilt instances running locally so lazytilt can switch
// between them. On Linux it scans /proc for `tilt up` processes and reads each
// one's --port / TILT_PORT and working directory. On other platforms it returns
// nothing (callers fall back to the -host/-port flags).
package discovery

import (
	"os"
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

// Discover returns the set of locally running Tilt instances, sorted by port.
func Discover() []Instance {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	seen := map[int]bool{}
	var out []Instance
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		inst, ok := inspect(pid)
		if !ok || seen[inst.Port] {
			continue
		}
		seen[inst.Port] = true
		out = append(out, inst)
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

// inspect returns an Instance if pid is a `tilt up` process.
func inspect(pid int) (Instance, bool) {
	args, err := readArgs(pid)
	if err != nil || len(args) < 2 {
		return Instance{}, false
	}
	if filepath.Base(args[0]) != "tilt" {
		return Instance{}, false
	}
	if !slices.Contains(args[1:], "up") {
		return Instance{}, false
	}

	port := portFromArgs(args)
	if port == 0 {
		port = portFromEnv(pid)
	}
	if port == 0 {
		port = 10350 // Tilt's default
	}

	label := ""
	if cwd, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd"); err == nil {
		label = labelFromPath(cwd)
	}

	return Instance{Host: "localhost", Port: port, Label: label, PID: pid}, true
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

func portFromEnv(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/environ")
	if err != nil {
		return 0
	}
	for kv := range strings.SplitSeq(string(b), "\x00") {
		if v, ok := strings.CutPrefix(kv, "TILT_PORT="); ok {
			if p, err := strconv.Atoi(v); err == nil {
				return p
			}
		}
	}
	return 0
}

func readArgs(pid int) ([]string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	return parts, nil
}
