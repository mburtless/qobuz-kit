package heart

import "qobuz-kit/internal/qobuz"

type Plan struct {
	Total       int
	Duplicates  int
	Existing    int
	Invalid     int
	Unavailable int
	Missing     []qobuz.Track
}

func MakePlan(tracks []qobuz.Track, favorites []int64) Plan {
	plan := Plan{Total: len(tracks)}
	favorited := make(map[int64]bool, len(favorites))
	for _, id := range favorites {
		favorited[id] = true
	}
	seen := make(map[int64]bool, len(tracks))
	for _, track := range tracks {
		switch {
		case track.ID <= 0:
			plan.Invalid++
		case seen[track.ID]:
			plan.Duplicates++
		default:
			seen[track.ID] = true
			if favorited[track.ID] {
				plan.Existing++
			} else if track.Streamable != nil && !*track.Streamable {
				plan.Unavailable++
			} else {
				plan.Missing = append(plan.Missing, track)
			}
		}
	}
	return plan
}
