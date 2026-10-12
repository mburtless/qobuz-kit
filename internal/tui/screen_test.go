package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestScreenRestored(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "quit normally"},
		{name: "API failure", err: errors.New("authentication failed")},
		{name: "interrupted", err: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			err := withScreen(&output, func() error {
				fmt.Fprint(&output, "Loading playlists...\nAdded: 2\n")
				return tc.err
			})
			if !errors.Is(err, tc.err) {
				t.Fatalf("error %v, want %v", err, tc.err)
			}
			want := ansi.SetAltScreenSaveCursorMode + "Loading playlists...\nAdded: 2\n" + ansi.ResetAltScreenSaveCursorMode
			if output.String() != want {
				t.Fatalf("screen lifecycle %q, want %q", output.String(), want)
			}
		})
	}
}

func TestScreenRestoredAfterPanic(t *testing.T) {
	var output bytes.Buffer
	defer func() {
		if recover() == nil {
			t.Error("panic was swallowed")
		}
		if !strings.HasSuffix(output.String(), ansi.ResetAltScreenSaveCursorMode) {
			t.Error("screen was not restored")
		}
	}()
	withScreen(&output, func() error { panic("unexpected error") })
}
