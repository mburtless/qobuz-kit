package qobuz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-App-Id") != "test-app" || r.Header.Get("X-User-Auth-Token") != "test-token" {
			t.Error("missing session headers")
		}
		if strings.Contains(r.URL.RawQuery, "test-token") {
			t.Error("token in request URL")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	c := New("test-app", "test-token")
	c.baseURL = server.URL + "/"
	c.wait = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestPagination(t *testing.T) {
	for _, kind := range []string{"playlists", "tracks", "favorites"} {
		t.Run(kind, func(t *testing.T) {
			var offsets []string
			c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Query().Get("limit") != "100" {
					t.Error("unexpected method or page limit")
				}
				if kind == "tracks" && (r.URL.Query().Get("extra") != "tracks" || r.URL.Query().Get("playlist_id") != "42") {
					t.Error("missing playlist track parameters")
				}
				offset := r.URL.Query().Get("offset")
				offsets = append(offsets, offset)
				items := `[{"id":1},{"id":2}]`
				if offset == "2" {
					items = `[{"id":3}]`
				}
				responseKind := kind
				if kind == "favorites" {
					responseKind = "tracks"
					if r.URL.Path != "/favorite/getUserFavorites" || r.URL.Query().Get("type") != "tracks" {
						t.Error("incorrect favorites request")
					}
				}
				fmt.Fprintf(w, `{"%s":{"total":3,"items":%s}}`, responseKind, items)
			})
			var ids []int64
			if kind == "playlists" {
				items, err := c.Playlists(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range items {
					ids = append(ids, item.ID)
				}
			} else if kind == "favorites" {
				var err error
				ids, err = c.FavoriteIDs(context.Background())
				if err != nil {
					t.Fatal(err)
				}
			} else {
				items, err := c.Tracks(context.Background(), 42)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range items {
					ids = append(ids, item.ID)
				}
			}
			if !reflect.DeepEqual(offsets, []string{"0", "2"}) || !reflect.DeepEqual(ids, []int64{1, 2, 3}) {
				t.Fatalf("offsets %v, IDs %v", offsets, ids)
			}
		})
	}
}

func TestIncompletePages(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantErr    bool
	}{
		{"empty playlist", `{"tracks":{"total":0,"items":[]}}`, false},
		{"empty early page", `{"tracks":{"total":2,"items":[]}}`, true},
		{"missing total", `{"tracks":{"items":[]}}`, true},
		{"negative total", `{"tracks":{"total":-1,"items":[]}}`, true},
		{"missing tracks", `{}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, tc.body) })
			_, err := c.Tracks(context.Background(), 42)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFavoriteIDs(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []int64
		wantErr    bool
	}{
		{"IDs", `{"tracks":{"total":3,"items":[{"id":1},{"id":2},{"id":3}]}}`, []int64{1, 2, 3}, false},
		{"empty favorites", `{"tracks":{"total":0,"items":[]}}`, []int64{}, false},
		{"missing favorites", `{}`, nil, true},
		{"null favorites", `{"tracks":null}`, nil, true},
		{"missing total", `{"tracks":{"items":[]}}`, nil, true},
		{"incomplete page", `{"tracks":{"total":25,"items":[]}}`, nil, true},
		{"invalid ID", `{"tracks":{"total":1,"items":[{"id":0}]}}`, nil, true},
		{"bad IDs", `{"tracks":{"total":1,"items":[{"id":"abc"}]}}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/favorite/getUserFavorites" || r.URL.Query().Get("type") != "tracks" || r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("offset") != "0" {
					t.Error("wrong favorites endpoint")
				}
				fmt.Fprint(w, tc.body)
			})
			got, err := c.FavoriteIDs(context.Background())
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, error %v", got, err)
			}
		})
	}
}

