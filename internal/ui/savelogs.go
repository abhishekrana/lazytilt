package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// writeLogsTempFile writes text to a timestamped temp file and returns its path.
// The name embeds the resource and a YYYYMMDD-HHMMSS stamp, plus a random suffix
// for uniqueness, e.g. /tmp/lazytilt-api-20260622-153045-9921.log.
func writeLogsTempFile(name, text string) (string, error) {
	stamp := time.Now().Format("20060102-150405")
	pattern := fmt.Sprintf("lazytilt-%s-%s-*.log", sanitizeFilename(name), stamp)
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// saveLogsCmd writes a resource's logs to a timestamped temp file and reports
// the path.
func saveLogsCmd(name, text string) tea.Cmd {
	return func() tea.Msg {
		path, err := writeLogsTempFile(name, text)
		if err != nil {
			return notifyMsg{text: "save failed: " + err.Error(), err: true}
		}
		return notifyMsg{text: "saved logs → " + path}
	}
}

// openLogsCmd writes a resource's logs to a timestamped temp file and opens it in
// the user's editor ($EDITOR, else vim). tea.ExecProcess hands the terminal to
// the editor and restores the TUI when it exits.
func openLogsCmd(name, text string) tea.Cmd {
	path, err := writeLogsTempFile(name, text)
	if err != nil {
		return func() tea.Msg { return notifyMsg{text: "save failed: " + err.Error(), err: true} }
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	return tea.ExecProcess(exec.Command(editor, path), func(err error) tea.Msg {
		if err != nil {
			return notifyMsg{text: "editor: " + err.Error(), err: true}
		}
		return notifyMsg{text: "logs at " + path}
	})
}

// sanitizeFilename reduces a resource name (which may contain spaces, slashes or
// parentheses, e.g. "(Tiltfile)") to a filesystem-safe slug.
func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if out := strings.Trim(b.String(), "-"); out != "" {
		return out
	}
	return "logs"
}
