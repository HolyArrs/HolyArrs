package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// KindRefreshArtists is the 12-hour artist refresh, or every artist by hand.
const KindRefreshArtists = "refresh_artists"

type useCacheKey struct{}

// useCache lets a refresh read the lookup cache (the refresh right after an
// add: the add just fetched the same answers).
func useCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, useCacheKey{}, true)
}

// ArtistRefresh says what one artist details refresh did.
type ArtistRefresh struct {
	Dropped bool `json:"dropped,omitempty"`
	ArtistSync
}

// RefreshArtist fetches an artist and its albums from MusicBrainz, then
// the Pressings and tracks of every included album, and brings the rows
// and Slots in line. An artist MusicBrainz no longer knows is flagged
// (dropped_at), never deleted.
func (s *Service) RefreshArtist(ctx context.Context, titleID int64, by domain.Actor, jobID *int64) (ArtistRefresh, error) {
	var out ArtistRefresh
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return out, err
	}
	if t.Kind != "artist" {
		return out, ErrNotArtist
	}
	p, err := s.artists()
	if err != nil {
		return out, err
	}
	defs, err := ReadMusicDefaults(ctx, s.Store.Read())
	if err != nil {
		return out, err
	}
	lctx := ctx
	if ctx.Value(useCacheKey{}) == nil {
		lctx = metadata.NoCache(ctx)
	}
	now := s.now()
	a, err := p.Artist(lctx, t.SourceIDs["musicbrainz"])
	if errors.Is(err, metadata.ErrArtistNotFound) {
		out.Dropped = true
		first := t.DroppedAt == nil
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET dropped_at = COALESCE(dropped_at, ?), refreshed_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				database.FormatTime(now), database.FormatTime(now), database.FormatTime(now), ActorText(by), titleID)
			return err
		})
		if err == nil && first {
			s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleDropped, TitleID: &titleID, JobID: jobID,
				Summary: t.Name + " is no longer known to MusicBrainz; kept and flagged", GroupKey: GroupKey(titleID)})
		}
		return out, err
	}
	if err != nil {
		return out, err
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		applyArtist(&t, a)
		t.DroppedAt, t.RefreshedAt = nil, &now
		if err := database.Update(tx, "titles", &t, by); err != nil {
			return err
		}
		var err error
		out.ArtistSync, err = syncAlbumsTx(ctx, tx, t, a, "", now, by)
		return err
	})
	if err != nil {
		return out, err
	}
	if len(t.Artwork) == 0 {
		s.artistPicture(ctx, titleID, t.SourceIDs["musicbrainz"], by)
	} else if s.artworkHasStandIn(t.Artwork) {
		// A stand-in saved earlier (a grey person, one flat colour) is swapped
		// for the server's photo now, or removed when it has none.
		if _, err := s.healArtistArt(ctx, titleID, t.Name, t.SourceIDs["musicbrainz"], t.Artwork, by); err != nil {
			log.Printf("library: stand-in picture for artist %d: %v", titleID, err)
		}
	}
	albums, err := database.Select[mediafiles.Part](ctx, s.Store.Read(), "parts", "title_id = ? AND kind = 'album' AND included = 1 AND dropped_at IS NULL ORDER BY release_date, id", titleID)
	if err != nil {
		return out, err
	}
	for _, al := range albums {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		ps, err := p.Pressings(lctx, al.SourceIDs["musicbrainz"])
		cut := errors.Is(err, metadata.ErrListCut)
		switch {
		case errors.Is(err, metadata.ErrArtistNotFound), err == nil && len(ps) == 0:
			// Gone since the album list was read, or no Pressings listed:
			// leave the album as it is; the album list decides whether it goes.
			continue
		case err != nil && !cut:
			return out, fmt.Errorf("album %s: %w", al.Name, err)
		}
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			res, err := syncPressingsTx(ctx, tx, t, al.ID, ps, cut, defs.PressingRule, now, by)
			out.add(res)
			return err
		})
		if err != nil {
			return out, err
		}
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		out.Opened, out.Closed, err = trackGatesTx(ctx, tx, titleID, now, by)
		return err
	})
	if err != nil {
		return out, err
	}
	data := map[string]any{"albums_added": out.AlbumsAdded, "albums_changed": out.AlbumsChanged, "albums_removed": out.AlbumsRemoved,
		"albums_dropped": out.AlbumsDropped, "pressings_chosen": out.PressingsChosen, "tracks_added": out.TracksAdded,
		"tracks_changed": out.TracksChanged, "tracks_removed": out.TracksRemoved, "tracks_dropped": out.TracksDropped}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleRefreshed, TitleID: &titleID, JobID: jobID,
		Summary: "Refreshed details of " + t.Name + " from MusicBrainz", Data: data, GroupKey: GroupKey(titleID)})
	if out.AlbumsAdded+out.AlbumsRemoved+out.AlbumsDropped+out.TracksAdded+out.TracksRemoved+out.TracksDropped > 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAlbumsChanged, TitleID: &titleID, JobID: jobID, GroupKey: GroupKey(titleID), Data: data,
			Summary: fmt.Sprintf("%s: %d new albums, %d removed, %d no longer listed but kept; %d new tracks, %d removed, %d no longer listed but kept",
				t.Name, out.AlbumsAdded, out.AlbumsRemoved, out.AlbumsDropped, out.TracksAdded, out.TracksRemoved, out.TracksDropped)})
	}
	s.trackGateEvents(ctx, t, out.Opened, out.Closed, by, jobID)
	return out, nil
}

