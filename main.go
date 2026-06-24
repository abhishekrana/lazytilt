package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/abhishekrana/lazytilt/internal/tilt"
	"github.com/abhishekrana/lazytilt/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// version is the lazytilt build version. It is "dev" for local builds and is
// overwritten at release time via -ldflags "-X main.version=...". When unset (a
// plain `go install` from a tag), resolveVersion falls back to the module
// version embedded in the build info.
var version = "dev"

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}

func main() {
	host := flag.String("host", "localhost", "fallback Tilt host (used if discovery finds nothing)")
	port := flag.Int("port", 10350, "fallback Tilt port (used if discovery finds nothing)")
	theme := flag.String("theme", "", "color theme: solarized-light (default), solarized-dark, dark")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("lazytilt", resolveVersion())
		return
	}

	token, _ := tilt.ReadToken()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The model owns the event channel; hand it to the hub, which discovers
	// instances and streams their /ws/view deltas onto it.
	m := ui.New(token, *host, *port, *theme)
	hub := ui.NewHub(token, *host, *port, m.Events())
	go hub.Run(ctx)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazytilt:", err)
		os.Exit(1)
	}
}
