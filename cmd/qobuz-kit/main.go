package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/charmbracelet/x/term"
	"qobuz-kit/internal/display"
	"qobuz-kit/internal/qobuz"
	"qobuz-kit/internal/tui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := execute(ctx, os.Args[1:], os.Stderr, func(dryRun bool) error {
		appID := strings.TrimSpace(os.Getenv("QOBUZ_APP_ID"))
		token := strings.TrimSpace(os.Getenv("QOBUZ_USER_AUTH_TOKEN"))
		if appID == "" || token == "" {
			return errors.New("set QOBUZ_APP_ID and QOBUZ_USER_AUTH_TOKEN from the same Qobuz web session; see README.md")
		}
		if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stderr.Fd()) {
			return errors.New("an interactive terminal on stdin and stderr is required for playlist selection")
		}
		return tui.Run(ctx, qobuz.New(appID, token), os.Stdin, os.Stderr, dryRun)
	})
	cancel()
	os.Exit(code)
}

func execute(ctx context.Context, args []string, output io.Writer, run func(bool) error) int {
	command := newRootCommand(run)
	command.SetArgs(args)
	command.SetOut(output)
	command.SetErr(output)
	err := command.ExecuteContext(ctx)
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		fmt.Fprintln(output, display.Warning.Render("Interrupted. Rerun to check and add any remaining hearts."))
		return 130
	}
	if err != nil {
		fmt.Fprintln(output, display.Error.Render("Error: "+err.Error()))
		return 1
	}
	return 0
}
