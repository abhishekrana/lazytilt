//go:build darwin

package discovery

import (
	"os/exec"
	"strconv"
	"strings"
)

// scanProcesses finds `tilt up` processes on macOS, which has no /proc: it lists
// processes with `ps`, then resolves each match's working directory with `lsof`.
func scanProcesses() []Instance {
	out, err := exec.Command("ps", "-axww", "-o", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	return parseTiltProcesses(string(out), cwdOf, portFromEnv)
}

// cwdOf returns a process's working directory via `lsof`, or "" if unavailable.
// `lsof -Fn` prints the path on a line prefixed with "n".
func cwdOf(pid int) string {
	out, err := exec.Command("lsof", "-a", "-d", "cwd", "-Fn", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if path, ok := strings.CutPrefix(line, "n"); ok {
			return path
		}
	}
	return ""
}

// portFromEnv tries to read TILT_PORT from the process environment via `ps -E`.
// macOS only exposes the environment for the caller's own processes (and only on
// some OS versions), so this is best-effort: on failure it returns 0 and the
// caller falls back to the port flag or Tilt's default.
func portFromEnv(pid int) int {
	out, err := exec.Command("ps", "-E", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	for field := range strings.FieldsSeq(string(out)) {
		if v, ok := strings.CutPrefix(field, "TILT_PORT="); ok {
			if p, err := strconv.Atoi(v); err == nil {
				return p
			}
		}
	}
	return 0
}