// Artist refresh rules (Lidarr's per-artist refresh rules, backend/jobs.md).
const (
	artistStale  = 30 * 24 * time.Hour // always refresh after this
	artistFresh  = 12 * time.Hour      // never refresh before this
	artistActive = 2 * 24 * time.Hour  // an active artist is refreshed after this
	artistRecent = 30 * 24 * time.Hour // an album out this recently means refresh
)

// ShouldRefreshArtist applies the artist refresh rules, in order: never
// fetched, yes; over 30 days since the last fetch, yes; under 12 hours, no;
// still active and over 2 days, yes; newest album out in the last 30 days,
// yes; else no.
func ShouldRefreshArtist(t mediafiles.Title, newest *time.Time, now time.Time) (yes bool, why string) {
	if t.RefreshedAt == nil {
		return true, "never fetched"
	}
	age := now.Sub(*t.RefreshedAt)
	switch {
	case age > artistStale:
		return true, "last fetch over 30 days ago"
	case age < artistFresh:
		return false, "last fetch under 12 hours ago"
	case t.Status != "ended" && age > artistActive:
		return true, "still active"
	case newest != nil && !newest.After(now) && now.Sub(*newest) <= artistRecent:
		return true, "an album came out in the last 30 days"
	}
	return false, "nothing new expected"
}

// ArtistRefreshPlan picks the artists a refresh-all run covers.
func (s *Service) ArtistRefreshPlan(ctx context.Context, byHand bool) (ids []int64, why map[int64]string, err error) {
	db := s.Store.Read()
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed' ORDER BY sort_name, id")
	if err != nil {
		return nil, nil, err
	}
	why = map[int64]string{}
	now := s.now()
	for _, t := range ts {
		if byHand {
			ids, why[t.ID] = append(ids, t.ID), "by hand"
			continue
		}
		var newest sql.NullString
		if err := db.QueryRowContext(ctx, "SELECT max(release_date) FROM parts WHERE title_id = ? AND kind = 'album' AND release_date <= ?",
			t.ID, database.FormatTime(now)).Scan(&newest); err != nil {
			return nil, nil, err
		}
		var at *time.Time
		if newest.Valid {
			if v, err := time.Parse(database.TimeLayout, newest.String); err == nil {
				at = &v
			}
		}
		if yes, w := ShouldRefreshArtist(t, at, now); yes {
			ids, why[t.ID] = append(ids, t.ID), w
		}
	}
	return ids, why, nil
}

// handleRefreshArtists refreshes the planned artists one by one. A failure
// on one artist is recorded and the run goes on, except when MusicBrainz
// stays busy: then the run stops and the next one tries again.
func (s *Service) handleRefreshArtists(ctx context.Context, r *jobs.Run) error {
	byHand, _ := r.Job.Params["by_hand"].(bool)
	ids, why, err := s.ArtistRefreshPlan(ctx, byHand)
	if err != nil {
		return err
	}
	var failed []string
	dropped, added := 0, 0
	for i, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = r.Progress(ctx, float64(i)/float64(len(ids)), fmt.Sprintf("refreshing artist %d of %d", i+1, len(ids)))
		out, err := s.RefreshArtist(ctx, id, JobActor(r), &r.Job.ID)
		if err != nil {
			if errors.Is(err, metadata.ErrMusicBusy) || errors.Is(err, ErrNoMusicSource) {
				return err
			}
			failed = append(failed, fmt.Sprintf("title %d: %v", id, err))
			continue
		}
		if out.Dropped {
			dropped++
		}
		added += out.AlbumsAdded
	}
	reasons := map[string]int{}
	for _, w := range why {
		reasons[w]++
	}
	r.SetResult(map[string]any{"refreshed": len(ids) - len(failed), "failed": failed, "dropped": dropped, "albums_added": added, "why": reasons})
	return nil
}

