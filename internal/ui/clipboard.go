package ui

import (
	"os"
	"os/exec"
	"strings"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
)

// openURLCmd opens url in the user's default browser via xdg-open (Linux). We
// Start (not Run) so the TUI never blocks on the spawned process. xdg-open hands
// off to the desktop, so it works the same inside and outside tmux.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if err := exec.Command("xdg-open", url).Start(); err != nil {
			return notifyMsg{text: "open failed: " + err.Error(), err: true}
		}
		return notifyMsg{text: "opened " + url}
	}
}

// clipboardTools are the Linux clipboard helpers we try in order: Wayland first,
// then the two common X11 ones. They talk to the display server directly, so —
// unlike an OSC 52 terminal escape — they are unaffected by tmux and behave
// identically whether or not lazytilt runs inside a multiplexer.
var clipboardTools = [][]string{
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
	{"xsel", "--clipboard", "--input"},
}

// copyURLCmd copies text to the clipboard. It prefers a system clipboard tool
// (tmux-transparent) and falls back to OSC 52 — wrapped for tmux/screen — for
// hosts without one, e.g. over SSH.
func copyURLCmd(text string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range clipboardTools {
			if _, err := exec.LookPath(c[0]); err != nil {
				continue
			}
			cmd := exec.Command(c[0], c[1:]...)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return notifyMsg{text: "copied " + text}
			}
		}
		if osc52Copy(text) {
			return notifyMsg{text: "copied " + text + " (osc52)"}
		}
		return notifyMsg{text: "copy failed: install wl-copy, xclip or xsel", err: true}
	}
}

// osc52Copy writes an OSC 52 clipboard sequence to the controlling terminal,
// wrapped for tmux (which needs `allow-passthrough on`) or GNU screen when
// detected. It writes to /dev/tty rather than stdout so it stays clear of Bubble
// Tea's renderer. Returns false if the terminal is unavailable.
func osc52Copy(text string) bool {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	defer tty.Close()

	seq := osc52.New(text)
	switch {
	case os.Getenv("TMUX") != "":
		seq = seq.Tmux()
	case strings.HasPrefix(os.Getenv("TERM"), "screen"):
		seq = seq.Screen()
	}
	_, err = seq.WriteTo(tty)
	return err == nil
}