func TestHeartRetries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statuses   []int
		body       string
		statusBody string
		statusCode int
		wantPosts  int
		wantChecks int
		wantErr    bool
		wantFatal  bool
	}{
		{name: "successful response without persisted heart", statuses: []int{200}, statusBody: `{"status":false}`, wantPosts: 1, wantChecks: 1, wantErr: true},
		{name: "success", statuses: []int{200}, statusBody: `{"status":true}`, wantPosts: 1, wantChecks: 1},
		{name: "retry explicit rate limit", statuses: []int{429, 200}, statusBody: `{"status":true}`, wantPosts: 2, wantChecks: 1},
		{name: "rate limit exhausted", statuses: []int{429}, wantPosts: 3, wantErr: true, wantFatal: true},
		{name: "rejected track", statuses: []int{400}, wantPosts: 1, wantErr: true},
		{name: "expired token", statuses: []int{401}, wantPosts: 1, wantErr: true, wantFatal: true},
		{name: "forbidden", statuses: []int{403}, wantPosts: 1, wantErr: true, wantFatal: true},
		{name: "uncertain but already hearted", statuses: []int{500}, statusBody: `{"status":true}`, wantPosts: 1, wantChecks: 1},
		{name: "verify before retrying", statuses: []int{500, 200}, statusBody: `{"status":false}`, wantPosts: 2, wantChecks: 2},
		{name: "ambiguous retries bounded", statuses: []int{500}, statusBody: `{"status":false}`, wantPosts: 3, wantChecks: 3, wantErr: true},
		{name: "verification expired token", statuses: []int{500}, statusCode: 401, wantPosts: 1, wantChecks: 1, wantErr: true, wantFatal: true},
		{name: "verification missing status", statuses: []int{500}, statusBody: `{}`, wantPosts: 1, wantChecks: 1, wantErr: true},
		{name: "bad write response", statuses: []int{200}, body: `broken`, statusBody: `{"status":true}`, wantPosts: 1, wantChecks: 1},
		{name: "API error in successful HTTP", statuses: []int{200}, body: `{"status":"error","code":401,"message":"test-token"}`, wantPosts: 1, wantErr: true, wantFatal: true},
		{name: "API error with unexpected code", statuses: []int{200}, body: `{"status":"error","code":"unexpected"}`, wantPosts: 1, wantErr: true},
		{name: "uncertain API error", statuses: []int{200}, body: `{"status":"error","code":500}`, statusBody: `{"status":true}`, wantPosts: 1, wantChecks: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts, checks := 0, 0
			c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/favorite/create":
					if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
						t.Error("incorrect favorite request")
					}
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.PostForm.Get("track_ids") != "7" || len(r.PostForm) != 1 {
						t.Errorf("unexpected write form: %v", r.PostForm)
					}
					code := tc.statuses[min(posts, len(tc.statuses)-1)]
					posts++
					w.Header().Set("Retry-After", "2")
					w.WriteHeader(code)
					body := tc.body
					if body == "" {
						body = `{"status":"success"}`
					}
					fmt.Fprint(w, body)
				case "/favorite/status":
					checks++
					if r.Method != http.MethodGet || r.URL.Query().Get("type") != "track" || r.URL.Query().Get("item_id") != "7" {
						t.Error("incorrect status query")
					}
					if tc.statusCode != 0 {
						w.WriteHeader(tc.statusCode)
					}
					body := tc.statusBody
					if tc.name == "verify before retrying" && posts == 2 {
						body = `{"status":true}`
					}
					if body == "" {
						body = `{"status":false}`
					}
					fmt.Fprint(w, body)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
			})
			var delays []time.Duration
			c.wait = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
			err := c.Heart(context.Background(), 7)
			if (err != nil) != tc.wantErr || IsFatal(err) != tc.wantFatal {
				t.Fatalf("error %v (fatal %v)", err, IsFatal(err))
			}
			if posts != tc.wantPosts || checks != tc.wantChecks {
				t.Fatalf("posts %d, checks %d; want %d, %d", posts, checks, tc.wantPosts, tc.wantChecks)
			}
			if err != nil && strings.Contains(err.Error(), "test-token") {
				t.Fatal("secret leaked in error")
			}
			if tc.statuses[0] == 429 && (len(delays) == 0 || delays[0] != 2*time.Second) {
				t.Fatalf("Retry-After ignored: %v", delays)
			}
		})
	}
}