// SetAlbum switches Keep up to date for an album and its tracks, and/or
// chooses its Pressing by MusicBrainz id or row id (a person's choice,
// kept across refreshes); the tracks follow the new Pressing at once.
func (s *Service) SetAlbum(ctx context.Context, partID int64, keep *bool, pressingID int64, by domain.Actor) (mediafiles.Part, error) {
	var p mediafiles.Part
	var t mediafiles.Title
	var changed []string
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "parts", partID, &p); err != nil {
			return err
		}
		if p.Kind != "album" {
			return ErrNotAlbum
		}
		if err := database.Get(ctx, tx, "titles", p.TitleID, &t); err != nil {
			return err
		}
		now := s.now()
		if keep != nil && *keep != p.KeepUpToDate {
			if *keep && !p.Included {
				return ErrAlbumNotIncluded
			}
			p.KeepUpToDate = *keep
			if err := database.Update(tx, "parts", &p, by); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE items SET keep_up_to_date = ?, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'track'",
				*keep, database.FormatTime(now), ActorText(by), p.ID); err != nil {
				return err
			}
			changed = append(changed, "Keep up to date "+map[bool]string{true: "on", false: "off"}[*keep])
		}
		if pressingID != 0 {
			var pr domain.AlbumPressing
			if err := database.Get(ctx, tx, "pressings", pressingID, &pr); errors.Is(err, database.ErrNotFound) || (err == nil && pr.PartID != p.ID) {
				return ErrNoSuchPressing
			} else if err != nil {
				return err
			}
			if !pr.Chosen || pr.ChosenBy != "person" {
				if _, err := tx.Exec("UPDATE pressings SET chosen = 0 WHERE part_id = ?", p.ID); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE pressings SET chosen = 1, chosen_by = 'person', updated_at = ?, updated_by = ? WHERE id = ?",
					database.FormatTime(now), ActorText(by), pr.ID); err != nil {
					return err
				}
				if _, err := syncTracksTx(ctx, tx, t.ID, p, pr.Tracks, now, by); err != nil {
					return err
				}
				if _, _, err := trackGatesTx(ctx, tx, t.ID, now, by); err != nil {
					return err
				}
				changed = append(changed, "Pressing "+pressingLabel(pr))
			}
		}
		return nil
	})
	if err != nil || len(changed) == 0 {
		return p, err
	}
	data := map[string]any{"part_id": p.ID, "keep_up_to_date": p.KeepUpToDate}
	if pressingID != 0 {
		data["pressing_id"] = pressingID
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAlbumChanged, TitleID: &t.ID, Summary: fmt.Sprintf("%s: %s: %s", t.Name, p.Name, joinWords(changed)),
		Data: data, GroupKey: GroupKey(t.ID)})
	return p, nil
}

func joinWords(ws []string) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += "; "
		}
		out += w
	}
	return out
}

// pressingLabel names a Pressing for people: "First Light (2020, XW, Digital Media, 2 tracks)".
func pressingLabel(p domain.AlbumPressing) string {
	parts := []string{}
	if p.Date != "" {
		parts = append(parts, p.Date[:min(4, len(p.Date))])
	}
	if p.Country != "" {
		parts = append(parts, p.Country)
	}
	formats := []string{}
	for _, f := range p.Formats {
		if f != "" && !slices.Contains(formats, f) {
			formats = append(formats, f)
		}
	}
	if len(formats) > 0 {
		parts = append(parts, joinComma(formats))
	}
	parts = append(parts, fmt.Sprintf("%d tracks", p.TrackCount))
	return p.Name + " (" + joinComma(parts) + ")"
}

func joinComma(ws []string) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += ", "
		}
		out += w
	}
	return out
}

// ArtistChoices are an artist's choices a person can change after adding.
type ArtistChoices struct {
	NewAlbums       *string  `json:"new_albums"`
	AlbumTypes      []string `json:"album_types"`
	SecondaryTypes  []string `json:"secondary_types"`
	ReleaseStatuses []string `json:"release_statuses"`
}

