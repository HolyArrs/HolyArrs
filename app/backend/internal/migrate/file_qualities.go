package migrate

import (
	"context"
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// TitleFile is one file the old app holds for a title and the quality it
// recorded for it ("WEBDL-1080p"). The old app read the quality from the
// release it downloaded, which the file name often no longer says.
type TitleFile struct {
	Name    string `json:"name"` // the file's base name
	Quality string `json:"quality"`
}

// fileQualitiesAPI reads the recorded quality of every file: Radarr keeps
// it on each movie, Sonarr per series. A series whose list cannot be read
// is skipped with a note; qualities are a help, not needed.
func (r arrReader) fileQualitiesAPI(ctx context.Context, c *apiClient, out *arrRows) {
	switch r.kind {
	case Radarr:
		var rows []map[string]any
		for _, m := range out.sets["titles"] {
			if f := mapOf(m["moviefile"]); f != nil {
				rows = append(rows, map[string]any{"titleid": m["id"], "relativepath": f["relativepath"], "quality": qualityName(f["quality"], nil)})
			}
		}
		out.sets["filequalities"] = rows
	case Sonarr:
		var rows []map[string]any
		skipped := 0
		for _, m := range out.sets["titles"] {
			id := num(m["id"])
			if id == 0 || ctx.Err() != nil {
				continue
			}
			var got []any
			if err := c.getJSON(ctx, r.apiBase()+"/episodefile", url.Values{"seriesId": {strconv.FormatInt(id, 10)}}, &got); err != nil {
				skipped++
				continue
			}
			for _, e := range got {
				if f := mapOf(e); f != nil {
					rows = append(rows, map[string]any{"titleid": id, "relativepath": f["relativepath"], "quality": qualityName(f["quality"], nil)})
				}
			}
		}
		if skipped > 0 {
			out.notes = append(out.notes, "file qualities of "+strconv.Itoa(skipped)+" series could not be read; skipped")
		}
		out.sets["filequalities"] = rows
	}
}

// fileQualitiesBackup reads the same from a backup's MovieFiles or
// EpisodeFiles table, where the quality is an id named in
// QualityDefinitions.
func (r arrReader) fileQualitiesBackup(ctx context.Context, t table, out *arrRows) {
	name, link := map[Kind]string{Radarr: "MovieFiles", Sonarr: "EpisodeFiles"}[r.kind], map[Kind]string{Radarr: "movieid", Sonarr: "seriesid"}[r.kind]
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
		outRows = append(outRows, map[string]any{"titleid": m[link], "relativepath": m["relativepath"], "quality": qualityName(m["quality"], defs)})
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
		if q == "" || p == "" {
			continue
		}
		base := path.Base(strings.ReplaceAll(p, `\`, "/"))
		out[num(m["titleid"])] = append(out[num(m["titleid"])], TitleFile{Name: base, Quality: q})
	}
	return out
}
