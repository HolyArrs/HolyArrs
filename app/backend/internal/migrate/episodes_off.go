package migrate

import "github.com/HolyArrs/holyarrs/app/backend/internal/sources"

// episodesOffRows picks the episodes switched off in Sonarr from its
// episode rows (API or a backup's Episodes table). A row with no
// monitored value counts as on. title is the show when the rows do not
// name it (the API's per-show list).
func episodesOffRows(rows []any, title int64) []map[string]any {
	var out []map[string]any
	for _, e := range rows {
		m := mapOf(e)
		if m == nil || m["monitored"] == nil || boolean(m["monitored"]) {
			continue
		}
		id := title
		if id == 0 {
			id = num(m["seriesid"])
		}
		out = append(out, map[string]any{"titleid": id, "seasonnumber": m["seasonnumber"], "episodenumber": m["episodenumber"]})
	}
	return out
}

// episodesOffByTitle groups the off episodes by show.
func episodesOffByTitle(rows []map[string]any) map[int64][]sources.EpisodeNumber {
	out := map[int64][]sources.EpisodeNumber{}
	for _, m := range rows {
		id := num(m["titleid"])
		if id == 0 {
			continue
		}
		out[id] = append(out[id], sources.EpisodeNumber{Season: smallInt(num(m["seasonnumber"])), Episode: smallInt(num(m["episodenumber"]))})
	}
	return out
}
