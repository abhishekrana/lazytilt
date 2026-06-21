package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/abhishekrana/lazytilt/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	host := flag.String("host", "localhost", "fallback Tilt host (used if discovery finds nothing)")
	port := flag.Int("port", 10350, "fallback Tilt port (used if discovery finds nothing)")
	theme := flag.String("theme", "", "color theme: solarized-light (default), solarized-dark, dark")
	flag.Parse()

	token, _ := tilt.ReadToken()

	p := tea.NewProgram(ui.New(token, *host, *port, *theme), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazytilt:", err)
		os.Exit(1)
	}
}
