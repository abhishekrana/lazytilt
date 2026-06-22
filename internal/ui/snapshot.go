package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	tea "github.com/charmbracelet/bubbletea"
)

// snapshotPath is the temp-file path a snapshot is written to: the instance label
// plus a YYYYMMDD-HHMMSS stamp, e.g. /tmp/lazytilt-snapshot-app-one-20260622-153045.json.
func snapshotPath(label string) string {
	stamp := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("lazytilt-snapshot-%s-%s.json", sanitizeFilename(label), stamp)
	return filepath.Join(os.TempDir(), name)
}

// snapshotCmd creates a snapshot of the host:port instance via the tilt CLI and
// reports the file path (or the error) in the footer.
func snapshotCmd(host string, port int, label string) tea.Cmd {
	return func() tea.Msg {
		path := snapshotPath(label)
		if err := tilt.CreateSnapshot(host, port, path); err != nil {
			return notifyMsg{text: "snapshot failed: " + err.Error(), err: true}
		}
		return notifyMsg{text: "snapshot → " + path + "  (tilt snapshot view to open)"}
	}
}
