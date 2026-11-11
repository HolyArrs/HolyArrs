package mediafiles

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// LibraryStatBucket is one value of a file fact: how many titles and files
// have it and the disk they use.
type LibraryStatBucket struct {
	Value  string `json:"value"`
	Titles int    `json:"titles"`
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
}

// LibraryKindStats is the file facts of one kind (movies, shows, music).
type LibraryKindStats struct {
	Kind   string                         `json:"kind"`
	Titles int                            `json:"titles"`
	Files  int                            `json:"files"`
	Bytes  int64                          `json:"bytes"`
	Facts  map[string][]LibraryStatBucket `json:"facts"`
}

// LibraryStats is the Library's files by fact, per shown kind.
type LibraryStats struct {
	Kinds []LibraryKindStats `json:"kinds"`
}

// statFact is one fact read from files.media_info. Only allowed values come
// back as themselves; anything else is "other", so header text never shows.
type statFact struct {
	key, missing string
	allowed      []string
	video        bool
	// fromExt lists file extensions (quality.source) that stand for the
	// fact on music files nobody inspected.
	fromExt []string
}

var statFacts = []statFact{
	{key: "video_codec", missing: "unknown", video: true, allowed: []string{"h264", "hevc", "av1", "vp9", "mpeg2", "xvid", "vc1"}},
	{key: "resolution", missing: "unknown", video: true, allowed: []string{"2160p", "1080p", "720p", "576p", "480p"}},
	{key: "hdr", missing: "sdr", video: true, allowed: []string{"hdr10", "hlg", "dv", "dv+hdr10"}},
	{key: "container", missing: "unknown", allowed: []string{"matroska", "webm", "mp4"}, fromExt: []string{"flac", "mp3", "m4a", "ogg", "opus", "wav"}},
	{key: "audio_codec", missing: "unknown", allowed: []string{"aac", "ac3", "eac3", "dts", "truehd", "flac", "alac", "mp3", "mp2", "opus", "vorbis", "pcm", "wavpack", "tta"},
		fromExt: []string{"flac", "mp3", "opus"}},
	{key: "audio_channels", missing: "unknown", allowed: []string{"1.0", "2.0", "3.0", "4.0", "5.0", "5.1", "6.1", "7.1"}},
}

// statValue is what every bucket name looks like.
var statValue = regexp.MustCompile(`^[a-z0-9.+]+$`)

// statKinds are the title kinds in page order and their names.
var statKinds = []struct{ title, name string }{{"movie", "movies"}, {"show", "shows"}, {"artist", "music"}}

func sqlList(vals []string) string {
	q := make([]string, len(vals))
	for i, v := range vals {
		q[i] = "'" + v + "'"
	}
	return strings.Join(q, ", ")
}

// bucketSQL turns one fact of f.media_info into its bucket name.
func (sf statFact) bucketSQL() string {
	status := `lower(coalesce(json_extract(f.media_info, '$.inspection_status'), ''))`
	val := `lower(trim(coalesce(json_extract(f.media_info, '$.` + sf.key + `'), '')))`
	out := `CASE WHEN ` + status + ` IN ('complete', 'partial') THEN (CASE WHEN ` + val + ` IN (` + sqlList(sf.allowed) + `) THEN ` + val +
		` WHEN ` + val + ` = '' THEN '` + sf.missing + `' ELSE 'other' END)`
	if len(sf.fromExt) > 0 {
		ext := `lower(coalesce(json_extract(f.quality, '$.source'), ''))`
		out += ` WHEN t.kind = 'artist' AND ` + ext + ` IN (` + sqlList(sf.fromExt) + `) THEN ` + ext
	}
	return out + ` WHEN ` + status + ` = '' THEN 'unchecked' ELSE 'unknown' END`
}

const statsFrom = ` FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
	WHERE f.state = 'present' AND f.role = 'main' AND t.state != 'removed' AND t.kind IN (`

// LibraryStatsOf counts present main files by fact for each kind shown
// (nil shown counts every kind). Read-only.
func LibraryStatsOf(ctx context.Context, s *database.Store, shown func(kind string) bool) (LibraryStats, error) {
	out := LibraryStats{Kinds: []LibraryKindStats{}}
	byKind := map[string]*LibraryKindStats{}
	var kinds []string
	for _, k := range statKinds {
		if shown != nil && !shown(k.title) {
			continue
		}
		ks := LibraryKindStats{Kind: k.name, Facts: map[string][]LibraryStatBucket{}}
		for _, sf := range statFacts {
			if !sf.video || k.title != "artist" { // music files have no picture
				ks.Facts[sf.key] = []LibraryStatBucket{}
			}
		}
		out.Kinds = append(out.Kinds, ks)
		kinds = append(kinds, k.title)
	}
	if len(kinds) == 0 {
		return out, nil
	}
	for i := range out.Kinds {
		byKind[kinds[i]] = &out.Kinds[i]
	}
	from := statsFrom + sqlList(kinds) + `)`
	rows, err := s.Read().QueryContext(ctx, `SELECT t.kind, count(DISTINCT t.id), count(*), coalesce(sum(f.size), 0)`+from+` GROUP BY 1`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var kind string
		var titles, files int
		var bytes int64
		if err := rows.Scan(&kind, &titles, &files, &bytes); err != nil {
			rows.Close()
			return out, err
		}
		if k := byKind[kind]; k != nil {
			k.Titles, k.Files, k.Bytes = titles, files, bytes
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, sf := range statFacts {
		rows, err := s.Read().QueryContext(ctx, `SELECT t.kind, `+sf.bucketSQL()+`, count(DISTINCT t.id), count(*), coalesce(sum(f.size), 0)`+from+` GROUP BY 1, 2`)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var kind string
			var b LibraryStatBucket
			if err := rows.Scan(&kind, &b.Value, &b.Titles, &b.Files, &b.Bytes); err != nil {
				rows.Close()
				return out, err
			}
			k := byKind[kind]
			if _, ok := k.Facts[sf.key]; k == nil || !ok {
				continue
			}
			if !statValue.MatchString(b.Value) {
				b.Value = "other"
			}
			k.Facts[sf.key] = append(k.Facts[sf.key], b)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
	}
	for _, k := range out.Kinds {
		for _, list := range k.Facts {
			sort.Slice(list, func(i, j int) bool {
				if list[i].Files != list[j].Files {
					return list[i].Files > list[j].Files
				}
				return list[i].Value < list[j].Value
			})
		}
	}
	return out, nil
}
