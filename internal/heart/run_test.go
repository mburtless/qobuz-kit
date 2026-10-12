package heart

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"qobuz-kit/internal/qobuz"
)

// These stubs are the network and interactive-input boundaries.
type account struct {
	tracks           []qobuz.Track
	favorites        []int64
	failures         map[int64]error
	writes           []int64
	noPlaylists      bool
	readErr          error
	cancel           context.CancelFunc
	tracksByPlaylist map[int64][]qobuz.Track
	favoriteReads    int
}

func (a *account) Playlists(context.Context) ([]qobuz.Playlist, error) {
	if a.noPlaylists {
		return nil, nil
	}
	p := qobuz.Playlist{ID: 123, Name: "Jazz"}
	if a.tracksByPlaylist != nil {
		return []qobuz.Playlist{p, {ID: 456, Name: "Night Music"}}, nil
	}
	return []qobuz.Playlist{p}, nil
}
func (a *account) Tracks(_ context.Context, id int64) ([]qobuz.Track, error) {
	if a.tracksByPlaylist != nil {
		return a.tracksByPlaylist[id], a.readErr
	}
	return a.tracks, a.readErr
}
func (a *account) FavoriteIDs(context.Context) ([]int64, error) {
	a.favoriteReads++
	return a.favorites, nil
}
func (a *account) Heart(_ context.Context, id int64) error {
	a.writes = append(a.writes, id)
	if a.cancel != nil {
		a.cancel()
		return context.Canceled
	}
	if err := a.failures[id]; err != nil {
		return err
	}
	a.favorites = append(a.favorites, id)
	return nil
}

type prompts struct {
	confirmed     bool
	confirmations int
	selections    int
	err           error
	choices       []int
	confirmErr    error
}

func (p *prompts) Select(_ context.Context, playlists []qobuz.Playlist) (qobuz.Playlist, error) {
	p.selections++
	if p.err == nil {
		if p.choices != nil {
			if p.selections > len(p.choices) {
				return qobuz.Playlist{}, ErrQuit
			}
			return playlists[p.choices[p.selections-1]], nil
		}
		if p.selections > 1 {
			return qobuz.Playlist{}, ErrQuit
		}
	}
	return playlists[0], p.err
}
func (p *prompts) Confirm(context.Context, int) (bool, error) {
	p.confirmations++
	if p.confirmErr != nil {
		return false, p.confirmErr
	}
	return p.confirmed, p.err
}

func TestQuitAtConfirmationDoesNotWrite(t *testing.T) {
	a := &account{tracks: []qobuz.Track{{ID: 1}}}
	p := &prompts{confirmErr: ErrQuit}
	var output bytes.Buffer
	if err := Run(context.Background(), a, p, &output, false); err != nil {
		t.Fatal(err)
	}
	if len(a.writes) != 0 || p.confirmations != 1 || p.selections != 1 {
		t.Fatal("quit did not end the session before writing")
	}
}

func TestRun(t *testing.T) {
	unavailable := false
	for _, tc := range []struct {
		name              string
		dryRun            bool
		confirmed         bool
		tracks            []qobuz.Track
		favorites         []int64
		failures          map[int64]error
		readErr           error
		wantWrites        []int64
		wantConfirmations int
		wantErr           bool
		wantOutput        string
	}{
		{name: "unavailable only", tracks: []qobuz.Track{{ID: 1, Streamable: &unavailable}}, wantOutput: "1 unavailable"},
		{name: "mixed availability", confirmed: true, tracks: []qobuz.Track{{ID: 1, Streamable: &unavailable}, {ID: 2}}, wantWrites: []int64{2}, wantConfirmations: 1, wantOutput: "Unavailable: 1"},
		{name: "dry run", dryRun: true, tracks: []qobuz.Track{{ID: 1}}, wantOutput: "1 to add"},
		{name: "declined", tracks: []qobuz.Track{{ID: 1}}, wantConfirmations: 1, wantOutput: "no hearts added"},
		{name: "empty", wantOutput: "0 to add"},
		{name: "already hearted", tracks: []qobuz.Track{{ID: 1}}, favorites: []int64{1}, wantOutput: "1 already hearted"},
		{name: "add missing once", confirmed: true, tracks: []qobuz.Track{{ID: 1}, {ID: 2}, {ID: 2}, {ID: 3}}, favorites: []int64{1}, wantWrites: []int64{2, 3}, wantConfirmations: 1, wantOutput: "Added: 2"},
		{name: "continue rejected track", confirmed: true, tracks: []qobuz.Track{{ID: 1}, {ID: 2}}, failures: map[int64]error{1: &qobuz.APIError{Status: 400}}, wantWrites: []int64{1, 2}, wantConfirmations: 1, wantErr: true, wantOutput: "Failed: 1"},
		{name: "stop expired token", confirmed: true, tracks: []qobuz.Track{{ID: 1}, {ID: 2}}, failures: map[int64]error{1: &qobuz.APIError{Status: 401}}, wantWrites: []int64{1}, wantConfirmations: 1, wantErr: true, wantOutput: "Unprocessed: 1"},
		{name: "stop exhausted rate limit", confirmed: true, tracks: []qobuz.Track{{ID: 1}, {ID: 2}}, failures: map[int64]error{1: &qobuz.APIError{Status: 429}}, wantWrites: []int64{1}, wantConfirmations: 1, wantErr: true, wantOutput: "Unprocessed: 1"},
		{name: "invalid entry", tracks: []qobuz.Track{{Title: "Gone"}}, wantErr: true, wantOutput: "Invalid entry 1: Gone"},
		{name: "read failed before confirmation", tracks: []qobuz.Track{{ID: 1}}, readErr: errors.New("playlist inaccessible"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &account{tracks: tc.tracks, favorites: tc.favorites, failures: tc.failures, readErr: tc.readErr}
			p := &prompts{confirmed: tc.confirmed}
			var output bytes.Buffer
			err := Run(context.Background(), a, p, &output, tc.dryRun)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error %v", err)
			}
			if !reflect.DeepEqual(a.writes, tc.wantWrites) {
				t.Fatalf("writes %v, want %v", a.writes, tc.wantWrites)
			}
			if p.confirmations != tc.wantConfirmations {
				t.Fatalf("confirmation count %d, want %d", p.confirmations, tc.wantConfirmations)
			}
			if !strings.Contains(output.String(), tc.wantOutput) {
				t.Fatalf("missing %q in %s", tc.wantOutput, output.String())
			}
		})
	}
}

