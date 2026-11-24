package migrate

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// TitleFile is one file the old app holds for a title and the quality it
// recorded for it ("WEBDL-1080p"). The old app read the quality from the
// release it downloaded, which the file name often no longer says.
type TitleFile struct {
	Name    string `json:"name"` // the file's base name
	Quality string `json:"quality"`
	// Episodes are the old app's episodes the file holds (Sonarr).
	Episodes []OldEpisode `json:"episodes,omitempty"`
}

// OldEpisode is one episode as the old app lists it.
type OldEpisode = library.OldEpisode

// fileQualitiesAPI reads the recorded quality of every file: Radarr keeps
// it on each movie, Sonarr per series. A series whose list cannot be read
// is skipped with a note; qualities are a help, not needed.
func (r arrReader) fileQualitiesAPI(ctx context.Context, c *apiClient, out *arrRows) {
	switch r.kind {
	case Radarr:
		var rows []map[string]any
		out.eachTitle(func(m map[string]any) {
			if f := mapOf(m["moviefile"]); f != nil {
				rows = append(rows, map[string]any{"titleid": m["id"], "relativepath": f["relativepath"], "quality": qualityName(f["quality"], nil)})
			}
		})
		out.sets["filequalities"] = rows
	case Sonarr:
		var rows, off []map[string]any
		skipped := 0
		var ids []int64
		out.eachTitle(func(m map[string]any) { ids = append(ids, num(m["id"])) })
		for _, id := range ids {
			if id == 0 || ctx.Err() != nil {
				continue
			}
			q := url.Values{"seriesId": {strconv.FormatInt(id, 10)}}
			got, err := c.getList(ctx, r.apiBase()+"/episodefile", q, keepEpisodeFile)
			if err != nil {
				skipped++
				continue
			}
			// The episode list is read once here: a show whose seasons differ
			// places a file only where its episode's day and name agree.
			epRows, _ := c.getList(ctx, r.apiBase()+"/episode", q, keepEpisode)
			eps := make([]any, len(epRows))
			for i, e := range epRows {
				eps[i] = e
			}
			byFile := oldEpisodesByFile(eps)
			off = append(off, episodesOffRows(eps, id)...)
			for _, f := range got {
				rows = append(rows, map[string]any{"titleid": id, "relativepath": f["relativepath"], "quality": qualityName(f["quality"], nil), "episodes": byFile[num(f["id"])]})
			}
		}
		if skipped > 0 {
			out.notes = append(out.notes, "file qualities of "+strconv.Itoa(skipped)+" series could not be read; skipped")
		}
		out.sets["filequalities"] = rows
		out.sets["episodesoff"] = off
	}
}

// fileQualitiesBackup reads the same from a backup's MovieFiles or
// EpisodeFiles table, where the quality is an id named in
// QualityDefinitions.
func (r arrReader) fileQualitiesBackup(ctx context.Context, t table, out *arrRows) {
	name, link := map[Kind]string{Radarr: "MovieFiles", Sonarr: "EpisodeFiles"}[r.kind], map[Kind]string{Radarr: "movieid", Sonarr: "seriesid"}[r.kind]
	// The episode list is read first: its off episodes count even when
	// the backup has no file table.
	var byFile map[int64][]OldEpisode
	if r.kind == Sonarr && t.exists(ctx, "Episodes") {
		if eps, err := t.all(ctx, "Episodes"); err == nil {
			list := make([]any, len(eps))
			for i, e := range eps {
				list[i] = e
			}
			byFile = oldEpisodesByFile(list)
			out.sets["episodesoff"] = episodesOffRows(list, 0)
		}
	}
	if name == "" || !t.exists(ctx, name) {
		return
	}
	rows, err := t.all(ctx, name)
	if err != nil {
		out.notes = append(out.notes, "file qualities could not be read; skipped")
		return
	}
	defs := map[int64]string{}
	for _, m := range out.sets["qualitydefs"] {
		defs[num(m["quality"])] = str(m["title"])
	}
	var outRows []map[string]any
	for _, m := range rows {
		outRows = append(outRows, map[string]any{"titleid": m[link], "relativepath": m["relativepath"], "quality": qualityName(m["quality"], defs), "episodes": byFile[num(m["id"])]})
	}
	out.sets["filequalities"] = outRows
}

// qualityName reads a recorded quality: the API's {"quality":{"name":...}}
// or a backup's JSON text {"quality":7} with the id named in defs.
func qualityName(v any, defs map[int64]string) string {
	if s, ok := v.(string); ok && strings.HasPrefix(strings.TrimSpace(s), "{") {
		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			return ""
		}
		v = m
	}
	q := mapOf(v)
	if q == nil {
		return ""
	}
	inner := q["quality"]
	if m := mapOf(inner); m != nil {
		return str(m["name"])
	}
	if defs != nil {
		return defs[num(inner)]
	}
	return ""
}

// mapOf is v as a map with lower-case keys (nil when it is not a map).
func mapOf(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return lower(m)
	}
	return nil
}

// filesByTitle groups the recorded qualities by the old app's title id.
func filesByTitle(rows []map[string]any) map[int64][]TitleFile {
	out := map[int64][]TitleFile{}
	for _, m := range rows {
		q, p := str(m["quality"]), str(m["relativepath"])
		eps, _ := m["episodes"].([]OldEpisode)
		if q == "" && len(eps) == 0 || p == "" {
			continue
		}
		base := path.Base(strings.ReplaceAll(p, `\`, "/"))
		out[num(m["titleid"])] = append(out[num(m["titleid"])], TitleFile{Name: base, Quality: q, Episodes: eps})
	}
	return out
}

// oldEpisodesByFile groups the old app's episodes (API rows or a backup's
// Episodes table) by the file that holds them.
func oldEpisodesByFile(rows []any) map[int64][]OldEpisode {
	out := map[int64][]OldEpisode{}
	for _, e := range rows {
		m := mapOf(e)
		if m == nil || num(m["episodefileid"]) == 0 {
			continue
		}
		day := str(m["airdate"])
		if len(day) > 10 {
			day = day[:10]
		}
		out[num(m["episodefileid"])] = append(out[num(m["episodefileid"])], OldEpisode{Season: int(num(m["seasonnumber"])), Number: int(num(m["episodenumber"])), AirDate: day, Title: str(m["title"])})
	}
	return out
}

// fileFacts is the recorded quality and the old app's episodes of each
// file, by its base name (nil when there are none). A base name that two
// files share gets no old episodes: which file they belong to is unknown.
func fileFacts(files []TitleFile) (map[string]string, map[string][]OldEpisode) {
	var q map[string]string
	var eps map[string][]OldEpisode
	seen := map[string]int{}
	for _, f := range files {
		seen[f.Name]++
	}
	for _, f := range files {
		if f.Quality != "" {
			if q == nil {
				q = map[string]string{}
			}
			q[f.Name] = f.Quality
		}
		if len(f.Episodes) > 0 && seen[f.Name] == 1 {
			if eps == nil {
				eps = map[string][]OldEpisode{}
			}
			eps[f.Name] = f.Episodes
		}
	}
	return q, eps
}
