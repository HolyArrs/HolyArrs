package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Album choices: which albums are kept up to date when an artist is added
// (design/domain/music.md section 2).
const (
	AlbumsAll     = "all"
	AlbumsFuture  = "future"
	AlbumsMissing = "missing"
	AlbumsLatest  = "latest"
	AlbumsFirst   = "first"
	AlbumsNone    = "none"
)

// Pressing rules: how the Pressing of an album is chosen (section 4).
const (
	PressingCommon     = "common"
	PressingMostTracks = "most_tracks"
	PressingEarliest   = "earliest"
)

// Album kinds and Pressing statuses an artist can include.
var (
	MainAlbumKinds  = []string{"album", "ep", "single", "broadcast", "other"}
	ExtraAlbumKinds = []string{"studio", "compilation", "soundtrack", "spokenword", "interview", "audiobook", "audio drama", "live",
		"remix", "dj-mix", "mixtape/street", "demo", "field recording"}
	PressingStatuses = []string{"official", "promotion", "bootleg", "pseudo-release"}
)

// Errors about music.
var (
	ErrBadAlbumChoice    = errors.New("albums must be all, future, missing, latest, first or none")
	ErrBadNewAlbums      = errors.New("new_albums must be all, new or none")
	ErrBadAlbumKinds     = errors.New("album_types must list at least one of album, ep, single, broadcast, other")
	ErrBadExtraKinds     = errors.New("secondary_types must list at least one of studio, compilation, soundtrack, spokenword, interview, audiobook, audio drama, live, remix, dj-mix, mixtape/street, demo, field recording")
	ErrBadStatuses       = errors.New("release_statuses must list at least one of official, promotion, bootleg, pseudo-release")
	ErrBadPressingRule   = errors.New("pressing_rule must be common, most_tracks or earliest")
	ErrBadMusicURL       = errors.New("musicbrainz_url must be empty or an http or https address with no user, query or fragment")
	ErrBadCacheHours     = errors.New("cache_hours must be between 1 and 720")
	ErrNotArtist         = errors.New("that Title is not an artist")
	ErrNoMusicFolder     = errors.New("no music Library folder")
	ErrNotMusicFolder    = errors.New("that Library folder is not for music")
	ErrNoMusicSource     = errors.New("music details (MusicBrainz) are not set up")
	ErrArtistNeverAdd    = errors.New("this artist is on the Never-add list")
	ErrNoSuchPressing    = errors.New("the album has no such Pressing")
	ErrAlbumNotIncluded  = errors.New("the album is not included by the artist's album kinds; change the kinds first")
	ErrBadArtistID       = errors.New(`send a MusicBrainz artist id as "mbid:<id>"`)
	ErrNotAlbum          = errors.New("no such album")
	errUnknownMusicGroup = errors.New("unknown music setting")
)

// ValidAlbumChoice reports whether c is an album choice.
func ValidAlbumChoice(c string) bool {
	return slices.Contains([]string{AlbumsAll, AlbumsFuture, AlbumsMissing, AlbumsLatest, AlbumsFirst, AlbumsNone}, c)
}

// newAlbumsPolicy maps the new_albums choice to the Title's new_parts_policy.
func newAlbumsPolicy(v string) (string, bool) {
	switch v {
	case "all":
		return "all", true
	case "new":
		return "future_only", true
	case "none":
		return "none", true
	}
	return "", false
}

// NewAlbumsOf is the new_albums word for a Title's new_parts_policy.
func NewAlbumsOf(policy string) string {
	if policy == "future_only" {
		return "new"
	}
	return policy
}

