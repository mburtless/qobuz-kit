package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"qobuz-kit/internal/heart"
	"qobuz-kit/internal/qobuz"
)

type playlistPicker struct {
	labels                     []string
	matches                    []int
	query                      string
	cursor, top, height, width int
	choice                     int
	err                        error
}

func newPlaylistPicker(playlists []qobuz.Playlist) playlistPicker {
	p := playlistPicker{height: 8, width: 80}
	for _, playlist := range playlists {
		p.labels = append(p.labels, fmt.Sprintf("%s — %s — %d tracks (#%d)", heart.Display(playlist.Name), heart.Display(playlist.Owner.Name), playlist.TracksCount, playlist.ID))
	}
	p.labels = append(p.labels, "Quit")
	p.filter()
	return p
}

func (p *playlistPicker) filter() {
	p.matches = nil
	for i, label := range p.labels {
		if strings.Contains(strings.ToLower(label), strings.ToLower(p.query)) {
			p.matches = append(p.matches, i)
		}
	}
	p.cursor, p.top = 0, 0
}

func (p playlistPicker) Init() tea.Cmd { return nil }

func (p playlistPicker) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		p.height, p.width = max(1, msg.Height-6), max(1, msg.Width)
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			p.err = heart.ErrQuit
			return p, tea.Quit
		case "ctrl+c":
			p.err = context.Canceled
			return p, tea.Quit
		case "up":
			p.cursor = max(0, p.cursor-1)
		case "down":
			p.cursor = min(max(0, len(p.matches)-1), p.cursor+1)
		case "enter":
			if len(p.matches) > 0 {
				p.choice = p.matches[p.cursor]
				if p.choice == len(p.labels)-1 {
					p.err = heart.ErrQuit
				}
				return p, tea.Quit
			}
		case "esc", "ctrl+u":
			p.query = ""
			p.filter()
		case "backspace":
			runes := []rune(p.query)
			if len(runes) > 0 {
				p.query = string(runes[:len(runes)-1])
				p.filter()
			}
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				p.query += string(msg.Runes)
				p.filter()
			}
		}
	}
	p.top = min(p.top, max(0, len(p.matches)-p.height))
	if p.cursor < p.top {
		p.top = p.cursor
	}
	if p.cursor >= p.top+p.height {
		p.top = p.cursor - p.height + 1
	}
	return p, nil
}

func (p playlistPicker) View() string {
	theme := huh.ThemeBase16().Focused
	var view strings.Builder
	view.WriteString(theme.Title.Render("Choose a playlist") + "\n")
	view.WriteString(theme.Description.Render("Type to search. ↑/↓ choose; Enter select; q quit.") + "\n")
	view.WriteString(theme.TextInput.Prompt.Render("Search: ") + theme.TextInput.Text.Render(p.query) + "\n\n")
	for row := p.top; row < min(len(p.matches), p.top+p.height); row++ {
		label := p.labels[p.matches[row]]
		if row == p.cursor {
			view.WriteString(lipgloss.NewStyle().MaxWidth(p.width).Render(theme.SelectSelector.Render("❯ ") + theme.SelectedOption.Render(label)))
		} else {
			view.WriteString(theme.UnselectedOption.Foreground(lipgloss.NoColor{}).MaxWidth(p.width).Render("  " + label))
		}
		view.WriteByte('\n')
	}
	if len(p.matches) == 0 {
		view.WriteString(theme.ErrorMessage.Render("No matching playlist. Esc clears search.") + "\n")
	}
	return view.String()
}
