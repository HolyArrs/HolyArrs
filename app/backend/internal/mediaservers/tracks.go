package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Default tracks: the owner lists sound and subtitle languages in order, and
// the media servers start each file in the first one the file has.
//   - Plex: each file of a Library title gets its sound and subtitle track
//     picked for the owner's Plex account. A file is set once per list, so a
//     track someone changes in Plex stays until the list changes.
//   - Jellyfin and Emby keep one language per person, not per file: each
//     person who has not picked their own gets the first language of each list.

// TrackLanguages reads language codes for default tracks; main fills it with
// the subtitles module's table.
type TrackLanguages struct {
	// Norm turns a code or name into the app's code (eng, en-US: en); "" unknown.
	Norm func(string) string
	// Match says a track in got meets a wish for want (pt takes pt-br).
	Match func(want, got string) bool
	// Three is a code's ISO 639-2 form (en: eng), as Jellyfin and Emby keep it.
	Three func(string) string
}

func (t TrackLanguages) norm(s string) string {
	if t.Norm == nil {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return t.Norm(s)
}

func (t TrackLanguages) match(want, got string) bool {
	if t.Match == nil {
		return want != "" && want == got
	}
	return t.Match(want, got)
}

func (t TrackLanguages) three(s string) string {
	if t.Three == nil {
		return s
	}
	return t.Three(s)
}

// SubtitlesOff in the subtitle list means "no subtitles".
const SubtitlesOff = "off"

// maxTrackLangs is the most languages in one list.
const maxTrackLangs = 5

const (
	tracksKey      = "default_tracks"          // "on"; anything else is off
	tracksAudioKey = "default_tracks_audio"    // codes, comma separated, in order
	tracksSubsKey  = "default_tracks_subs"     // codes or "off", in order
	tracksJellyKey = "default_tracks_jf_given" // "audio|subs" last given to Jellyfin and Emby people
)

// TrackSettings is the owner's default track choice.
type TrackSettings struct {
	Enabled   bool     `json:"enabled"`
	Audio     []string `json:"audio"`
	Subtitles []string `json:"subtitles"`
}

// choice is the lists as one string, so a file set for other lists is set again.
func (s TrackSettings) choice() string {
	return strings.Join(s.Audio, ",") + "|" + strings.Join(s.Subtitles, ",")
}

func splitCodes(s string) []string {
	out := []string{}
	for _, c := range strings.Split(s, ",") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// ReadTrackSettings reads the owner's default track choice.
func ReadTrackSettings(ctx context.Context, q database.Querier) TrackSettings {
	return TrackSettings{
		Enabled:   database.SettingOr(ctx, q, watchGroup, tracksKey, "") == "on",
		Audio:     splitCodes(database.SettingOr(ctx, q, watchGroup, tracksAudioKey, "")),
		Subtitles: splitCodes(database.SettingOr(ctx, q, watchGroup, tracksSubsKey, "")),
	}
}

// CleanTrackLangs turns what was sent into known codes, in order, once each.
// "off" is allowed in the subtitle list only; what comes after it is never
// reached, so it is dropped.
func CleanTrackLangs(langs TrackLanguages, in []string, subs bool) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, c := range in {
		c = strings.ToLower(strings.TrimSpace(c))
		if c == "" {
			continue
		}
		if c == SubtitlesOff {
			if !subs {
				return nil, errors.New("sound can't be off")
			}
			out = append(out, c)
			break
		}
		code := langs.norm(c)
		if code == "" {
			return nil, fmt.Errorf("%q is not a language HolyArrs knows", c)
		}
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	if len(out) > maxTrackLangs {
		return nil, fmt.Errorf("pick up to %d languages", maxTrackLangs)
	}
	return out, nil
}

// SaveTrackSettings keeps the owner's default track choice.
func SaveTrackSettings(ctx context.Context, s *database.Store, in TrackSettings, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		on := "off"
		if in.Enabled {
			on = "on"
		}
		for k, v := range map[string]string{tracksKey: on, tracksAudioKey: strings.Join(in.Audio, ","), tracksSubsKey: strings.Join(in.Subtitles, ",")} {
			if err := database.PutSetting(tx, watchGroup, k, v, by, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// track is one sound or subtitle track of a file.
type track struct {
	ID       string
	Lang     string // HolyArrs code, "" when unknown
	Forced   bool
	Selected bool
}

// pickTrack is the track a file should start with for want, in order: the
// first language the file has (a full track before a forced-only one).
// "off" gives "0", Plex's "no subtitles". ok is false when nothing in want
// is in the file, so the file is left as it is.
func pickTrack(langs TrackLanguages, tracks []track, want []string) (id string, ok bool) {
	for _, w := range want {
		if w == SubtitlesOff {
			return "0", true
		}
		var forced string
		for _, t := range tracks {
			if !langs.match(w, t.Lang) {
				continue
			}
			if !t.Forced {
				return t.ID, true
			}
			if forced == "" {
				forced = t.ID
			}
		}
		if forced != "" {
			return forced, true
		}
	}
	return "", false
}

// selectedTrack is the id of the track the file starts with now ("0": none).
func selectedTrack(tracks []track) string {
	for _, t := range tracks {
		if t.Selected {
			return t.ID
		}
	}
	return "0"
}

// plexPart is one file of a Plex item with its tracks.
type plexPart struct {
	ID          string
	Audio, Subs []track
}

// plexParts reads the files and tracks of one Plex movie or episode.
func (l *Linker) plexParts(ctx context.Context, m domain.MediaServer, ratingKey string) ([]plexPart, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/library/metadata/"+url.PathEscape(ratingKey), nil)
	if err != nil {
		return nil, err
	}
	return parsePlexParts(l.Langs, b)
}

func parsePlexParts(langs TrackLanguages, b []byte) ([]plexPart, error) {
	type stream struct {
		ID           string `xml:"id,attr"`
		StreamType   string `xml:"streamType,attr"`
		LanguageCode string `xml:"languageCode,attr"`
		LanguageTag  string `xml:"languageTag,attr"`
		Forced       string `xml:"forced,attr"`
		Selected     string `xml:"selected,attr"`
	}
	var mc struct {
		Videos []struct {
			Media []struct {
				Parts []struct {
					ID      string   `xml:"id,attr"`
					Streams []stream `xml:"Stream"`
				} `xml:"Part"`
			} `xml:"Media"`
		} `xml:"Video"`
	}
	if err := xml.Unmarshal(b, &mc); err != nil {
		return nil, errors.New("the server's answer was not a Plex item")
	}
	var out []plexPart
	for _, v := range mc.Videos {
		for _, md := range v.Media {
			for _, p := range md.Parts {
				if p.ID == "" {
					continue
				}
				pp := plexPart{ID: p.ID}
				for _, s := range p.Streams {
					lang := ""
					if s.LanguageTag != "" {
						lang = langs.norm(s.LanguageTag)
					}
					if lang == "" && s.LanguageCode != "" {
						lang = langs.norm(s.LanguageCode)
					}
					t := track{ID: s.ID, Lang: lang, Forced: s.Forced == "1", Selected: s.Selected == "1"}
					switch s.StreamType {
					case "2":
						pp.Audio = append(pp.Audio, t)
					case "3":
						pp.Subs = append(pp.Subs, t)
					}
				}
				out = append(out, pp)
			}
		}
	}
	return out, nil
}

// plexLeaves is the episode keys of one Plex show.
func (l *Linker) plexLeaves(ctx context.Context, m domain.MediaServer, showKey string) ([]string, error) {
	var keys []string
	for start := 0; ; start += watchPage {
		b, err := l.do(ctx, m, http.MethodGet, "/library/metadata/"+url.PathEscape(showKey)+"/allLeaves?"+strings.TrimPrefix(plexRange(start), "&"), nil)
		if err != nil {
			return nil, err
		}
		var mc struct {
			Videos []struct {
				RatingKey string `xml:"ratingKey,attr"`
			} `xml:"Video"`
		}
		if err := xml.Unmarshal(b, &mc); err != nil {
			return nil, errors.New("the server's answer was not a Plex episode list")
		}
		for _, v := range mc.Videos {
			if v.RatingKey != "" {
				keys = append(keys, v.RatingKey)
			}
		}
		if len(mc.Videos) < watchPage {
			return keys, nil
		}
	}
}

// setPlexPart picks a file's tracks; empty ids are left as they are.
func (l *Linker) setPlexPart(ctx context.Context, m domain.MediaServer, partID, audio, subs string) error {
	q := url.Values{}
	q.Set("allParts", "1")
	if audio != "" {
		q.Set("audioStreamID", audio)
	}
	if subs != "" {
		q.Set("subtitleStreamID", subs)
	}
	_, err := l.do(ctx, m, http.MethodPut, "/library/parts/"+url.PathEscape(partID)+"?"+q.Encode(), nil)
	return err
}

// tracksPlex sets the tracks of every Library file on one Plex server that
// was not set for these lists yet. It answers how many files changed.
func (l *Linker) tracksPlex(ctx context.Context, m domain.MediaServer, set TrackSettings) (int, error) {
	done, err := tracksDone(ctx, l.Store.Read(), m.ID)
	if err != nil {
		return 0, err
	}
	keys, err := mediafiles.TitlesBySourceID(ctx, l.Store.Read())
	if err != nil {
		return 0, err
	}
	var items []plexItem
	if err := l.plexItems(ctx, m, keys, func(_ string, it plexItem, _ int64) { items = append(items, it) }); err != nil {
		return 0, err
	}
	choice := set.choice()
	n := 0
	doItem := func(key string) error {
		parts, err := l.plexParts(ctx, m, key)
		if err != nil {
			return err
		}
		for _, p := range parts {
			if done[p.ID] == choice {
				continue
			}
			audio, subs := "", ""
			if id, ok := pickTrack(l.Langs, p.Audio, set.Audio); ok && id != selectedTrack(p.Audio) {
				audio = id
			}
			if id, ok := pickTrack(l.Langs, p.Subs, set.Subtitles); ok && id != selectedTrack(p.Subs) {
				subs = id
			}
			if audio != "" || subs != "" {
				if err := l.setPlexPart(ctx, m, p.ID, audio, subs); err != nil {
					return err
				}
				n++
			}
			if err := markTracks(ctx, l.Store, m.ID, p.ID, choice); err != nil {
				return err
			}
		}
		return nil
	}
	for _, it := range items {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		if it.typ != "2" {
			if err := doItem(it.ratingKey); err != nil {
				return n, err
			}
			continue
		}
		eps, err := l.plexLeaves(ctx, m, it.ratingKey)
		if err != nil {
			return n, err
		}
		for _, k := range eps {
			if err := doItem(k); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// jellyGive works out a person's new language settings: a language is given
// only when the person has none, or still has the one HolyArrs gave before.
// It answers false when nothing changes.
func jellyGive(cfg map[string]any, audio, subs, prevAudio, prevSubs string) bool {
	str := func(k string) string { s, _ := cfg[k].(string); return s }
	changed := false
	if audio != "" {
		if have := str("AudioLanguagePreference"); (have == "" || have == prevAudio) && have != audio {
			cfg["AudioLanguagePreference"] = audio
			cfg["PlayDefaultAudioTrack"] = false
			changed = true
		}
	}
	if subs != "" {
		modeOf := func(s string) string {
			if s == SubtitlesOff {
				return "None"
			}
			return "Always"
		}
		lang, mode := str("SubtitleLanguagePreference"), str("SubtitleMode")
		prevLang := prevSubs
		if prevSubs == SubtitlesOff {
			prevLang = ""
		}
		// Untouched: no language of their own and the server's own mode, or
		// exactly what HolyArrs gave last time.
		own := (lang == "" && (mode == "" || mode == "Default")) ||
			(prevSubs != "" && lang == prevLang && mode == modeOf(prevSubs))
		if own {
			wantLang := lang
			if subs != SubtitlesOff {
				wantLang = subs
			}
			if lang != wantLang || mode != modeOf(subs) {
				cfg["SubtitleLanguagePreference"] = wantLang
				cfg["SubtitleMode"] = modeOf(subs)
				changed = true
			}
		}
	}
	return changed
}

// firstLang is a list's first entry as Jellyfin keeps it ("off" stays).
func firstLang(langs TrackLanguages, list []string) string {
	if len(list) == 0 {
		return ""
	}
	if list[0] == SubtitlesOff {
		return SubtitlesOff
	}
	return langs.three(list[0])
}

// tracksJelly gives the first language of each list to the people on one
// Jellyfin or Emby server who did not pick their own.
func (l *Linker) tracksJelly(ctx context.Context, m domain.MediaServer, set TrackSettings, prev string) (int, error) {
	audio, subs := firstLang(l.Langs, set.Audio), firstLang(l.Langs, set.Subtitles)
	pa, ps, _ := strings.Cut(prev, "|")
	ids, err := l.userIDs(ctx, m)
	if err != nil {
		return 0, err
	}
	n := 0
	for id := range ids {
		b, err := l.do(ctx, m, http.MethodGet, "/Users/"+url.PathEscape(id), nil)
		if err != nil {
			return n, err
		}
		var u struct {
			Configuration map[string]any `json:"Configuration"`
		}
		if err := json.Unmarshal(b, &u); err != nil || u.Configuration == nil {
			return n, errors.New("the server's answer was not a person's settings")
		}
		if !jellyGive(u.Configuration, audio, subs, pa, ps) {
			continue
		}
		body, err := json.Marshal(u.Configuration)
		if err != nil {
			return n, err
		}
		if _, err := l.do(ctx, m, http.MethodPost, "/Users/"+url.PathEscape(id)+"/Configuration", body); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// SetDefaultTracks sets the default tracks on every switched-on Plex,
// Jellyfin and Emby server. It answers how many files and people changed.
func (l *Linker) SetDefaultTracks(ctx context.Context) (int, error) {
	l.tracking.Lock()
	defer l.tracking.Unlock()
	set := ReadTrackSettings(ctx, l.Store.Read())
	if !set.Enabled || (len(set.Audio) == 0 && len(set.Subtitles) == 0) {
		return 0, nil
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return 0, err
	}
	prev := database.SettingOr(ctx, l.Store.Read(), watchGroup, tracksJellyKey, "")
	given := firstLang(l.Langs, set.Audio) + "|" + firstLang(l.Langs, set.Subtitles)
	n := 0
	var errs []error
	jellyOK := true
	for _, m := range ms {
		if m.TokenSecretID == nil {
			continue
		}
		var got int
		var err error
		switch m.Kind {
		case "plex":
			got, err = l.tracksPlex(ctx, m, set)
		case "jellyfin", "emby":
			got, err = l.tracksJelly(ctx, m, set, prev)
			if err != nil {
				jellyOK = false
			}
		default:
			continue
		}
		n += got
		if err != nil {
			if ctx.Err() != nil {
				return n, ctx.Err()
			}
			log.Printf("media servers: %s: default tracks: %v", m.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", m.Name, err))
		}
	}
	if jellyOK && given != prev {
		if err := l.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			return database.PutSetting(tx, watchGroup, tracksJellyKey, given, domain.Actor{}, time.Now().UTC())
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return n, errors.Join(errs...)
}

// SetDefaultTracksSoon sets the default tracks now, in the background.
func (l *Linker) SetDefaultTracksSoon() {
	if l.Paused {
		return
	}
	l.mu.Lock()
	if l.stopTracks != nil {
		l.stopTracks()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	l.stopTracks = cancel
	l.wg.Add(1)
	l.mu.Unlock()
	go func() {
		defer l.wg.Done()
		defer cancel()
		jobs.Guard("media_servers", func() {
			if _, err := l.SetDefaultTracks(ctx); err != nil && ctx.Err() == nil {
				log.Printf("media servers: default tracks: %v", err)
			}
		})
	}()
}

// TracksPass is the scheduled pass: new files, and people who joined.
func (l *Linker) TracksPass(ctx context.Context) error {
	_, err := l.SetDefaultTracks(ctx)
	return err
}
