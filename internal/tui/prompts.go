package tui

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"qobuz-kit/internal/heart"
	"qobuz-kit/internal/qobuz"
)

type Prompts struct {
	Input  io.Reader
	Output io.Writer
}

func (p Prompts) Select(ctx context.Context, playlists []qobuz.Playlist) (qobuz.Playlist, error) {
	model := newPlaylistPicker(playlists)
	result, err := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(p.Input), tea.WithOutput(p.Output)).Run()
	if ctx.Err() != nil {
		return qobuz.Playlist{}, ctx.Err()
	}
	if err != nil {
		return qobuz.Playlist{}, err
	}
	picker := result.(playlistPicker)
	if picker.err != nil {
		return qobuz.Playlist{}, picker.err
	}
	return playlists[picker.choice], nil
}

func (p Prompts) Confirm(ctx context.Context, count int) (bool, error) {
	confirmed := false
	form := huh.NewForm(huh.NewGroup(huh.NewConfirm().
		Title(fmt.Sprintf("Add %d new hearts to your Qobuz account?", count)).
		Description("Press q to quit.").
		Affirmative("Yes").Negative("No").Value(&confirmed)))
	if err := p.runForm(ctx, form, nil); err != nil {
		return false, err
	}
	return confirmed, nil
}

func (p Prompts) runForm(ctx context.Context, form *huh.Form, resize func(tea.WindowSizeMsg)) error {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "q")
	form.WithKeyMap(keyMap)
	form.WithTheme(huh.ThemeBase16())
	quit := false
	form.WithProgramOptions(tea.WithFilter(func(_ tea.Model, message tea.Msg) tea.Msg {
		if key, ok := message.(tea.KeyMsg); ok && key.String() == "q" {
			quit = true
		}
		if size, ok := message.(tea.WindowSizeMsg); ok && resize != nil {
			resize(size)
		}
		return message
	})).WithInput(p.Input).WithOutput(p.Output)
	if err := form.RunWithContext(ctx); err != nil {
		if quit && ctx.Err() == nil {
			return heart.ErrQuit
		}
		return promptError(ctx, err)
	}
	return nil
}

func promptError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return context.Canceled
	}
	return err
}
