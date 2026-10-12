package display

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const Accent = lipgloss.Color("13")

var (
	Title   = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	Info    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	Error   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
)

func Configure(output io.Writer, mode string) error {
	terminal := termenv.NewOutput(output)
	var profile termenv.Profile
	switch mode {
	case "auto":
		profile = terminal.EnvColorProfile()
	case "always":
		profile = termenv.ANSI
	case "never":
		profile = termenv.Ascii
	default:
		return fmt.Errorf("invalid color mode %q: use auto, always, or never", mode)
	}
	// Existing styles share this renderer; update it rather than replacing it.
	lipgloss.DefaultRenderer().SetOutput(terminal)
	lipgloss.SetColorProfile(profile)
	return nil
}