func TestReadRetries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		codes     []int
		wantCalls int
		wantErr   bool
	}{
		{"transient service failure", []int{503, 503, 200}, 3, false},
		{"rate limited", []int{429, 200}, 2, false},
		{"exhausted rate limit", []int{429}, 3, true},
		{"authentication", []int{401}, 1, true},
		{"not found", []int{404}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
				code := tc.codes[min(calls, len(tc.codes)-1)]
				calls++
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(code)
				fmt.Fprint(w, `{"tracks":{"total":0,"items":[]}}`)
			})
			_, err := c.FavoriteIDs(context.Background())
			if (err != nil) != tc.wantErr || calls != tc.wantCalls {
				t.Fatalf("calls %d, error %v", calls, err)
			}
		})
	}
}

func TestCancelledRetryAndRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	c := fixture(t, func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(429); cancel() })
	c.wait = wait
	_, err := c.FavoriteIDs(ctx)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("error %v, requests %d", err, calls)
	}
	if err := c.Heart(ctx, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error %v", err)
	}
	if calls != 1 {
		t.Fatal("request sent after cancellation")
	}
}

func TestRetryDelay(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"2", 2 * time.Second}, {"0", 0}, {"-1", time.Second}, {"invalid", time.Second},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second},
		{now.Add(-time.Second).Format(http.TimeFormat), 0},
	} {
		if got := retryDelay(tc.value, now); got != tc.want {
			t.Errorf("%q: %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestRedirectDoesNotForwardSession(t *testing.T) {
	forwarded := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
		fmt.Fprint(w, `{"tracks":{"total":0,"items":[]}}`)
	}))
	defer destination.Close()
	c := fixture(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) })
	_, err := c.FavoriteIDs(context.Background())
	if err == nil || forwarded {
		t.Fatalf("redirect followed: %v, error %v", forwarded, err)
	}
}

func TestLostWriteResponseChecksStatus(t *testing.T) {
	posts, checks := 0, 0
	c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favorite/create" {
			posts++
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
			return
		}
		checks++
		fmt.Fprint(w, `{"status":true}`)
	})
	if err := c.Heart(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || checks != 1 {
		t.Fatalf("posts %d, checks %d", posts, checks)
	}
}

func TestFavoriteIDsIncludesTracksBeyondFirstPage(t *testing.T) {
	var offsets []int
	c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/favorite/getUserFavorites" || r.URL.Query().Get("type") != "tracks" {
			t.Error("incorrect favorites endpoint")
			w.WriteHeader(400)
			return
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		offsets = append(offsets, offset)
		var items []Track
		for id := offset + 1; id <= min(offset+100, 125); id++ {
			items = append(items, Track{ID: int64(id)})
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"tracks": map[string]any{"total": 125, "items": items}}); err != nil {
			t.Error(err)
		}
	})
	ids, err := c.FavoriteIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 125 || ids[124] != 125 || !reflect.DeepEqual(offsets, []int{0, 100}) {
		t.Fatalf("got %d favorites, offsets %v", len(ids), offsets)
	}
}

func TestTrackAvailability(t *testing.T) {
	c := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tracks":{"total":4,"items":[{"id":1,"streamable":false},{"id":2,"streamable":true},{"id":3},{"id":4,"streamable":null}]}}`)
	})
	tracks, err := c.Tracks(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 4 {
		t.Fatalf("got %d tracks", len(tracks))
	}
	if tracks[0].Streamable == nil || *tracks[0].Streamable || tracks[1].Streamable == nil || !*tracks[1].Streamable || tracks[2].Streamable != nil || tracks[3].Streamable != nil {
		t.Fatalf("incorrect availability: %#v", tracks)
	}
}
