package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// A big library's show, movie or artist list is one answer with no paging
// (35 MB for 7,700 shows). getList reads such a list one entry at a
// time and keeps only the fields the import uses, so memory stays near the
// size of what is kept, not the size of the answer.

// keepFunc turns one entry (keys already lower case) into the slim row kept,
// or nil to skip it.
type keepFunc func(map[string]any) map[string]any

// stream calls GET path and hands each entry of its JSON list to each,
// which decodes it from d. The answer may be up to MaxList bytes; it is never
// held whole.
//
// A big list can take longer than one call's Timeout to arrive, so Timeout
// here bounds silence (no bytes for that long), not the whole read; the
// caller's context still bounds the whole read.
func (c *apiClient) stream(ctx context.Context, path string, q url.Values, each func(d *json.Decoder) error) error {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	idle := time.AfterFunc(c.lim.Timeout, cancel)
	defer idle.Stop()
	sc := *c
	h := *c.h
	h.Timeout = 0
	sc.h = &h
	resp, err := sc.send(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrBadKey
	case resp.StatusCode >= 300:
		return fmt.Errorf("%w: it answered %d", ErrBadAnswer, resp.StatusCode)
	}
	// Refuse before reading when the app says how big it is.
	if resp.ContentLength > c.lim.MaxList {
		return ErrTooBig
	}
	g := &guardReader{r: resp.Body, max: c.lim.MaxList, maxEntry: c.lim.MaxAnswer, maxDepth: c.lim.MaxDepth, progress: func() { idle.Reset(c.lim.Timeout) }}
	defer func() { c.got += g.n }()
	d := json.NewDecoder(g)
	d.UseNumber()
	fail := func(err error) error {
		if g.err != nil {
			return g.err
		}
		if errors.Is(err, ErrTooBig) {
			return err
		}
		if pe := parent.Err(); pe != nil {
			return pe
		}
		var se *json.SyntaxError
		var te *json.UnmarshalTypeError
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || errors.As(err, &se) || errors.As(err, &te) {
			return fmt.Errorf("%w: not the JSON the app sends, or cut off", ErrBadAnswer)
		}
		return fmt.Errorf("%w: the answer was cut off", ErrUnreachable)
	}
	tok, err := d.Token()
	if err != nil {
		return fail(err)
	}
	if dl, ok := tok.(json.Delim); !ok || dl != '[' {
		return fmt.Errorf("%w: not a list", ErrBadAnswer)
	}
	for d.More() {
		if err := parent.Err(); err != nil {
			return err
		}
		if err := each(d); err != nil {
			return fail(err)
		}
	}
	if _, err := d.Token(); err != nil {
		return fail(err)
	}
	return nil
}

// getList reads a JSON list entry by entry, keeping keep's slim rows.
func (c *apiClient) getList(ctx context.Context, path string, q url.Values, keep keepFunc) ([]map[string]any, error) {
	var rows []map[string]any
	err := c.stream(ctx, path, q, func(d *json.Decoder) error {
		var e any
		if err := d.Decode(&e); err != nil {
			return err
		}
		if m, ok := e.(map[string]any); ok {
			if k := keep(lower(m)); k != nil {
				rows = append(rows, k)
				if len(rows) > c.lim.MaxRows {
					return ErrTooBig
				}
			}
		}
		return nil
	})
	return rows, err
}

// guardReader counts the bytes read and the JSON nesting as they pass,
// stopping at max bytes, at one entry over maxEntry bytes (the decoder holds
// a whole entry, so one huge entry would undo reading entry by entry), or
// past maxDepth.
type guardReader struct {
	r          io.Reader
	n, max     int64
	entry      int64
	maxEntry   int64
	depth      int
	maxDepth   int
	inStr, esc bool
	err        error
	progress   func()
}

func (g *guardReader) Read(p []byte) (int, error) {
	if g.err != nil {
		return 0, g.err
	}
	n, err := g.r.Read(p)
	if n > 0 && g.progress != nil {
		g.progress()
	}
	g.n += int64(n)
	if g.n > g.max {
		g.err = ErrTooBig
		return 0, g.err
	}
	for _, b := range p[:n] {
		if g.depth >= 1 {
			if g.entry++; g.maxEntry > 0 && g.entry > g.maxEntry {
				g.err = ErrTooBig
				return 0, g.err
			}
		}
		switch {
		case g.esc:
			g.esc = false
		case g.inStr:
			if b == '\\' {
				g.esc = true
			} else if b == '"' {
				g.inStr = false
			}
		case b == '"':
			g.inStr = true
		case b == '{' || b == '[':
			if g.depth++; g.depth > g.maxDepth {
				g.err = fmt.Errorf("%w: nested too deep", ErrBadAnswer)
				return 0, g.err
			}
		case b == '}' || b == ']':
			g.depth--
		case b == ',' && g.depth == 1:
			g.entry = 0
		}
	}
	return n, err
}