// MusicDefaults are the music settings: What to pick for new artists, and
// Links for where MusicBrainz is reached.
type MusicDefaults struct {
	AlbumChoice     string   `json:"album_choice"`     // all, future, missing, latest, first, none
	NewAlbums       string   `json:"new_albums"`       // all, new, none
	AlbumTypes      []string `json:"album_types"`      // main album kinds included
	SecondaryTypes  []string `json:"secondary_types"`  // extra album kinds included; studio = none
	ReleaseStatuses []string `json:"release_statuses"` // Pressing statuses that count
	PressingRule    string   `json:"pressing_rule"`    // common, most_tracks, earliest
	MusicBrainzURL  string   `json:"musicbrainz_url"`  // "" = https://musicbrainz.org
	CacheHours      int      `json:"cache_hours"`      // how long MusicBrainz answers are kept
	// AllowLocal lets musicbrainz_url point at this machine or the local
	// network (a mirror at home); off, such addresses are refused.
	AllowLocal bool `json:"musicbrainz_allow_local_network"`
}

// musicKeys are the settings rows: group and key per field.
var musicKeys = map[string][2]string{
	"album_choice": {"what_to_pick", "music_album_choice"}, "new_albums": {"what_to_pick", "music_new_albums"},
	"album_types": {"what_to_pick", "music_album_types"}, "secondary_types": {"what_to_pick", "music_secondary_types"},
	"release_statuses": {"what_to_pick", "music_release_statuses"}, "pressing_rule": {"what_to_pick", "music_pressing_rule"},
	"musicbrainz_url": {"links", "musicbrainz_url"}, "cache_hours": {"links", "musicbrainz_cache_hours"},
	"allow_local": {"links", "musicbrainz_allow_local"},
}

// DefaultMusic are the recommended music settings (Lidarr's standard
// filter with EP added).
func DefaultMusic() MusicDefaults {
	return MusicDefaults{AlbumChoice: AlbumsAll, NewAlbums: "all", AlbumTypes: []string{"album", "ep"}, SecondaryTypes: []string{"studio"},
		ReleaseStatuses: []string{"official"}, PressingRule: PressingCommon, CacheHours: 24}
}

// cleanList lowercases, trims and de-duplicates v, keeping only words in
// allowed; ok is false when a word is not allowed or nothing is left.
func cleanList(v []string, allowed []string) ([]string, bool) {
	out := []string{}
	for _, w := range v {
		w = strings.ToLower(strings.TrimSpace(w))
		if !slices.Contains(allowed, w) {
			return nil, false
		}
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out, len(out) > 0
}

// ReadMusicDefaults reads the music settings; unset or odd values take the
// recommended defaults.
func ReadMusicDefaults(ctx context.Context, db database.Lister) (MusicDefaults, error) {
	d := DefaultMusic()
	rows, err := db.QueryContext(ctx, `SELECT grp, key, value FROM settings WHERE (grp = 'what_to_pick' AND key LIKE 'music\_%' ESCAPE '\')
		OR (grp = 'links' AND key IN ('musicbrainz_url', 'musicbrainz_cache_hours', 'musicbrainz_allow_local'))`)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var g, k, v string
		if err := rows.Scan(&g, &k, &v); err != nil {
			return d, err
		}
		var list []string
		_ = json.Unmarshal([]byte(v), &list)
		switch k {
		case "music_album_choice":
			if ValidAlbumChoice(v) {
				d.AlbumChoice = v
			}
		case "music_new_albums":
			if _, ok := newAlbumsPolicy(v); ok {
				d.NewAlbums = v
			}
		case "music_album_types":
			if l, ok := cleanList(list, MainAlbumKinds); ok {
				d.AlbumTypes = l
			}
		case "music_secondary_types":
			if l, ok := cleanList(list, ExtraAlbumKinds); ok {
				d.SecondaryTypes = l
			}
		case "music_release_statuses":
			if l, ok := cleanList(list, PressingStatuses); ok {
				d.ReleaseStatuses = l
			}
		case "music_pressing_rule":
			if validPressingRule(v) {
				d.PressingRule = v
			}
		case "musicbrainz_url":
			if validMusicURL(v) {
				d.MusicBrainzURL = strings.TrimRight(strings.TrimSpace(v), "/")
			}
		case "musicbrainz_allow_local":
			d.AllowLocal = v == "true"
		case "musicbrainz_cache_hours":
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 720 {
				d.CacheHours = n
			}
		}
	}
	return d, rows.Err()
}

