package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// ArtistAddOptions are the choices made when adding an artist. Empty values
// take the music settings: the first music Library folder, its default
// Pick rules (else Good everyday), kept up to date.
type ArtistAddOptions struct {
	LibraryFolderID int64    `json:"library_folder_id"`
	PickRulesID     int64    `json:"pick_rules_id"`
	KeepUpToDate    *bool    `json:"keep_up_to_date"`
	Albums          string   `json:"albums"`           // all, future, missing, latest, first, none
	NewAlbums       string   `json:"new_albums"`       // all, new, none
	AlbumTypes      []string `json:"album_types"`      // main album kinds included
	SecondaryTypes  []string `json:"secondary_types"`  // extra album kinds included
	ReleaseStatuses []string `json:"release_statuses"` // Pressing statuses that count
	// WhyAdded records why the artist was added (by_hand when empty).
	WhyAdded string `json:"-"`
	// Adopt keeps an artist folder found on disk as the artist's folder.
	Adopt *MusicAdoption `json:"-"`
}

// ArtistAlreadyAddedError says the artist is already in the Library.
type ArtistAlreadyAddedError struct{ TitleID int64 }

func (e *ArtistAlreadyAddedError) Error() string {
	return fmt.Sprintf("this artist is already in the Library (Title %d)", e.TitleID)
}

// ArtistMBID reads "mbid:<id>", "musicbrainz:<id>" or a bare MusicBrainz id.
func ArtistMBID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	low := strings.ToLower(id)
	for _, p := range []string{"mbid:", "musicbrainz:"} {
		if strings.HasPrefix(low, p) {
			id = strings.TrimSpace(id[len(p):])
			break
		}
	}
	return strings.ToLower(id), metadata.ValidMBID(id)
}

