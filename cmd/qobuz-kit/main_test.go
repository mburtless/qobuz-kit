package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestExecute(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		err        error
		wantCode   int
		wantCalls  int
		wantDry    bool
		wantOutput string
	}{
		{name: "help", args: []string{"--help"}, wantOutput: "--dry-run"},
		{name: "short help", args: []string{"-h"}, wantOutput: "QOBUZ_USER_AUTH_TOKEN"},
		{name: "bad flag", args: []string{"--oops"}, wantCode: 1, wantOutput: "unknown flag"},
		{name: "unexpected argument", args: []string{"123"}, wantCode: 1, wantOutput: "unknown command"},
		{name: "argument after separator", args: []string{"--", "123"}, wantCode: 1},
		{name: "default", wantCalls: 1},
		{name: "dry run", args: []string{"--dry-run"}, wantCalls: 1, wantDry: true},
		{name: "explicit false", args: []string{"--dry-run=false"}, wantCalls: 1},
		{name: "invalid boolean", args: []string{"--dry-run=oops"}, wantCode: 1},
		{name: "force color", args: []string{"--color=always"}, wantCalls: 1},
		{name: "disable color", args: []string{"--color=never"}, wantCalls: 1},
		{name: "invalid color", args: []string{"--color=oops"}, wantCode: 1, wantOutput: "invalid color mode"},
		{name: "failure", err: errors.New("failed"), wantCode: 1, wantCalls: 1, wantOutput: "Error: failed"},
		{name: "interrupted", err: context.Canceled, wantCode: 130, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			calls := 0
			code := execute(context.Background(), tc.args, &output, func(dry bool) error {
				calls++
				if dry != tc.wantDry {
					t.Fatal("unexpected dry-run value")
				}
				return tc.err
			})
			if code != tc.wantCode || calls != tc.wantCalls {
				t.Fatalf("code %d, calls %d; want %d, %d", code, calls, tc.wantCode, tc.wantCalls)
			}
			if !strings.Contains(output.String(), tc.wantOutput) {
				t.Fatalf("missing %q in output %q", tc.wantOutput, output.String())
			}
			if tc.err != nil && strings.Count(output.String(), "Error:") > 1 {
				t.Fatal("error printed more than once")
			}
		})
	}
}

func TestDefaultColorEnabledWithNoColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	var output bytes.Buffer
	code := execute(context.Background(), nil, &output, func(bool) error {
		if rendered := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render("Playlist"); !strings.Contains(rendered, "\x1b[") {
			t.Fatal("default command did not enable colors")
		}
		return nil
	})
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
}