func validPressingRule(r string) bool {
	return r == PressingCommon || r == PressingMostTracks || r == PressingEarliest
}

func validMusicURL(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	u, err := url.Parse(v)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

// Check validates d and cleans its lists.
func (d *MusicDefaults) Check() error {
	var ok bool
	switch {
	case !ValidAlbumChoice(d.AlbumChoice):
		return ErrBadAlbumChoice
	case func() bool { _, ok := newAlbumsPolicy(d.NewAlbums); return !ok }():
		return ErrBadNewAlbums
	case !validPressingRule(d.PressingRule):
		return ErrBadPressingRule
	case !validMusicURL(d.MusicBrainzURL):
		return ErrBadMusicURL
	case d.CacheHours < 1 || d.CacheHours > 720:
		return ErrBadCacheHours
	}
	if d.AlbumTypes, ok = cleanList(d.AlbumTypes, MainAlbumKinds); !ok {
		return ErrBadAlbumKinds
	}
	if d.SecondaryTypes, ok = cleanList(d.SecondaryTypes, ExtraAlbumKinds); !ok {
		return ErrBadExtraKinds
	}
	if d.ReleaseStatuses, ok = cleanList(d.ReleaseStatuses, PressingStatuses); !ok {
		return ErrBadStatuses
	}
	d.MusicBrainzURL = strings.TrimRight(strings.TrimSpace(d.MusicBrainzURL), "/")
	return nil
}

// SetMusicDefaults stores the music settings.
func SetMusicDefaults(ctx context.Context, s *database.Store, d MusicDefaults, by domain.Actor) error {
	if err := d.Check(); err != nil {
		return err
	}
	enc := func(v []string) string { b, _ := json.Marshal(v); return string(b) }
	vals := map[string]string{"album_choice": d.AlbumChoice, "new_albums": d.NewAlbums, "album_types": enc(d.AlbumTypes),
		"secondary_types": enc(d.SecondaryTypes), "release_statuses": enc(d.ReleaseStatuses), "pressing_rule": d.PressingRule,
		"musicbrainz_url": d.MusicBrainzURL, "cache_hours": strconv.Itoa(d.CacheHours), "allow_local": strconv.FormatBool(d.AllowLocal)}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now())
		for f, v := range vals {
			gk, ok := musicKeys[f]
			if !ok {
				return errUnknownMusicGroup
			}
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				gk[0], gk[1], v, now, ActorText(by), now, ActorText(by))
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// musicDate reads a MusicBrainz date as UTC midnight: YYYY-MM-DD that day,
// YYYY-MM the last day of the month and YYYY 31 December, so an album
// known only by month or year opens at the end of it; nil when empty or odd.
func musicDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if len(s) == len(layout) {
			if t, err := time.Parse(layout, s); err == nil {
				t = t.UTC()
				switch layout {
				case "2006-01":
					t = t.AddDate(0, 1, -1)
				case "2006":
					t = t.AddDate(1, 0, -1)
				}
				return &t
			}
		}
	}
	return nil
}

// musicDatePrecision says how exact a MusicBrainz date is: day, month or
// year; nil when musicDate reads none.
func musicDatePrecision(s string) *string {
	if musicDate(s) == nil {
		return nil
	}
	p := map[int]string{10: "day", 7: "month", 4: "year"}[len(strings.TrimSpace(s))]
	return &p
}

// AlbumKindsOK reports whether an album's main and extra kinds pass an
// artist's album kinds: the main kind is included (an album with none
// counts as other), and every extra kind is included (an album with none
// needs studio).
func AlbumKindsOK(t mediafiles.Title, primary string, secondary []string) bool {
	if primary == "" {
		primary = "other"
	}
	if !slices.Contains(t.MusicTypes, primary) {
		return false
	}
	if len(secondary) == 0 {
		return slices.Contains(t.MusicSecondary, "studio")
	}
	for _, s := range secondary {
		if !slices.Contains(t.MusicSecondary, s) {
			return false
		}
	}
	return true
}