// ArtistByMBID finds the artist Title with a MusicBrainz id; 0 when none.
func ArtistByMBID(ctx context.Context, db store.Querier, mbid string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed' AND json_extract(source_ids, '$.musicbrainz') = ?", mbid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ArtistOnNeverAdd says whether the artist is on the Never-add list.
func ArtistOnNeverAdd(ctx context.Context, db store.Querier, mbid string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM never_add WHERE kind = 'artist' AND json_extract(source_ids, '$.musicbrainz') = ?", mbid).Scan(&n)
	return n > 0, err
}

func (s *Service) artists() (metadata.ArtistProvider, error) {
	if s.Artists == nil {
		return nil, ErrNoMusicSource
	}
	return s.Artists, nil
}

// LookupArtists finds artists by name, or one by "mbid:<id>".
func (s *Service) LookupArtists(ctx context.Context, term string) ([]metadata.Artist, error) {
	p, err := s.artists()
	if err != nil {
		return nil, err
	}
	low := strings.ToLower(strings.TrimSpace(term))
	if strings.HasPrefix(low, "mbid:") || strings.HasPrefix(low, "musicbrainz:") || metadata.ValidMBID(low) {
		mbid, ok := ArtistMBID(term)
		if !ok {
			return []metadata.Artist{}, nil
		}
		a, err := p.Artist(ctx, mbid)
		if errors.Is(err, metadata.ErrArtistNotFound) {
			return []metadata.Artist{}, nil
		}
		if err != nil {
			return nil, err
		}
		return []metadata.Artist{a}, nil
	}
	return p.SearchArtists(ctx, term)
}

// applyArtist copies looked-up details onto an artist Title.
func applyArtist(t *domain.Title, a metadata.Artist) {
	t.Name, t.SortName = a.Name, sortName(a.Name)
	ids := map[string]string{}
	for k, v := range t.SourceIDs {
		ids[k] = v
	}
	ids["musicbrainz"] = a.MBID
	t.SourceIDs = ids
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	t.ArtistType, t.Disambiguation = str(a.Type), str(a.Disambiguation)
	t.Status = "running"
	if a.Status == "ended" {
		t.Status = "ended"
	}
}

// AddArtist adds an artist by MusicBrainz id with its album choice, new
// album choice and album kinds. The artist, a main Version and every album
// are written at once (each album kept up to date or not by the choice);
// a details refresh Job follows that fetches the Pressings and tracks of
// the included albums and makes one Slot per track.
func (s *Service) AddArtist(ctx context.Context, id string, opt ArtistAddOptions, by domain.Actor) (int64, int64, error) {
	mbid, ok := ArtistMBID(id)
	if !ok {
		return 0, 0, ErrBadArtistID
	}
	p, err := s.artists()
	if err != nil {
		return 0, 0, err
	}
	db := s.Store.Read()
	defs, err := ReadMusicDefaults(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	if opt.Albums == "" {
		opt.Albums = defs.AlbumChoice
	}
	if opt.NewAlbums == "" {
		opt.NewAlbums = defs.NewAlbums
	}
	if opt.AlbumTypes == nil {
		opt.AlbumTypes = defs.AlbumTypes
	}
	if opt.SecondaryTypes == nil {
		opt.SecondaryTypes = defs.SecondaryTypes
	}
	if opt.ReleaseStatuses == nil {
		opt.ReleaseStatuses = defs.ReleaseStatuses
	}
	if opt.WhyAdded == "" {
		opt.WhyAdded = "by_hand"
	}
	policy, ok := newAlbumsPolicy(opt.NewAlbums)
	switch {
	case !ValidAlbumChoice(opt.Albums):
		return 0, 0, ErrBadAlbumChoice
	case !ok:
		return 0, 0, ErrBadNewAlbums
	}
	if opt.AlbumTypes, ok = cleanList(opt.AlbumTypes, MainAlbumKinds); !ok {
		return 0, 0, ErrBadAlbumKinds
	}
	if opt.SecondaryTypes, ok = cleanList(opt.SecondaryTypes, ExtraAlbumKinds); !ok {
		return 0, 0, ErrBadExtraKinds
	}
	if opt.ReleaseStatuses, ok = cleanList(opt.ReleaseStatuses, PressingStatuses); !ok {
		return 0, 0, ErrBadStatuses
	}
	switch opt.WhyAdded {
	case "by_hand", "list", "series_follow", "ask", "import", "migration":
	default:
		return 0, 0, errUnknownWhyAdded
	}
	keep := opt.KeepUpToDate == nil || *opt.KeepUpToDate

	var lf domain.LibraryFolder
	if opt.LibraryFolderID == 0 {
		fs, err := store.Select[domain.LibraryFolder](ctx, db, "library_folders", "kind = 'music' ORDER BY id LIMIT 1")
		if err != nil {
			return 0, 0, err
		}
		if len(fs) == 0 {
			return 0, 0, ErrNoMusicFolder
		}
		lf = fs[0]
	} else if err := store.Get(ctx, db, "library_folders", opt.LibraryFolderID, &lf); errors.Is(err, store.ErrNotFound) {
		return 0, 0, ErrNoMusicFolder
	} else if err != nil {
		return 0, 0, err
	}
	if lf.Kind != "music" {
		return 0, 0, ErrNotMusicFolder
	}
	if h := CheckFolder(lf.Path, s.downloadPaths(ctx)); !h.Usable() {
		return 0, 0, &FolderError{Health: h}
	}
	a, err := p.Artist(ctx, mbid)
	if err != nil {
		return 0, 0, err
	}
	ns, err := naming.Read(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	now := s.now()
	var t domain.Title
	var sync ArtistSync
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if existing, err := ArtistByMBID(ctx, tx, a.MBID); err != nil {
			return err
		} else if existing != 0 {
			return &ArtistAlreadyAddedError{TitleID: existing}
		}
		if on, err := ArtistOnNeverAdd(ctx, tx, a.MBID); err != nil {
			return err
		} else if on {
			return ErrArtistNeverAdd
		}
		prID := opt.PickRulesID
		if prID != 0 {
			if err := pickRulesExist(tx, &prID); err != nil {
				return err
			}
			if !musicRulesTx(tx, prID) {
				return ErrNotMusicRules
			}
		}
		if prID == 0 && lf.DefaultPickRules != nil && musicRulesTx(tx, *lf.DefaultPickRules) {
			prID = *lf.DefaultPickRules
		}
		if prID == 0 {
			// A folder whose default is for movies and shows gets the music default.
			if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", domain.MusicDefaultName).Scan(&prID); err != nil {
				return fmt.Errorf("pick rules: %w", err)
			}
		}
		choice := opt.Albums
		t = domain.Title{Kind: "artist", Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "musicbrainz", KeepUpToDate: keep,
			NewPartsPolicy: policy, ReleaseGate: GateReleased, MusicTypes: opt.AlbumTypes, MusicSecondary: opt.SecondaryTypes,
			MusicStatuses: opt.ReleaseStatuses, WhyAdded: opt.WhyAdded, Artwork: []string{}, State: "active", GateDriver: "none",
			AlbumChoice: &choice, RefreshedAt: &now}
		applyArtist(&t, a)
		if _, err := store.Insert(tx, "titles", &t, by); err != nil {
			return err
		}
		v := &domain.Version{TitleID: t.ID, Label: "Main", PickRulesID: prID, LibraryFolderID: lf.ID, FolderName: naming.ArtistFolder(ns, naming.TrackFacts{Artist: t.Name}), KeepUpToDate: keep, IsMain: true}
		if opt.Adopt != nil && opt.Adopt.FolderName != "" {
			v.FolderName = opt.Adopt.FolderName
		}
		if _, err := store.Insert(tx, "versions", v, by); err != nil {
			return err
		}
		var err error
		sync, err = syncAlbumsTx(ctx, tx, t, a, choice, now, by)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleAdded, TitleID: &t.ID, Summary: "Added " + t.Name,
		Data: map[string]any{"why_added": t.WhyAdded, "albums": opt.Albums, "new_albums": opt.NewAlbums, "album_types": opt.AlbumTypes,
			"secondary_types": opt.SecondaryTypes, "release_statuses": opt.ReleaseStatuses, "albums_added": sync.AlbumsAdded},
		GroupKey: groupKey(t.ID)})
	s.artistPicture(ctx, t.ID, t.Name, by)
	jobID, _, err := s.Queue.Enqueue(ctx, KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID},
		map[string]any{"then": []any{KindScanTitle}, "new": true}, "refresh_title:"+strconv.FormatInt(t.ID, 10), by)
	return t.ID, jobID, err
}

