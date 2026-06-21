package tilt

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ActionKind is a mutating operation lazytilt can perform on a resource. We
// shell out to the `tilt` CLI scoped by --port rather than reimplementing the
// HUD POST (trigger) and apiserver ConfigMap writes (enable/disable) ourselves.
type ActionKind int

const (
	ActionTrigger ActionKind = iota
	ActionEnable
	ActionDisable
)

func (k ActionKind) String() string {
	switch k {
	case ActionTrigger:
		return "trigger"
	case ActionEnable:
		return "enable"
	case ActionDisable:
		return "disable"
	default:
		return "?"
	}
}

// RunAction invokes `tilt <verb> <resource> --port <port>` and returns any error
// with the CLI's output attached.
func RunAction(kind ActionKind, resource string, port int) error {
	cmd := exec.Command("tilt", kind.String(), resource, "--port", strconv.Itoa(port))
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}