// ChoosePressing picks the Pressing of an album by rule among ps, counting
// only statuses (all of ps when none has one of them). -1 when ps is empty.
func ChoosePressing(ps []domain.AlbumPressing, statuses []string, rule string) int {
	idx := []int{}
	for i, p := range ps {
		if slices.Contains(statuses, p.Status) {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		for i := range ps {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return -1
	}
	digital := func(p domain.AlbumPressing) bool {
		for _, f := range p.Formats {
			if strings.EqualFold(f, "Digital Media") {
				return true
			}
		}
		return false
	}
	date := func(p domain.AlbumPressing) string {
		if p.Date == "" {
			return "9999"
		}
		return p.Date
	}
	common := map[int]int{}
	for _, i := range idx {
		common[ps[i].TrackCount]++
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := ps[idx[a]], ps[idx[b]]
		switch rule {
		case PressingMostTracks:
			if x.TrackCount != y.TrackCount {
				return x.TrackCount > y.TrackCount
			}
		case PressingEarliest:
			if date(x) != date(y) {
				return date(x) < date(y)
			}
		default:
			if common[x.TrackCount] != common[y.TrackCount] {
				return common[x.TrackCount] > common[y.TrackCount]
			}
		}
		if digital(x) != digital(y) {
			return digital(x)
		}
		if date(x) != date(y) {
			return date(x) < date(y)
		}
		return x.MBID < y.MBID
	})
	return idx[0]
}

// albumChoiceKeep says which of an artist's included albums start kept up
// to date for an album choice at add time. dates are release dates (nil
// when unknown); haveFiles marks albums that already hold track Files.
func albumChoiceKeep(choice string, ids []string, dates map[string]*time.Time, haveFiles map[string]bool, now time.Time) map[string]bool {
	keep := map[string]bool{}
	switch choice {
	case AlbumsAll:
		for _, id := range ids {
			keep[id] = true
		}
	case AlbumsMissing:
		for _, id := range ids {
			keep[id] = !haveFiles[id]
		}
	case AlbumsFuture:
		for _, id := range ids {
			keep[id] = dates[id] == nil || dates[id].After(now)
		}
	case AlbumsLatest, AlbumsFirst:
		pick := ""
		var at time.Time
		out := false // latest: whether pick is out
		for _, id := range ids {
			d := dates[id]
			if d == nil {
				continue
			}
			isOut := !d.After(now)
			switch {
			case pick == "":
			case choice == AlbumsFirst:
				if !d.Before(at) {
					continue
				}
			case out && !isOut:
				continue // an out album beats one still to come
			case isOut == out && !d.After(at):
				continue
			}
			pick, at, out = id, *d, isOut
		}
		if pick != "" {
			keep[pick] = true
		}
	}
	return keep
}

// AlbumGate says whether an album released at release is out at now, and
// why in one phrase. An album with no release date is never out.
func AlbumGate(release *time.Time, now time.Time) (open bool, reason string) {
	if release == nil {
		return false, "no release date yet"
	}
	day := release.UTC().Format("2006-01-02")
	if now.Before(*release) {
		return false, "out on " + day
	}
	return true, "out since " + day
}

// trackGatesTx moves an artist's track Slots across the release-date gate:
// not_out_yet to wanted once the album is out, and wanted back to
// not_out_yet when the date moved later. titleID 0 means every artist.
func trackGatesTx(ctx context.Context, tx *sql.Tx, titleID int64, now time.Time, by domain.Actor) (opened, closed []int64, err error) {
	q := `SELECT s.id, s.state, i.air_date, COALESCE(s.last_reason, '') FROM slots s JOIN items i ON i.id = s.item_id
		WHERE i.kind = 'track' AND s.state IN ('not_out_yet', 'wanted')`
	args := []any{}
	if titleID != 0 {
		q += " AND i.title_id = ?"
		args = append(args, titleID)
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	type row struct {
		id     int64
		state  string
		air    *time.Time
		reason string
	}
	var rs []row
	for rows.Next() {
		var r row
		var air sql.NullString
		if err := rows.Scan(&r.id, &r.state, &air, &r.reason); err != nil {
			rows.Close()
			return nil, nil, err
		}
		if air.Valid {
			if t, err := time.Parse(database.TimeLayout, air.String); err == nil {
				r.air = &t
			}
		}
		rs = append(rs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for _, r := range rs {
		open, reason := AlbumGate(r.air, now)
		switch {
		case open && r.state == string(mediafiles.SlotNotOutYet):
			if err := mediafiles.MoveSlotTx(tx, r.id, mediafiles.SlotWanted, reason, by); err != nil {
				return nil, nil, err
			}
			opened = append(opened, r.id)
		case !open && r.state == string(mediafiles.SlotWanted):
			if err := mediafiles.MoveSlotTx(tx, r.id, mediafiles.SlotNotOutYet, reason, by); err != nil {
				return nil, nil, err
			}
			closed = append(closed, r.id)
		case r.state == string(mediafiles.SlotNotOutYet) && r.reason != reason:
			if _, err := tx.Exec("UPDATE slots SET last_reason = ? WHERE id = ?", reason, r.id); err != nil {
				return nil, nil, err
			}
		}
	}
	return opened, closed, nil
}

// OpenTrackGates moves track Slots whose album is now out from not_out_yet
// to wanted. It reads first and writes only when a Slot crosses.
func (s *Service) OpenTrackGates(ctx context.Context) (int, error) {
	now := s.now()
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT DISTINCT i.title_id FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE i.kind = 'track' AND s.state = 'not_out_yet' AND t.state != 'removed' AND i.air_date IS NOT NULL AND i.air_date <= ?`, database.FormatTime(now))
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		var t mediafiles.Title
		var opened []int64
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			if err := database.Get(ctx, tx, "titles", id, &t); err != nil {
				return err
			}
			var err error
			opened, _, err = trackGatesTx(ctx, tx, id, now, domain.System)
			return err
		})
		if err != nil {
			return n, err
		}
		n += len(opened)
		s.trackGateEvents(ctx, t, opened, nil, domain.System, nil)
	}
	return n, nil
}

// trackGateEvents records each album whose track Slots crossed the gate:
// one Event per album, not one per track.
func (s *Service) trackGateEvents(ctx context.Context, t mediafiles.Title, opened, closed []int64, by domain.Actor, jobID *int64) {
	for _, list := range []struct {
		ids  []int64
		kind string
		to   mediafiles.SlotState
		says string
	}{{opened, domain.EventGateOpened, mediafiles.SlotWanted, " is out: its tracks are wanted now"}, {closed, domain.EventGateClosed, mediafiles.SlotNotOutYet, " is not out yet: its release date moved"}} {
		if len(list.ids) == 0 {
			continue
		}
		byAlbum := map[string][]int64{}
		var order []string
		for _, id := range list.ids {
			var album string
			_ = s.Store.Read().QueryRowContext(ctx, `SELECT p.name FROM slots sl JOIN items i ON i.id = sl.item_id JOIN parts p ON p.id = i.part_id WHERE sl.id = ?`, id).Scan(&album)
			if _, ok := byAlbum[album]; !ok {
				order = append(order, album)
			}
			byAlbum[album] = append(byAlbum[album], id)
			s.Publish("slot", map[string]any{"id": id, "state": list.to})
		}
		for _, album := range order {
			ids := byAlbum[album]
			s.Emit(ctx, &domain.Event{Actor: by, Kind: list.kind, TitleID: &t.ID, JobID: jobID, Summary: fmt.Sprintf("%s: %s%s", t.Name, album, list.says),
				Data: map[string]any{"album": album, "slot_ids": ids, "tracks": len(ids)}, GroupKey: GroupKey(t.ID)})
		}
	}
}