// artistPicture finds an artist's picture and keeps it as the artist's
// artwork. Nothing found, or the picture source down, changes nothing and
// fails nothing.
func (s *Service) artistPicture(ctx context.Context, titleID int64, name string, by domain.Actor) {
	if s.ArtistImages == nil || s.Artwork == nil {
		return
	}
	pic, err := s.ArtistImages.Image(ctx, name)
	if err != nil {
		log.Printf("library: picture for artist %d: %v", titleID, err)
		return
	}
	s.cachePoster(ctx, titleID, pic, by)
}

// ArtistSync says what one sync of an artist's albums, Pressings and
// tracks did.
type ArtistSync struct {
	AlbumsAdded     int     `json:"albums_added"`
	AlbumsChanged   int     `json:"albums_changed"`
	AlbumsRemoved   int     `json:"albums_removed"`
	AlbumsDropped   int     `json:"albums_dropped"` // kept because a file or download holds them
	PressingsChosen int     `json:"pressings_chosen"`
	TracksAdded     int     `json:"tracks_added"`
	TracksChanged   int     `json:"tracks_changed"`
	TracksRemoved   int     `json:"tracks_removed"`
	TracksDropped   int     `json:"tracks_dropped"`
	Opened          []int64 `json:"gate_opened_slots,omitempty"`
	Closed          []int64 `json:"gate_closed_slots,omitempty"`
}

func (a *ArtistSync) add(b ArtistSync) {
	a.AlbumsAdded += b.AlbumsAdded
	a.AlbumsChanged += b.AlbumsChanged
	a.AlbumsRemoved += b.AlbumsRemoved
	a.AlbumsDropped += b.AlbumsDropped
	a.PressingsChosen += b.PressingsChosen
	a.TracksAdded += b.TracksAdded
	a.TracksChanged += b.TracksChanged
	a.TracksRemoved += b.TracksRemoved
	a.TracksDropped += b.TracksDropped
}

