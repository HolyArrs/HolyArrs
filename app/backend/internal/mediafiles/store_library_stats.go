package mediafiles

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strconv"
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

// bucket names one file's fact: an allowed value as itself, an empty one
// as missing, any other value as "other"; a file nobody read yet is
// "unchecked" (music by its extension when that names the fact) and one
// that could not be read is "unknown".
func (sf statFact) bucket(kind, status, ext, val string) string {
	status = strings.ToLower(status)
	if status == "complete" || status == "partial" {
		val = strings.ToLower(strings.TrimSpace(val))
		switch {
		case slices.Contains(sf.allowed, val):
			return val
		case val == "":
			return sf.missing
		}
		return "other"
	}
	if ext = strings.ToLower(ext); kind == "artist" && slices.Contains(sf.fromExt, ext) {
		return ext
	}
	if status == "" {
		return "unchecked"
	}
	return "unknown"
}

const statsFrom = ` FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id
	WHERE f.state = 'present' AND f.role = 'main' AND t.state != 'removed' AND t.kind IN (`

// factText is one fact as read from media_info: text as itself, a number
// as its digits, anything else (an object, a list, true) as a word no
// bucket allows, and nothing as "".
func factText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return "?"
}

// statTally adds up the titles, files and bytes of one bucket or kind.
type statTally struct {
	titles map[int64]bool
	files  int
	bytes  int64
}

func (st *statTally) add(title, size int64) {
	st.titles[title] = true
	st.files++
	st.bytes += size
}

// statKey is one bucket of one fact of one kind.
type statKey struct{ kind, fact, value string }

// LibraryStatsOf counts present main files by fact for each kind shown
// (nil shown counts every kind). Read-only; each file's media_info is read
// once (inspection status first, then each fact).
func LibraryStatsOf(ctx context.Context, s *database.Store, shown func(kind string) bool) (LibraryStats, error) {
	out := LibraryStats{Kinds: []LibraryKindStats{}}
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
	paths := `'$.inspection_status'`
	for _, sf := range statFacts {
		paths += `, '$.` + sf.key + `'`
	}
	rows, err := s.Read().QueryContext(ctx, `SELECT t.kind, t.id, f.size, coalesce(json_extract(f.media_info, `+paths+`), '[]'),
		coalesce(json_extract(f.quality, '$.source'), '')`+statsFrom+sqlList(kinds)+`)`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	perKind := map[string]*statTally{}
	buckets := map[statKey]*statTally{}
	var order []statKey
	var kind, raw, ext string
	var title, size int64
	for rows.Next() {
		if err := rows.Scan(&kind, &title, &size, &raw, &ext); err != nil {
			return out, err
		}
		var facts []any
		if json.Unmarshal([]byte(raw), &facts) != nil || len(facts) != len(statFacts)+1 {
			facts = make([]any, len(statFacts)+1)
			facts[0] = "unreadable" // read, but not as facts
		}
		status := factText(facts[0])
		if perKind[kind] == nil {
			perKind[kind] = &statTally{titles: map[int64]bool{}}
		}
		perKind[kind].add(title, size)
		for i, sf := range statFacts {
			if sf.video && kind == "artist" {
				continue
			}
			key := statKey{kind, sf.key, sf.bucket(kind, status, ext, factText(facts[i+1]))}
			b := buckets[key]
			if b == nil {
				if !statValue.MatchString(key.value) {
					key.value = "other"
				}
				if b = buckets[key]; b == nil {
					b = &statTally{titles: map[int64]bool{}}
					buckets[key] = b
					order = append(order, key)
				}
			}
			b.add(title, size)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i, k := range kinds {
		ks := &out.Kinds[i]
		if t := perKind[k]; t != nil {
			ks.Titles, ks.Files, ks.Bytes = len(t.titles), t.files, t.bytes
		}
		for _, key := range order {
			if key.kind == k {
				b := buckets[key]
				ks.Facts[key.fact] = append(ks.Facts[key.fact], LibraryStatBucket{Value: key.value, Titles: len(b.titles), Files: b.files, Bytes: b.bytes})
			}
		}
		for _, list := range ks.Facts {
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
