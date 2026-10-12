package qobuz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL     = "https://www.qobuz.com/api.json/0.2/"
	pageSize    = 100
	maxAttempts = 3
)

type Playlist struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	TracksCount int    `json:"tracks_count"`
	Owner       struct {
		Name string `json:"name"`
	} `json:"owner"`
}

type Track struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Streamable *bool  `json:"streamable"`
}

type Client struct {
	appID   string
	token   string
	baseURL string
	http    *http.Client
	wait    func(context.Context, time.Duration) error
}

func New(appID, token string) *Client {
	return &Client{
		appID: appID, token: token, baseURL: baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
			// Session headers must never follow a redirect to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		wait: wait,
	}
}

type APIError struct {
	Endpoint   string
	Status     int
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "Qobuz authentication failed; refresh the session token and its matching app ID"
	case http.StatusTooManyRequests:
		return "Qobuz rate limit reached; try again later"
	default:
		return fmt.Sprintf("Qobuz %s failed (status %d)", e.Endpoint, e.Status)
	}
}

func IsFatal(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403 || apiErr.Status == 429)
}

type uncertainWrite struct{ err error }

func (e *uncertainWrite) Error() string { return e.err.Error() }
func (e *uncertainWrite) Unwrap() error { return e.err }

type page[T any] struct {
	Total *int `json:"total"`
	Items []T  `json:"items"`
}

func nextOffset(offset int, total *int, count int) (int, bool, error) {
	if total == nil || *total < 0 || (count == 0 && offset < *total) {
		return 0, false, errors.New("Qobuz returned an incomplete page")
	}
	next := offset + count
	return next, next < *total, nil
}

func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	var playlists []Playlist
	for offset := 0; ; {
		var response struct {
			Playlists page[Playlist] `json:"playlists"`
		}
		if err := c.get(ctx, "playlist/getUserPlaylists", paging(offset), &response); err != nil {
			return nil, err
		}
		playlists = append(playlists, response.Playlists.Items...)
		next, more, err := nextOffset(offset, response.Playlists.Total, len(response.Playlists.Items))
		if err != nil {
			return nil, err
		}
		if !more {
			return playlists, nil
		}
		offset = next
	}
}

func (c *Client) Tracks(ctx context.Context, playlistID int64) ([]Track, error) {
	var tracks []Track
	for offset := 0; ; {
		params := paging(offset)
		params.Set("playlist_id", strconv.FormatInt(playlistID, 10))
		params.Set("extra", "tracks")
		var response struct {
			Tracks page[Track] `json:"tracks"`
		}
		if err := c.get(ctx, "playlist/get", params, &response); err != nil {
			return nil, err
		}
		tracks = append(tracks, response.Tracks.Items...)
		next, more, err := nextOffset(offset, response.Tracks.Total, len(response.Tracks.Items))
		if err != nil {
			return nil, err
		}
		if !more {
			return tracks, nil
		}
		offset = next
	}
}