// newAlbumKeep says whether an album a refresh found (or newly included)
// starts kept up to date: the artist's new-album choice decides.
func newAlbumKeep(t domain.Title, release *time.Time) bool {
	if !t.KeepUpToDate {
		return false
	}
	switch t.NewPartsPolicy {
	case "all":
		return true
	case "future_only":
		return release == nil || release.After(t.CreatedAt)
	}
	return false
}

// partHeld reports whether a Slot of the album's tracks holds a File or a
// Download.
func partHeld(tx *sql.Tx, partID int64) (bool, error) {
	var n int
	err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id IN (SELECT id FROM items WHERE part_id = ?) AND ("+holdingSlots+")", partID).Scan(&n)
	return n > 0, err
}

// statusOK reports whether an album's stored Pressings allow it: it has
// none yet, or one with a counted status.
func statusOK(tx *sql.Tx, t domain.Title, partID int64) (bool, error) {
	rows, err := tx.Query("SELECT status FROM pressings WHERE part_id = ?", partID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	any := false
	for rows.Next() {
		var st string
		if err := rows.Scan(&st); err != nil {
			return false, err
		}
		any = true
		if slices.Contains(t.MusicStatuses, st) {
			return true, nil
		}
	}
	return !any, rows.Err()
}

// syncAlbumsTx brings an artist's album rows in line with a. choice is the
// album choice when the artist is being added, "" on a refresh. Albums
// match by MusicBrainz id, so rows, Slots and Files survive. An album the
// source no longer lists is removed when nothing holds it, else flagged;
// when the source's list is cut short (a.Partial) nothing is removed.
func syncAlbumsTx(ctx context.Context, tx *sql.Tx, t domain.Title, a metadata.Artist, choice string, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	parts, err := store.Select[domain.Part](ctx, tx, "parts", "title_id = ? AND kind = 'album'", t.ID)
	if err != nil {
		return out, err
	}
	byMBID := map[string]*domain.Part{}
	for i := range parts {
		byMBID[parts[i].SourceIDs["musicbrainz"]] = &parts[i]
	}
	var albums []metadata.Album
	seen := map[string]bool{}
	for _, al := range a.Albums {
		if al.MBID == "" || seen[al.MBID] {
			continue
		}
		seen[al.MBID] = true
		albums = append(albums, al)
	}
	var keepAtAdd map[string]bool
	if choice != "" {
		var ids []string
		dates := map[string]*time.Time{}
		for _, al := range albums {
			if AlbumKindsOK(t, al.PrimaryType, al.SecondaryTypes) {
				ids = append(ids, al.MBID)
				dates[al.MBID] = musicDate(al.ReleaseDate)
			}
		}
		keepAtAdd = albumChoiceKeep(choice, ids, dates, nil, now)
	}
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	for _, al := range albums {
		date, prec := musicDate(al.ReleaseDate), musicDatePrecision(al.ReleaseDate)
		typ := al.PrimaryType
		sec := al.SecondaryTypes
		if sec == nil {
			sec = []string{}
		}
		kinds := AlbumKindsOK(t, typ, sec)
		if p := byMBID[al.MBID]; p != nil {
			ok, err := statusOK(tx, t, p.ID)
			if err != nil {
				return out, err
			}
			included := kinds && ok
			dateMoved := !eqTime(p.ReleaseDate, date)
			changed := p.Name != al.Name || !eqStr(p.ReleaseType, str(typ)) || !slices.Equal(p.SecondaryTypes, sec) || dateMoved ||
				!eqStr(p.ReleaseDatePrecision, prec) ||
				!eqStr(p.Disambiguation, str(al.Disambiguation)) || p.DroppedAt != nil || p.Included != included
			if !changed {
				continue
			}
			if included != p.Included {
				p.KeepUpToDate = included && newAlbumKeep(t, date)
				if _, err := tx.Exec("UPDATE items SET keep_up_to_date = ? WHERE part_id = ? AND kind = 'track'", p.KeepUpToDate, p.ID); err != nil {
					return out, err
				}
			}
			p.Name, p.ReleaseType, p.SecondaryTypes, p.ReleaseDate, p.Disambiguation, p.DroppedAt, p.Included = al.Name, str(typ), sec, date, str(al.Disambiguation), nil, included
			p.ReleaseDatePrecision = prec
			if err := store.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
			if dateMoved {
				var d any
				if date != nil {
					d = store.FormatTime(*date)
				}
				if _, err := tx.Exec("UPDATE items SET air_date = ? WHERE part_id = ? AND kind = 'track'", d, p.ID); err != nil {
					return out, err
				}
			}
			if !included {
				res, err := dropTracksTx(tx, p.ID, nil, now, by)
				if err != nil {
					return out, err
				}
				out.add(res)
			}
			out.AlbumsChanged++
			continue
		}
		keep := kinds && t.KeepUpToDate
		if choice != "" {
			keep = keep && keepAtAdd[al.MBID]
		} else {
			keep = keep && newAlbumKeep(t, date)
		}
		p := &domain.Part{TitleID: t.ID, Kind: "album", Name: al.Name, ReleaseType: str(typ), ReleaseDate: date, ReleaseDatePrecision: prec, KeepUpToDate: keep,
			SourceIDs: map[string]string{"musicbrainz": al.MBID}, Pressings: []domain.Pressing{}, SecondaryTypes: sec, Included: kinds,
			Disambiguation: str(al.Disambiguation)}
		if _, err := store.Insert(tx, "parts", p, by); err != nil {
			return out, err
		}
		out.AlbumsAdded++
	}
	for mbid, p := range byMBID {
		if seen[mbid] || a.Partial {
			continue
		}
		held, err := partHeld(tx, p.ID)
		if err != nil {
			return out, err
		}
		if !held {
			if _, err := tx.Exec("DELETE FROM parts WHERE id = ?", p.ID); err != nil {
				return out, err
			}
			out.AlbumsRemoved++
			continue
		}
		if p.DroppedAt == nil {
			d := now
			p.DroppedAt = &d
			if err := store.Update(tx, "parts", p, by); err != nil {
				return out, err
			}
			out.AlbumsDropped++
		}
	}
	return out, nil
}

// dropTracksTx removes an album's tracks that keep is nil for or that keep
// does not hold (item ids): deleted when nothing holds them, else kept and
// flagged.
func dropTracksTx(tx *sql.Tx, partID int64, keep map[int64]bool, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	rows, err := tx.Query("SELECT id, dropped_at IS NOT NULL FROM items WHERE part_id = ? AND kind = 'track'", partID)
	if err != nil {
		return out, err
	}
	type row struct {
		id      int64
		dropped bool
	}
	var rs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.dropped); err != nil {
			rows.Close()
			return out, err
		}
		rs = append(rs, r)
	}
	rows.Close()
	for _, r := range rs {
		if keep[r.id] {
			continue
		}
		var held int
		if err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id = ? AND ("+holdingSlots+")", r.id).Scan(&held); err != nil {
			return out, err
		}
		if held == 0 {
			if _, err := tx.Exec("DELETE FROM items WHERE id = ?", r.id); err != nil {
				return out, err
			}
			out.TracksRemoved++
			continue
		}
		if !r.dropped {
			if _, err := tx.Exec("UPDATE items SET dropped_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				store.FormatTime(now), store.FormatTime(now), actorText(by), r.id); err != nil {
				return out, err
			}
			out.TracksDropped++
		}
	}
	return out, nil
}