// SetArtistChoices changes an artist's new-album choice and album kinds,
// then queues a details refresh so albums now included get their Pressings
// and tracks (and albums no longer included lose theirs).
func (s *Service) SetArtistChoices(ctx context.Context, titleID int64, c ArtistChoices, by domain.Actor) (mediafiles.Title, int64, error) {
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "artist" {
			return ErrNotArtist
		}
		var ok bool
		if c.NewAlbums != nil {
			if t.NewPartsPolicy, ok = newAlbumsPolicy(*c.NewAlbums); !ok {
				return ErrBadNewAlbums
			}
		}
		if c.AlbumTypes != nil {
			if t.MusicTypes, ok = cleanList(c.AlbumTypes, MainAlbumKinds); !ok {
				return ErrBadAlbumKinds
			}
		}
		if c.SecondaryTypes != nil {
			if t.MusicSecondary, ok = cleanList(c.SecondaryTypes, ExtraAlbumKinds); !ok {
				return ErrBadExtraKinds
			}
		}
		if c.ReleaseStatuses != nil {
			if t.MusicStatuses, ok = cleanList(c.ReleaseStatuses, PressingStatuses); !ok {
				return ErrBadStatuses
			}
		}
		return database.Update(tx, "titles", &t, by)
	})
	if err != nil {
		return t, 0, err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventArtistChanged, TitleID: &t.ID, Summary: t.Name + ": album choices changed",
		Data: map[string]any{"new_albums": NewAlbumsOf(t.NewPartsPolicy), "album_types": t.MusicTypes, "secondary_types": t.MusicSecondary,
			"release_statuses": t.MusicStatuses}, GroupKey: GroupKey(t.ID)})
	jobID, _, err := s.Queue.Enqueue(ctx, KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID}, map[string]any{"new": true},
		fmt.Sprintf("refresh_title:%d", t.ID), by)
	return t, jobID, err
}

// AlbumSummary is one album of an artist with its track counts.
type AlbumSummary struct {
	ID             int64      `json:"id"`
	MBID           string     `json:"mbid"`
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	SecondaryTypes []string   `json:"secondary_types"`
	Disambiguation string     `json:"disambiguation,omitempty"`
	ReleaseDate    *time.Time `json:"release_date,omitempty"`
	Included       bool       `json:"included"`
	KeepUpToDate   bool       `json:"keep_up_to_date"`
	Dropped        bool       `json:"dropped,omitempty"`
	Pressings      int        `json:"pressings"`
	Pressing       string     `json:"pressing,omitempty"` // the chosen Pressing, named
	PressingID     int64      `json:"pressing_id,omitempty"`
	Tracks         int        `json:"tracks"`
	InLibrary      int        `json:"in_library"`    // tracks with a file in the main Version
	Missing        int        `json:"missing"`       // out, kept up to date, no file yet
	BetterWanted   int        `json:"better_wanted"` // in the Library, a better copy is sought
	NotOutYet      int        `json:"not_out_yet"`
	State          string     `json:"state"` // not_included, not_out_yet, missing, partial, complete, no_tracks, not_kept
}

// Albums summarises an artist's albums (newest first) from their tracks
// and the main Version's Slots.
func Albums(ctx context.Context, db database.Lister, titleID int64) ([]AlbumSummary, error) {
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' ORDER BY release_date IS NULL, release_date DESC, id", titleID)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return []AlbumSummary{}, nil
	}
	// Three queries for the whole artist instead of two per album.
	ps, err := database.Select[domain.AlbumPressing](ctx, db, "pressings",
		"part_id IN (SELECT id FROM parts WHERE title_id = ? AND kind = 'album')", titleID)
	if err != nil {
		return nil, err
	}
	byPart := map[int64][]domain.AlbumPressing{}
	for _, pr := range ps {
		byPart[pr.PartID] = append(byPart[pr.PartID], pr)
	}
	tracks := map[int64][]trackState{}
	rows, err := db.QueryContext(ctx, `SELECT i.part_id, i.keep_up_to_date, COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.title_id = ? AND i.kind = 'track' AND i.part_id IS NOT NULL`, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var part int64
		var ts trackState
		if err := rows.Scan(&part, &ts.keep, &ts.state); err != nil {
			return nil, err
		}
		tracks[part] = append(tracks[part], ts)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]AlbumSummary, 0, len(parts))
	for _, p := range parts {
		out = append(out, summarise(p, byPart[p.ID], tracks[p.ID]))
	}
	return out, nil
}

type trackState struct {
	keep  bool
	state string
}

