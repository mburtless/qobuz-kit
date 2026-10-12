package tui

import (
	"context"
	"errors"
	"io"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/ansi"
	"qobuz-kit/internal/heart"
)

type loadFinished struct{ err error }

type loadingModel struct {
	spinner spinner.Model
	work    tea.Cmd
	err     error
}

func (m loadingModel) Init() tea.Cmd { return tea.Batch(m.spinner.Tick, m.work) }

func (m loadingModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case loadFinished:
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			m.err = heart.ErrQuit
			return m, tea.Quit
		case "ctrl+c":
			m.err = context.Canceled
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(message)
	return m, cmd
}

func (m loadingModel) View() string {
	return m.spinner.View() + " Loading playlist tracks and existing hearts...\n"
}

func (p Prompts) Load(ctx context.Context, work func(context.Context) error) (err error) {
	loadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	clear := func() error {
		_, err := io.WriteString(p.Output, ansi.EraseEntireScreen+ansi.CursorHomePosition)
		return err
	}
	if err := clear(); err != nil {
		return err
	}
	// Clear the final spinner frame before the plan and confirmation are rendered.
	defer func() { err = errors.Join(err, clear()) }()
	model := loadingModel{
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(huh.ThemeBase16().Focused.SelectSelector)),
		work:    func() tea.Msg { return loadFinished{err: work(loadCtx)} },
	}
	result, err := tea.NewProgram(model, tea.WithContext(loadCtx), tea.WithInput(p.Input), tea.WithOutput(p.Output)).Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return result.(loadingModel).err
}
