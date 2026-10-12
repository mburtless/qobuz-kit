package heart

import (
	"reflect"
	"testing"

	"qobuz-kit/internal/qobuz"
)

func TestMakePlan(t *testing.T) {
	streamable, unavailable := true, false
	for _, tc := range []struct {
		name      string
		tracks    []qobuz.Track
		favorites []int64
		want      Plan
	}{
		{name: "empty"},
		{name: "skip unavailable and deduplicate", tracks: []qobuz.Track{{ID: 1, Streamable: &unavailable}, {ID: 1, Streamable: &unavailable}, {ID: 2, Streamable: &streamable}, {ID: 3}}, want: Plan{Total: 4, Unavailable: 1, Duplicates: 1, Missing: []qobuz.Track{{ID: 2, Streamable: &streamable}, {ID: 3}}}},
		{name: "already hearted unavailable track", tracks: []qobuz.Track{{ID: 1, Streamable: &unavailable}}, favorites: []int64{1}, want: Plan{Total: 1, Existing: 1}},
		{name: "already complete", tracks: []qobuz.Track{{ID: 1}, {ID: 2}}, favorites: []int64{1, 2, 99}, want: Plan{Total: 2, Existing: 2}},
		{name: "deduplicate and preserve order", tracks: []qobuz.Track{{ID: 3}, {ID: 1}, {ID: 3}, {ID: 2}, {ID: 1}}, favorites: []int64{1}, want: Plan{Total: 5, Duplicates: 2, Existing: 1, Missing: []qobuz.Track{{ID: 3}, {ID: 2}}}},
		{name: "invalid IDs", tracks: []qobuz.Track{{ID: 0}, {ID: -1}, {ID: 7, Title: "A song"}}, want: Plan{Total: 3, Invalid: 2, Missing: []qobuz.Track{{ID: 7, Title: "A song"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]qobuz.Track(nil), tc.tracks...)
			if got := MakePlan(tc.tracks, tc.favorites); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(original, tc.tracks) {
				t.Fatal("input tracks changed")
			}
		})
	}
}

func TestDisplay(t *testing.T) {
	if got := Display("Jazz\n\x1b[31m\t夜"); got != "Jazz  [31m 夜" {
		t.Fatalf("unexpected display text %q", got)
	}
}
