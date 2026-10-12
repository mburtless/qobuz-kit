package tui

import (
	"context"
	"errors"
	"io"

	"github.com/charmbracelet/x/ansi"
	"qobuz-kit/internal/heart"
)

func Run(ctx context.Context, api heart.API, input io.Reader, output io.Writer, dryRun bool) error {
	return withScreen(output, func() error {
		prompts := Prompts{Input: input, Output: output}
		return heart.Run(ctx, api, prompts, output, dryRun)
	})
}

func withScreen(output io.Writer, run func() error) (err error) {
	// Own the screen for the whole session, including time between Huh forms.
	defer func() {
		_, restoreErr := io.WriteString(output, ansi.ResetAltScreenSaveCursorMode)
		err = errors.Join(err, restoreErr)
	}()
	if _, err = io.WriteString(output, ansi.SetAltScreenSaveCursorMode); err != nil {
		return err
	}
	return run()
}
