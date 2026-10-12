package heart

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"qobuz-kit/internal/display"
	"qobuz-kit/internal/qobuz"
)

var ErrIncomplete = errors.New("some playlist tracks could not be hearted; rerun to retry missing hearts")
var ErrQuit = errors.New("playlist selection finished")

type API interface {
	Playlists(context.Context) ([]qobuz.Playlist, error)
	Tracks(context.Context, int64) ([]qobuz.Track, error)
	FavoriteIDs(context.Context) ([]int64, error)
	Heart(context.Context, int64) error
}

type Prompts interface {
	Load(context.Context, func(context.Context) error) error
	Select(context.Context, []qobuz.Playlist) (qobuz.Playlist, error)
	Confirm(context.Context, int) (bool, error)
}

// Display keeps catalog text from inserting control sequences into the terminal.
func Display(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func Run(ctx context.Context, api API, prompts Prompts, output io.Writer, dryRun bool) error {
	fmt.Fprintln(output, display.Info.Render("Loading your Qobuz playlists..."))
	playlists, err := api.Playlists(ctx)
	if err != nil {
		return err
	}
	if len(playlists) == 0 {
		fmt.Fprintln(output, display.Info.Render("No playlists found in your account."))
		return nil
	}
	var sessionErr error
	for {
		playlist, err := prompts.Select(ctx, playlists)
		if errors.Is(err, ErrQuit) {
			return sessionErr
		}
		if err != nil {
			return err
		}
		err = processPlaylist(ctx, api, prompts, output, playlist, dryRun)
		if errors.Is(err, ErrQuit) {
			return sessionErr
		}
		if dryRun {
			return err
		}
		if errors.Is(err, ErrIncomplete) {
			sessionErr = ErrIncomplete
			fmt.Fprintln(output, display.Warning.Render("Some tracks could not be hearted. Select the playlist again to retry."))
		} else if err != nil {
			return err
		}
	}
}

func processPlaylist(ctx context.Context, api API, prompts Prompts, output io.Writer, playlist qobuz.Playlist, dryRun bool) error {
	var tracks []qobuz.Track
	var favorites []int64
	err := prompts.Load(ctx, func(loadCtx context.Context) error {
		var err error
		tracks, err = api.Tracks(loadCtx, playlist.ID)
		if err != nil {
			return err
		}
		favorites, err = api.FavoriteIDs(loadCtx)
		return err
	})
	if err != nil {
		return err
	}
	plan := MakePlan(tracks, favorites)
	fmt.Fprintln(output, "\n"+display.Title.Render(Display(playlist.Name)))
	fmt.Fprintln(output, display.Info.Render(fmt.Sprintf("%d entries | %d duplicates | %d already hearted | %d unavailable | %d invalid | %d to add",
		plan.Total, plan.Duplicates, plan.Existing, plan.Unavailable, plan.Invalid, len(plan.Missing))))
	for i, track := range tracks {
		if track.ID <= 0 {
			fmt.Fprintln(output, display.Warning.Render(fmt.Sprintf("Invalid entry %d: %s (missing valid track ID)", i+1, Display(track.Title))))
		}
	}
	if dryRun || len(plan.Missing) == 0 {
		if dryRun {
			fmt.Fprintln(output, display.Info.Render("Dry run: no hearts added."))
		} else {
			fmt.Fprintln(output, display.Success.Render("No new hearts to add."))
		}
		if plan.Invalid > 0 {
			return ErrIncomplete
		}
		return nil
	}
	confirmed, err := prompts.Confirm(ctx, len(plan.Missing))
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Fprintln(output, display.Warning.Render("Cancelled: no hearts added."))
		return nil
	}
	return apply(ctx, api, output, plan)
}

func apply(ctx context.Context, api API, output io.Writer, plan Plan) error {
	added, failed, processed := 0, 0, 0
	var stopped error
	for i, track := range plan.Missing {
		if err := ctx.Err(); err != nil {
			stopped = err
			break
		}
		err := api.Heart(ctx, track.ID)
		if ctx.Err() != nil {
			// An interrupted request may have completed remotely; leave it unprocessed.
			stopped = ctx.Err()
			break
		}
		processed++
		if err != nil {
			failed++
			fmt.Fprintln(output, display.Error.Render(fmt.Sprintf("Failed track %d (%s): %s", track.ID, Display(track.Title), err)))
			if qobuz.IsFatal(err) {
				stopped = err
				break
			}
		} else {
			added++
			fmt.Fprintln(output, display.Success.Render(fmt.Sprintf("[%d/%d] Hearted %s (track %d)", i+1, len(plan.Missing), Display(track.Title), track.ID)))
		}
	}
	summaryStyle := display.Success
	if failed > 0 || plan.Invalid > 0 || stopped != nil {
		summaryStyle = display.Warning
	}
	fmt.Fprintln(output, "\n"+summaryStyle.Render(fmt.Sprintf("Added: %d | Already hearted: %d | Failed: %d | Unavailable: %d | Invalid: %d | Unprocessed: %d",
		added, plan.Existing, failed, plan.Unavailable, plan.Invalid, len(plan.Missing)-processed)))
	if stopped != nil {
		return stopped
	}
	if failed > 0 || plan.Invalid > 0 {
		return ErrIncomplete
	}
	return nil
}