// pick copies the named keys of m that are present.
func pick(m map[string]any, keys []string) map[string]any {
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

// slimTitle is one show, movie or artist with only the fields the import
// reads (arr_build.go title, file_qualities.go, albumsByArtist); the rest of
// the entry is skipped while it is decoded, never held. Values stay untyped
// so an app's odd value is read the same way the whole answer was.
type slimTitle struct {
	ID, Path, RootFolderPath, Monitored, Added, Tags, QualityProfileID, ProfileID any
	TmdbID, ImdbID, TvdbID, TvMazeID, ForeignArtistID                             any
	Title, Year, MinimumAvailability, SeriesType, SeasonFolder, MonitorNewItems   any
	LanguageProfileID, ArtistName, Name, MetadataProfileID, ArtistMetadataID      any
	Seasons                                                                       slimSeasons
	MovieFile                                                                     *slimFile
	// Statistics is only read for an artist's file count (the folder step).
	Statistics *struct{ TrackFileCount any }
}

type slimSeason struct{ SeasonNumber, Monitored any }

// slimSeasons is a show's seasons; anything but a list reads as none.
type slimSeasons struct {
	list []slimSeason
	set  bool
}

func (s *slimSeasons) UnmarshalJSON(b []byte) error {
	var l []json.RawMessage
	if decodeNumbers(b, &l) != nil {
		return nil
	}
	s.set = true
	for _, e := range l {
		var se slimSeason
		if decodeNumbers(e, &se) == nil {
			s.list = append(s.list, se)
		}
	}
	return nil
}

// slimFile is a movie's file: its name, recorded quality and the release
// it came from (scene name and group).
type slimFile struct {
	ID, RelativePath, Quality, SceneName, ReleaseGroup any
	ok                                                 bool
}

func (f *slimFile) UnmarshalJSON(b []byte) error {
	var v struct{ ID, RelativePath, Quality, SceneName, ReleaseGroup any }
	if decodeNumbers(b, &v) == nil {
		f.ID, f.RelativePath, f.Quality, f.SceneName, f.ReleaseGroup, f.ok = v.ID, v.RelativePath, v.Quality, v.SceneName, v.ReleaseGroup, true
	}
	return nil
}

// row is the title as the lower-case row the builder reads.
func (t *slimTitle) row() map[string]any {
	m := make(map[string]any, 16)
	for k, v := range map[string]any{"id": t.ID, "path": t.Path, "rootfolderpath": t.RootFolderPath, "monitored": t.Monitored,
		"added": t.Added, "tags": t.Tags, "qualityprofileid": t.QualityProfileID, "profileid": t.ProfileID,
		"tmdbid": t.TmdbID, "imdbid": t.ImdbID, "tvdbid": t.TvdbID, "tvmazeid": t.TvMazeID, "foreignartistid": t.ForeignArtistID,
		"title": t.Title, "year": t.Year, "minimumavailability": t.MinimumAvailability, "seriestype": t.SeriesType,
		"seasonfolder": t.SeasonFolder, "monitornewitems": t.MonitorNewItems, "languageprofileid": t.LanguageProfileID,
		"artistname": t.ArtistName, "name": t.Name, "metadataprofileid": t.MetadataProfileID, "artistmetadataid": t.ArtistMetadataID} {
		if v != nil {
			m[k] = v
		}
	}
	if t.Statistics != nil && t.Statistics.TrackFileCount != nil {
		m["trackfilecount"] = t.Statistics.TrackFileCount
	}
	if t.Seasons.set {
		ss := make([]any, 0, len(t.Seasons.list))
		for _, se := range t.Seasons.list {
			ss = append(ss, map[string]any{"seasonnumber": se.SeasonNumber, "monitored": se.Monitored})
		}
		m["seasons"] = ss
	}
	if t.MovieFile != nil && t.MovieFile.ok {
		f := map[string]any{"id": t.MovieFile.ID, "relativepath": t.MovieFile.RelativePath, "quality": t.MovieFile.Quality}
		if t.MovieFile.SceneName != nil {
			f["scenename"] = t.MovieFile.SceneName
		}
		if t.MovieFile.ReleaseGroup != nil {
			f["releasegroup"] = t.MovieFile.ReleaseGroup
		}
		m["moviefile"] = f
	}
	return m
}

// getTitles reads a title list entry by entry into slim titles.
func (c *apiClient) getTitles(ctx context.Context, path string) ([]slimTitle, error) {
	var out []slimTitle
	err := c.stream(ctx, path, nil, func(d *json.Decoder) error {
		var t slimTitle
		if err := d.Decode(&t); err != nil {
			// An entry that is not an object is skipped, as before.
			var te *json.UnmarshalTypeError
			if errors.As(err, &te) {
				return nil
			}
			return err
		}
		if t.ID == nil && t.Path == nil && t.Title == nil && t.ArtistName == nil && t.Name == nil {
			return nil
		}
		out = append(out, t)
		if len(out) > c.lim.MaxRows {
			return ErrTooBig
		}
		return nil
	})
	return out, err
}

// decodeNumbers decodes b into v keeping numbers exact, as the whole answer was.
func decodeNumbers(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	return d.Decode(v)
}

// keepEpisodeFile and keepEpisode are the slim rows of Sonarr's per-show
// file and episode lists; an episode is kept when it has a file or is
// switched off.
func keepEpisodeFile(m map[string]any) map[string]any {
	return pick(m, []string{"id", "relativepath", "quality", "scenename", "releasegroup"})
}

func keepEpisode(m map[string]any) map[string]any {
	if num(m["episodefileid"]) == 0 && (m["monitored"] == nil || boolean(m["monitored"])) {
		return nil
	}
	return pick(m, []string{"episodefileid", "airdate", "seasonnumber", "episodenumber", "title", "monitored"})
}