func paging(offset int) url.Values {
	return url.Values{"limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}}
}

func (c *Client) FavoriteIDs(ctx context.Context) ([]int64, error) {
	ids := make([]int64, 0)
	for offset := 0; ; {
		params := paging(offset)
		params.Set("type", "tracks")
		var response struct {
			Tracks page[Track] `json:"tracks"`
		}
		if err := c.get(ctx, "favorite/getUserFavorites", params, &response); err != nil {
			return nil, err
		}
		for _, track := range response.Tracks.Items {
			if track.ID <= 0 {
				return nil, errors.New("Qobuz returned an invalid favorite track ID")
			}
			ids = append(ids, track.ID)
		}
		next, more, err := nextOffset(offset, response.Tracks.Total, len(response.Tracks.Items))
		if err != nil {
			return nil, err
		}
		if !more {
			return ids, nil
		}
		offset = next
	}
}

func (c *Client) FavoriteStatus(ctx context.Context, id int64) (bool, error) {
	var response struct {
		Status *bool `json:"status"`
	}
	params := url.Values{"type": {"track"}, "item_id": {strconv.FormatInt(id, 10)}}
	if err := c.get(ctx, "favorite/status", params, &response); err != nil {
		return false, err
	}
	if response.Status == nil {
		return false, errors.New("Qobuz returned no favorite status")
	}
	return *response.Status, nil
}

func (c *Client) Heart(ctx context.Context, id int64) error {
	params := url.Values{"track_ids": {strconv.FormatInt(id, 10)}}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		_, err := c.request(ctx, http.MethodPost, "favorite/create", params)
		if err == nil {
			hearted, checkErr := c.FavoriteStatus(ctx, id)
			if checkErr != nil {
				return fmt.Errorf("could not verify added heart: %w", checkErr)
			}
			if !hearted {
				return errors.New("Qobuz accepted the request but the track is still not hearted")
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var apiErr *APIError
		var uncertain *uncertainWrite
		switch {
		case errors.As(err, &apiErr) && apiErr.Status == 429:
			if attempt == maxAttempts-1 {
				return err
			}
			if err := c.wait(ctx, apiErr.RetryAfter); err != nil {
				return err
			}
		case errors.As(err, &uncertain):
			// The server may have committed the heart before the response failed.
			hearted, checkErr := c.FavoriteStatus(ctx, id)
			if checkErr != nil {
				return fmt.Errorf("could not verify uncertain heart: %w", checkErr)
			}
			if hearted {
				return nil
			}
			if attempt == maxAttempts-1 {
				return err
			}
			if err := c.wait(ctx, time.Second<<attempt); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return errors.New("heart attempts exhausted")
}

func (c *Client) get(ctx context.Context, endpoint string, params url.Values, out any) error {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		body, err := c.request(ctx, http.MethodGet, endpoint, params)
		if err == nil {
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("Qobuz %s returned unexpected data", endpoint)
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var apiErr *APIError
		delay := time.Second << attempt
		if errors.As(err, &apiErr) {
			if apiErr.Status != 429 && apiErr.Status < 500 {
				return err
			}
			if apiErr.Status == 429 {
				delay = apiErr.RetryAfter
			}
		}
		if attempt == maxAttempts-1 {
			return err
		}
		if err := c.wait(ctx, delay); err != nil {
			return err
		}
	}
	return errors.New("read attempts exhausted")
}

func (c *Client) request(ctx context.Context, method, endpoint string, params url.Values) ([]byte, error) {
	address := c.baseURL + endpoint
	var input io.Reader
	if method == http.MethodGet {
		if len(params) > 0 {
			address += "?" + params.Encode()
		}
	} else {
		input = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, address, input)
	if err != nil {
		return nil, errors.New("could not construct Qobuz request")
	}
	req.Header.Set("X-App-Id", c.appID)
	req.Header.Set("X-User-Auth-Token", c.token)
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		err = errors.New("could not reach Qobuz")
		if method == http.MethodPost {
			err = &uncertainWrite{err}
		}
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		err := error(&APIError{Endpoint: endpoint, Status: response.StatusCode, RetryAfter: retryDelay(response.Header.Get("Retry-After"), time.Now())})
		if method == http.MethodPost && response.StatusCode >= 500 {
			err = &uncertainWrite{err}
		}
		return nil, err
	}
	const maxBody = 8 << 20
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if readErr != nil || len(body) > maxBody || (!json.Valid(body) && response.StatusCode != http.StatusNoContent) {
		err := errors.New("Qobuz returned an unreadable response")
		if method == http.MethodPost {
			return nil, &uncertainWrite{err}
		}
		return nil, err
	}
	var envelope struct {
		Status json.RawMessage `json:"status"`
		Code   json.RawMessage `json:"code"`
	}
	if json.Unmarshal(body, &envelope) == nil && string(envelope.Status) == `"error"` {
		var code int
		// An unexpected error-code shape must still be treated as a failure.
		_ = json.Unmarshal(envelope.Code, &code)
		err := error(&APIError{Endpoint: endpoint, Status: code, RetryAfter: retryDelay(response.Header.Get("Retry-After"), time.Now())})
		if method == http.MethodPost && code >= 500 {
			err = &uncertainWrite{err}
		}
		return nil, err
	}
	return body, nil
}

func retryDelay(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if date, err := http.ParseTime(value); err == nil {
		return max(date.Sub(now), 0)
	}
	return time.Second
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