func TestRerunSkipsPreviouslyAddedHearts(t *testing.T) {
	a := &account{tracks: []qobuz.Track{{ID: 1}, {ID: 2}}}
	p := &prompts{confirmed: true}
	var output bytes.Buffer
	for range 2 {
		p.selections = 0
		if err := Run(context.Background(), a, p, &output, false); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(a.writes, []int64{1, 2}) || p.confirmations != 1 {
		t.Fatalf("rerun wrote again: %v", a.writes)
	}
}

func TestSelectAnotherPlaylistRefreshesFavorites(t *testing.T) {
	a := &account{tracksByPlaylist: map[int64][]qobuz.Track{
		123: {{ID: 1}, {ID: 2}},
		456: {{ID: 2}, {ID: 3}},
	}}
	p := &prompts{confirmed: true, choices: []int{0, 1}}
	var output bytes.Buffer
	if err := Run(context.Background(), a, p, &output, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.writes, []int64{1, 2, 3}) {
		t.Fatalf("unexpected writes: %v", a.writes)
	}
	if a.favoriteReads != 2 || p.confirmations != 2 || p.selections != 3 {
		t.Fatalf("favorites reads %d, confirmations %d, selections %d", a.favoriteReads, p.confirmations, p.selections)
	}
}

func TestReturnToSelection(t *testing.T) {
	for _, tc := range []struct {
		name           string
		dryRun         bool
		confirmed      bool
		tracks         []qobuz.Track
		failures       map[int64]error
		wantSelections int
		wantErr        error
	}{
		{name: "added hearts", confirmed: true, tracks: []qobuz.Track{{ID: 1}}, wantSelections: 2},
		{name: "declined", tracks: []qobuz.Track{{ID: 1}}, wantSelections: 2},
		{name: "empty playlist", wantSelections: 2},
		{name: "partial failure", confirmed: true, tracks: []qobuz.Track{{ID: 1}}, failures: map[int64]error{1: errors.New("rejected")}, wantSelections: 2, wantErr: ErrIncomplete},
		{name: "dry run remains single playlist", dryRun: true, tracks: []qobuz.Track{{ID: 1}}, wantSelections: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &account{tracks: tc.tracks, failures: tc.failures}
			p := &prompts{confirmed: tc.confirmed}
			var output bytes.Buffer
			err := Run(context.Background(), a, p, &output, tc.dryRun)
			if !errors.Is(err, tc.wantErr) || p.selections != tc.wantSelections {
				t.Fatalf("error %v, selections %d", err, p.selections)
			}
		})
	}
}

func TestInterruptReportsUnprocessed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &account{tracks: []qobuz.Track{{ID: 1}, {ID: 2}}, cancel: cancel}
	var output bytes.Buffer
	err := Run(ctx, a, &prompts{confirmed: true}, &output, false)
	if !errors.Is(err, context.Canceled) || !strings.Contains(output.String(), "Unprocessed: 2") {
		t.Fatalf("error %v, output %s", err, output.String())
	}
}

func TestNoPlaylistsAndAbortedPicker(t *testing.T) {
	for _, tc := range []struct {
		name  string
		empty bool
		err   error
	}{
		{name: "no playlists", empty: true}, {name: "picker aborted", err: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &account{noPlaylists: tc.empty}
			p := &prompts{err: tc.err}
			var output bytes.Buffer
			if err := Run(context.Background(), a, p, &output, false); !errors.Is(err, tc.err) {
				t.Fatalf("error %v, want %v", err, tc.err)
			}
			if len(a.writes) != 0 || p.confirmations != 0 {
				t.Fatal("unexpected write or confirmation")
			}
		})
	}
}

func (p *prompts) Load(ctx context.Context, work func(context.Context) error) error { return work(ctx) }
