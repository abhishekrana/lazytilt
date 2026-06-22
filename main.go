package main

import (
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

	p := tea.NewProgram(ui.New(token, *host, *port, *theme), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lazytilt:", err)
		os.Exit(1)
	}
}
