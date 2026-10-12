package display

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestColorModes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	renderer := lipgloss.DefaultRenderer()
	previousOutput, previousProfile := renderer.Output(), renderer.ColorProfile()
	t.Cleanup(func() { renderer.SetOutput(previousOutput); renderer.SetColorProfile(previousProfile) })
	for _, tc := range []struct {
		mode      string
		wantColor bool
		wantErr   bool
	}{
		{mode: "auto"},
		{mode: "always", wantColor: true},
		{mode: "never"},
		{mode: "invalid", wantErr: true},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var output bytes.Buffer
			err := Configure(&output, tc.mode)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.wantErr {
				return
			}
			for _, style := range []lipgloss.Style{Title, Info, Success, Warning, Error} {
				text := style.Render("A message")
				if strings.Contains(text, "\x1b[") != tc.wantColor {
					t.Fatalf("mode %s rendered %q", tc.mode, text)
				}
			}
		})
	}
}
