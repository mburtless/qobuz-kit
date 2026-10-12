package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestLoadingClearsScreen(t *testing.T) {
	failure := errors.New("tracks unavailable")
	for _, tc := range []struct {
		name string
		want error
	}{
		{name: "success"},
		{name: "API error", want: failure},
		{name: "cancellation", want: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			var output bytes.Buffer
			prompts := Prompts{Input: strings.NewReader(""), Output: &output}
			var called atomic.Bool
			err := prompts.Load(ctx, func(ctx context.Context) error {
				called.Store(true)
				if tc.want == context.DeadlineExceeded {
					<-ctx.Done()
					return ctx.Err()
				}
				return tc.want
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error %v, want %v", err, tc.want)
			}
			// Cancellation can return before the worker; only inspect synchronous completion.
			if tc.want != context.DeadlineExceeded && !called.Load() {
				t.Fatal("load was not called")
			}
			clear := ansi.EraseEntireScreen + ansi.CursorHomePosition
			if !strings.HasPrefix(output.String(), clear) || !strings.HasSuffix(output.String(), clear) {
				t.Fatalf("screen not cleared before and after loading: %q", output.String())
			}
		})
	}
}
