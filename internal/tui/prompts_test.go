package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"qobuz-kit/internal/heart"
	"qobuz-kit/internal/qobuz"
)

func TestPlaylistSearch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		search  string
		up      bool
		want    int
		wantErr bool
	}{
		{name: "matching playlist", search: "Night", want: 1},
		{name: "case insensitive", search: "night", want: 1},
		{name: "quit", search: "Quit", want: -1},
		{name: "no match after moving up", search: "Missing", up: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			picker := newPlaylistPicker([]qobuz.Playlist{{ID: 1, Name: "Jazz"}, {ID: 2, Name: "Night Music"}})
			model, _ := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.search)})
			if tc.up {
				model, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			}
			model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := model.(playlistPicker)
			if tc.wantErr {
				if cmd != nil {
					t.Fatal("accepted a search with no matches")
				}
			} else if tc.want == -1 {
				if !errors.Is(got.err, heart.ErrQuit) {
					t.Fatalf("error %v", got.err)
				}
			} else if got.choice != tc.want || cmd == nil {
				t.Fatalf("selected %d, want %d", got.choice, tc.want)
			}
		})
	}
}

func TestQuitKeys(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  error
	}{
		{name: "q", input: "q", want: heart.ErrQuit},
		{name: "ctrl+c", input: "\x03", want: context.Canceled},
	} {
		for _, prompt := range []string{"selection", "confirmation"} {
			t.Run(tc.name+"/"+prompt, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				p := Prompts{Input: strings.NewReader(tc.input), Output: io.Discard}
				var err error
				if prompt == "selection" {
					_, err = p.Select(ctx, []qobuz.Playlist{{ID: 1, Name: "Jazz"}})
				} else {
					_, err = p.Confirm(ctx, 2)
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("error %v, want %v", err, tc.want)
				}
			})
		}
	}
}

func TestPlaylistScrolling(t *testing.T) {
	picker := newPlaylistPicker(make([]qobuz.Playlist, 10))
	picker.height = 3
	for _, tc := range []struct {
		key         tea.KeyType
		cursor, top int
	}{
		{tea.KeyDown, 1, 0}, {tea.KeyDown, 2, 0}, {tea.KeyDown, 3, 1},
		{tea.KeyDown, 4, 2}, {tea.KeyUp, 3, 2}, {tea.KeyUp, 2, 2}, {tea.KeyUp, 1, 1},
	} {
		model, _ := picker.Update(tea.KeyMsg{Type: tc.key})
		picker = model.(playlistPicker)
		if picker.cursor != tc.cursor || picker.top != tc.top {
			t.Fatalf("cursor/top %d/%d, want %d/%d", picker.cursor, picker.top, tc.cursor, tc.top)
		}
	}
}