// syncPressingsTx brings an album's Pressings in line with ps, keeps the
// chosen Pressing (a person's choice, or the rule's earlier pick) while the
// source lists it, else chooses by rule, and makes the album's tracks the
// chosen Pressing's tracks. An album none of whose Pressings has a counted
// status stops being included. An empty ps changes nothing; with cut (the
// source's list was cut short) Pressings it did not list are kept.
func syncPressingsTx(ctx context.Context, tx *sql.Tx, t domain.Title, partID int64, ps []metadata.Pressing, cut bool, rule string, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	if len(ps) == 0 {
		return out, nil
	}
	var p domain.Part
	if err := store.Get(ctx, tx, "parts", partID, &p); err != nil {
		return out, err
	}
	rows, err := store.Select[domain.AlbumPressing](ctx, tx, "pressings", "part_id = ?", partID)
	if err != nil {
		return out, err
	}
	have := map[string]*domain.AlbumPressing{}
	for i := range rows {
		have[rows[i].MBID] = &rows[i]
	}
	listed := map[string]bool{}
	var kept []domain.AlbumPressing
	for _, mp := range ps {
		if listed[mp.MBID] {
			continue
		}
		listed[mp.MBID] = true
		tracks := make([]domain.PressingTrack, 0, len(mp.Tracks))
		for _, tr := range mp.Tracks {
			tracks = append(tracks, domain.PressingTrack(tr))
		}
		formats := mp.Formats
		if formats == nil {
			formats = []string{}
		}
		row := have[mp.MBID]
		if row == nil {
			row = &domain.AlbumPressing{PartID: partID, MBID: mp.MBID, ChosenBy: "auto"}
		}
		row.Name, row.Status, row.Country, row.Date, row.Disambiguation, row.Formats, row.TrackCount, row.Tracks, row.DroppedAt =
			mp.Name, mp.Status, mp.Country, mp.Date, mp.Disambiguation, formats, len(tracks), tracks, nil
		if row.ID == 0 {
			if _, err := store.Insert(tx, "pressings", row, by); err != nil {
				return out, err
			}
		} else if err := store.Update(tx, "pressings", row, by); err != nil {
			return out, err
		}
		kept = append(kept, *row)
	}
	for mbid, row := range have {
		if !listed[mbid] && cut {
			kept = append(kept, *row)
		} else if !listed[mbid] {
			if _, err := tx.Exec("DELETE FROM pressings WHERE id = ?", row.ID); err != nil {
				return out, err
			}
		}
	}
	countable := false
	for _, k := range kept {
		countable = countable || slices.Contains(t.MusicStatuses, k.Status)
	}
	if len(kept) > 0 && !countable {
		if p.Included {
			p.Included, p.KeepUpToDate = false, false
			if err := store.Update(tx, "parts", &p, by); err != nil {
				return out, err
			}
			out.AlbumsChanged++
		}
		res, err := dropTracksTx(tx, partID, nil, now, by)
		out.add(res)
		return out, err
	}
	chosen := -1
	for i, k := range kept {
		if k.Chosen {
			chosen = i
		}
	}
	if chosen < 0 {
		chosen = ChoosePressing(kept, t.MusicStatuses, rule)
		if chosen >= 0 {
			if _, err := tx.Exec("UPDATE pressings SET chosen = 0 WHERE part_id = ?", partID); err != nil {
				return out, err
			}
			if _, err := tx.Exec("UPDATE pressings SET chosen = 1, chosen_by = 'auto' WHERE id = ?", kept[chosen].ID); err != nil {
				return out, err
			}
			out.PressingsChosen++
		}
	}
	var tracks []domain.PressingTrack
	if chosen >= 0 {
		tracks = kept[chosen].Tracks
	}
	res, err := syncTracksTx(ctx, tx, t.ID, p, tracks, now, by)
	out.add(res)
	return out, err
}