// AlbumOf summarises one album.
func AlbumOf(ctx context.Context, db database.Lister, p mediafiles.Part) (AlbumSummary, error) {
	return albumSummary(ctx, db, p)
}

func albumSummary(ctx context.Context, db database.Lister, p mediafiles.Part) (AlbumSummary, error) {
	ps, err := database.Select[domain.AlbumPressing](ctx, db, "pressings", "part_id = ?", p.ID)
	if err != nil {
		return AlbumSummary{}, err
	}
	rows, err := db.QueryContext(ctx, `SELECT i.keep_up_to_date, COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = i.title_id ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.part_id = ? AND i.kind = 'track'`, p.ID)
	if err != nil {
		return AlbumSummary{}, err
	}
	defer rows.Close()
	var ts []trackState
	for rows.Next() {
		var t trackState
		if err := rows.Scan(&t.keep, &t.state); err != nil {
			return AlbumSummary{}, err
		}
		ts = append(ts, t)
	}
	if err := rows.Err(); err != nil {
		return AlbumSummary{}, err
	}
	return summarise(p, ps, ts), nil
}

// summarise builds one album's summary from its pressings and track states.
func summarise(p mediafiles.Part, ps []domain.AlbumPressing, ts []trackState) AlbumSummary {
	a := AlbumSummary{ID: p.ID, MBID: p.SourceIDs["musicbrainz"], Name: p.Name, SecondaryTypes: p.SecondaryTypes, ReleaseDate: p.ReleaseDate,
		Included: p.Included, KeepUpToDate: p.KeepUpToDate, Dropped: p.DroppedAt != nil}
	if p.ReleaseType != nil {
		a.Type = *p.ReleaseType
	}
	if p.Disambiguation != nil {
		a.Disambiguation = *p.Disambiguation
	}
	if a.SecondaryTypes == nil {
		a.SecondaryTypes = []string{}
	}
	a.Pressings = len(ps)
	for _, pr := range ps {
		if pr.Chosen {
			a.Pressing, a.PressingID = pressingLabel(pr), pr.ID
		}
	}
	for _, t := range ts {
		a.Tracks++
		switch mediafiles.SlotState(t.state) {
		case mediafiles.SlotInLibrary:
			a.InLibrary++
		case mediafiles.SlotBetterWanted:
			a.InLibrary++
			a.BetterWanted++
		case mediafiles.SlotNotOutYet:
			a.NotOutYet++
		case mediafiles.SlotWanted:
			if t.keep {
				a.Missing++
			}
		}
	}
	switch {
	case !a.Included:
		a.State = "not_included"
	case a.Tracks == 0:
		a.State = "no_tracks"
	case a.InLibrary == a.Tracks:
		a.State = "complete"
	case a.NotOutYet == a.Tracks:
		a.State = "not_out_yet"
	case !a.KeepUpToDate && a.InLibrary == 0:
		a.State = "not_kept"
	case a.InLibrary > 0:
		a.State = "partial"
	default:
		a.State = "missing"
	}
	return a
}

// ArtistTotals are an artist's album and track counts, as Albums would sum them.
type ArtistTotals struct {
	Albums, Included, Tracks, InLibrary, Missing, NotOutYet, BetterWanted int
	// Downloading counts tracks on the way; Failed, failed or waiting on a decision.
	Downloading, Failed int
	NextRelease         *time.Time
}

// Totals counts an artist's albums and tracks in two aggregate queries, so
// the Title list does not read every album and pressing row.
func Totals(ctx context.Context, db database.Querier, titleID int64, now time.Time) (ArtistTotals, error) {
	var t ArtistTotals
	var next sql.NullString
	err := db.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(included), 0), min(CASE WHEN included AND release_date > ? THEN release_date END)
		FROM parts WHERE title_id = ? AND kind = 'album'`, database.FormatTime(now), titleID).Scan(&t.Albums, &t.Included, &next)
	if err != nil {
		return t, err
	}
	if next.Valid {
		if at, err := time.Parse(database.TimeLayout, next.String); err == nil {
			t.NextRelease = &at
		}
	}
	err = db.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(s.state IN (?, ?)), 0), COALESCE(sum(s.state = ?), 0),
		COALESCE(sum(s.state = ?), 0), COALESCE(sum(s.state = ? AND i.keep_up_to_date), 0),
		COALESCE(sum(s.state IN (?, ?, ?, ?)), 0), COALESCE(sum(s.state IN (?, ?)), 0) FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.title_id = ? AND i.kind = 'track' AND i.part_id IN (SELECT id FROM parts WHERE title_id = ? AND kind = 'album')`,
		mediafiles.SlotInLibrary, mediafiles.SlotBetterWanted, mediafiles.SlotBetterWanted, mediafiles.SlotNotOutYet, mediafiles.SlotWanted,
		mediafiles.SlotChosen, mediafiles.SlotDownloading, mediafiles.SlotStalled, mediafiles.SlotImporting, mediafiles.SlotFailed, mediafiles.SlotNeedsADecision, titleID, titleID, titleID).
		Scan(&t.Tracks, &t.InLibrary, &t.BetterWanted, &t.NotOutYet, &t.Missing, &t.Downloading, &t.Failed)
	return t, err
}

