//go:build linux

package discovery

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// scanProcesses scans /proc for `tilt up` processes.
func scanProcesses() []Instance {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []Instance
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if inst, ok := inspect(pid); ok {
			out = append(out, inst)
		}
	}
	return out
}

// inspect returns an Instance if pid is a live `tilt up` or a launcher bringing a
// tilt stack up (a launcher only counts when its dir holds a Tiltfile).
func inspect(pid int) (Instance, bool) {
	args, err := readArgs(pid)
	if err != nil {
		return Instance{}, false
	}
	cwd, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")

	if isTiltUp(args) {
		port := portFromArgs(args)
		if port == 0 {
			port = portFromEnv(pid)
		}
		if port == 0 {
			port = 10350 // Tilt's default
		}
		return Instance{Host: "localhost", Port: port, Label: labelFromPath(cwd), Dir: cwd, PID: pid}, true
	}
	if isTiltLauncher(args) && cwd != "" && hasTiltfile(cwd) {
		return Instance{Host: "localhost", Label: labelFromPath(cwd), Dir: cwd, PID: pid, Starting: true}, true
	}
	return Instance{}, false
}

// hasTiltfile reports whether dir contains a regular Tiltfile.
func hasTiltfile(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "Tiltfile"))
	return err == nil && !fi.IsDir()
}

// portFromEnv reads TILT_PORT from /proc/<pid>/environ.
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

// readArgs reads a process's argv from /proc/<pid>/cmdline.
func readArgs(pid int) ([]string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
	return parts, nil
}