// syncTracksTx makes an album's track rows the tracks given (the chosen
// Pressing's). Rows match by track id, then recording id, then disc and
// number, so their Slots and Files stay; tracks no longer there are
// removed, or kept and flagged when a File or Download holds them; new
// tracks get one Slot per Version, not out yet or wanted by the album's
// release date. No tracks changes nothing: an empty list is never taken
// to mean every track went.
func syncTracksTx(ctx context.Context, tx *sql.Tx, titleID int64, p domain.Part, tracks []domain.PressingTrack, now time.Time, by domain.Actor) (ArtistSync, error) {
	var out ArtistSync
	if len(tracks) == 0 {
		return out, nil
	}
	items, err := store.Select[domain.Item](ctx, tx, "items", "part_id = ? AND kind = 'track' ORDER BY id", p.ID)
	if err != nil {
		return out, err
	}
	versions, err := store.Select[domain.Version](ctx, tx, "versions", "title_id = ?", titleID)
	if err != nil {
		return out, err
	}
	claimed := map[int64]bool{}
	match := make([]*domain.Item, len(tracks))
	find := func(ok func(it domain.Item, tr domain.PressingTrack) bool) {
		for i, tr := range tracks {
			if match[i] != nil {
				continue
			}
			for j := range items {
				if !claimed[items[j].ID] && ok(items[j], tr) {
					match[i], claimed[items[j].ID] = &items[j], true
					break
				}
			}
		}
	}
	find(func(it domain.Item, tr domain.PressingTrack) bool {
		return tr.ID != "" && it.SourceIDs["musicbrainz"] == tr.ID
	})
	find(func(it domain.Item, tr domain.PressingTrack) bool {
		return tr.RecordingID != "" && it.RecordingID != nil && *it.RecordingID == tr.RecordingID
	})
	find(func(it domain.Item, tr domain.PressingTrack) bool {
		return it.Numbers.Disc != nil && it.Numbers.Track != nil && *it.Numbers.Disc == tr.Disc && *it.Numbers.Track == tr.Number
	})
	res, err := dropTracksTx(tx, p.ID, claimed, now, by)
	if err != nil {
		return out, err
	}
	out.add(res)
	open, reason := AlbumGate(p.ReleaseDate, now)
	for i, tr := range tracks {
		name, rec := tr.Name, tr.RecordingID
		var length *int
		if tr.LengthMS > 0 {
			l := tr.LengthMS
			length = &l
		}
		disc, num := tr.Disc, tr.Number
		if it := match[i]; it != nil {
			changed := !eqStr(it.Name, &name) || !eqStr(it.RecordingID, &rec) || !eqInt(it.LengthMS, length) || it.SourceIDs["musicbrainz"] != tr.ID ||
				!eqInt(it.Numbers.Disc, &disc) || !eqInt(it.Numbers.Track, &num) || it.DroppedAt != nil || !eqTime(it.AirDate, p.ReleaseDate)
			if !changed {
				continue
			}
			ids := map[string]string{}
			for k, v := range it.SourceIDs {
				ids[k] = v
			}
			ids["musicbrainz"] = tr.ID
			it.Name, it.RecordingID, it.LengthMS, it.SourceIDs, it.Numbers.Disc, it.Numbers.Track, it.DroppedAt, it.AirDate =
				&name, &rec, length, ids, &disc, &num, nil, p.ReleaseDate
			if err := store.Update(tx, "items", it, by); err != nil {
				return out, err
			}
			out.TracksChanged++
			continue
		}
		it := &domain.Item{TitleID: titleID, PartID: &p.ID, Kind: "track", Numbers: domain.Numbers{Disc: &disc, Track: &num}, Name: &name,
			AirDate: p.ReleaseDate, KeepUpToDate: p.KeepUpToDate, RecordingID: &rec, LengthMS: length, SourceIDs: map[string]string{"musicbrainz": tr.ID}}
		if _, err := store.Insert(tx, "items", it, by); err != nil {
			return out, err
		}
		out.TracksAdded++
		state := domain.SlotNotOutYet
		if open {
			state = domain.SlotWanted
		}
		for _, v := range versions {
			r := reason
			if _, err := store.Insert(tx, "slots", &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: state, LastReason: &r}, by); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

// ErrNotMusicRules says Pick rules for movies and shows were given for an
// artist: they have no music steps, so they could never pick an album.
var ErrNotMusicRules = errors.New("these Pick rules are for movies and shows; choose music Pick rules (Lossless first, Lossy 320 or V0, Any music)")

// musicRulesTx says the Pick rules judge music.
func musicRulesTx(tx *sql.Tx, id int64) bool {
	var pr domain.PickRules
	if err := store.Get(context.Background(), tx, "pick_rules", id, &pr); err != nil {
		return false
	}
	return domain.IsMusicRules(pr.Body)
}
