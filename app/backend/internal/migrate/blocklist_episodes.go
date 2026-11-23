package migrate

import (
	"context"
	"net/url"
	"slices"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Sonarr keeps the exact episodes of each blocklist entry as episode ids.
// They are turned into season and episode numbers here, so an anime or
// daily release name never has to be guessed from. Both readers are a
// help: when the numbers can't be read, the name decides as before.

// blockedIDs lists the episode ids of every Sonarr blocklist row.
func (r arrReader) blockedIDs(out *arrRows) []int64 {
	if r.kind != Sonarr {
		return nil
	}
	var all []int64
	for _, m := range out.sets["blocklist"] {
		for _, id := range ids(m["episodeids"]) {
			if !slices.Contains(all, id) {
				all = append(all, id)
			}
		}
	}
	return all
}

// setBlockedEpisodes writes the numbers found onto each row as "episodes".
func setBlockedEpisodes(out *arrRows, nums map[int64]sources.EpisodeNumber) {
	for _, m := range out.sets["blocklist"] {
		var eps []sources.EpisodeNumber
		for _, id := range ids(m["episodeids"]) {
			if n, ok := nums[id]; ok {
				eps = append(eps, n)
			}
		}
		m["episodes"] = eps
	}
}

// blockedEpisodesAPI asks Sonarr for the episodes by id, 100 at a time.
func (r arrReader) blockedEpisodesAPI(ctx context.Context, c *apiClient, out *arrRows) {
	want := r.blockedIDs(out)
	nums := map[int64]sources.EpisodeNumber{}
	for chunk := range slices.Chunk(want, 100) {
		q := url.Values{}
		for _, id := range chunk {
			q.Add("episodeIds", strconv.FormatInt(id, 10))
		}
		var got []map[string]any
		if err := c.getJSON(ctx, r.apiBase()+"/episode", q, &got); err != nil {
			out.notes = append(out.notes, "the app did not say which episodes its blocklist was for; matched by release name")
			break
		}
		for _, e := range got {
			e = lower(e)
			nums[num(e["id"])] = sources.EpisodeNumber{Season: smallInt(num(e["seasonnumber"])), Episode: smallInt(num(e["episodenumber"]))}
		}
	}
	if len(want) > 0 {
		setBlockedEpisodes(out, nums)
	}
}

// blockedEpisodes reads the "episodes" a reader set on a blocklist row.
func blockedEpisodes(v any) []sources.EpisodeNumber {
	eps, _ := v.([]sources.EpisodeNumber)
	return eps
}
