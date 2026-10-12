package main

import (
	"github.com/spf13/cobra"
	"qobuz-kit/internal/display"
)

func newRootCommand(run func(bool) error) *cobra.Command {
	var dryRun bool
	var colorMode string
	command := &cobra.Command{
		Use:           "qobuz-kit",
		Short:         "Choose a Qobuz playlist and heart all its tracks",
		Long:          "Choose a Qobuz playlist and heart all its tracks.\nRequires QOBUZ_APP_ID and QOBUZ_USER_AUTH_TOKEN.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := display.Configure(command.ErrOrStderr(), colorMode); err != nil {
				return err
			}
			return run(dryRun)
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "preview missing hearts without changing your account")
	command.Flags().StringVar(&colorMode, "color", "always", "color output: auto, always, or never")
	return command
}