// ChooseEdition makes the album's chosen Pressing the edition with this
// MusicBrainz release id (a person's choice, kept by later refreshes). When
// the album's Pressings are not read yet they are read first. It reports
// whether the edition was found and chosen.
func (s *Service) ChooseEdition(ctx context.Context, partID int64, release string, by domain.Actor) (bool, error) {
	find := func() (int64, error) {
		var id int64
		err := s.Store.Read().QueryRowContext(ctx, "SELECT id FROM pressings WHERE part_id = ? AND lower(mbid) = lower(?) AND dropped_at IS NULL", partID, release).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return id, err
	}
	id, err := find()
	if err != nil {
		return false, err
	}
	if id == 0 {
		var p mediafiles.Part
		var t mediafiles.Title
		if err := database.Get(ctx, s.Store.Read(), "parts", partID, &p); err != nil {
			return false, err
		}
		if err := database.Get(ctx, s.Store.Read(), "titles", p.TitleID, &t); err != nil {
			return false, err
		}
		src, err := s.artists()
		if err != nil {
			return false, err
		}
		defs, err := ReadMusicDefaults(ctx, s.Store.Read())
		if err != nil {
			return false, err
		}
		ps, err := src.Pressings(ctx, p.SourceIDs["musicbrainz"])
		cut := errors.Is(err, metadata.ErrListCut)
		if err != nil && !cut || len(ps) == 0 {
			return false, err
		}
		now := time.Now().UTC()
		if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := syncPressingsTx(ctx, tx, t, partID, ps, cut, defs.PressingRule, now, by)
			return err
		}); err != nil {
			return false, err
		}
		if id, err = find(); err != nil || id == 0 {
			return false, err
		}
	}
	_, err = s.SetAlbum(ctx, partID, nil, id, by)
	return err == nil, err
}

// IncludeAlbums takes just the asked albums in, whatever the artist's album
// kinds say. The kinds stay as they are; an album kept up to date stays
// included on later refreshes.
func (s *Service) IncludeAlbums(ctx context.Context, titleID int64, partIDs []int64, by domain.Actor) error {
	return s.IncludeAlbumsThen(ctx, titleID, partIDs, false, by)
}

// IncludeAlbumsThen is IncludeAlbums; with search, everything the artist
// still wants is searched once the refresh has made the new albums'
// tracks (and the folder scan is done).
func (s *Service) IncludeAlbumsThen(ctx context.Context, titleID int64, partIDs []int64, search bool, by domain.Actor) error {
	if len(partIDs) == 0 {
		return nil
	}
	var t mediafiles.Title
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.Kind != "artist" {
			return ErrNotArtist
		}
		for _, id := range partIDs {
			var p mediafiles.Part
			if err := database.Get(ctx, tx, "parts", id, &p); err != nil {
				return err
			}
			if p.TitleID != t.ID || p.Kind != "album" {
				return ErrNotAlbum
			}
			if !p.Included || !p.KeepUpToDate {
				// Kept up to date in the same write, so a refresh queued
				// below can't drop the asked album before it is kept.
				p.Included, p.KeepUpToDate = true, true
				if err := database.Update(tx, "parts", &p, by); err != nil {
					return err
				}
				if _, err := tx.Exec("UPDATE items SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'track'",
					database.FormatTime(s.now()), ActorText(by), p.ID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventArtistChanged, TitleID: &t.ID, Summary: t.Name + ": asked albums included and kept up to date; album kinds unchanged",
		Data: map[string]any{"part_ids": partIDs}, GroupKey: GroupKey(t.ID)})
	params := map[string]any{"new": true}
	if search {
		params["then"] = []any{KindScanTitle, stepSearchWanted}
	}
	_, _, err = s.Queue.Enqueue(ctx, KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID}, params,
		fmt.Sprintf("refresh_title:%d", t.ID), by)
	return err
}
