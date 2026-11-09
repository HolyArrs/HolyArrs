package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ArtistAddOptions are the choices made when adding an artist. Empty values
// take the music settings: the first music Library folder, its default
// Pick rules (else Standard), kept up to date.
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
	// SearchAfter searches all the artist still wants once the MusicBrainz
	// refresh (which fills the tracks) and the folder scan are done.
	SearchAfter bool `json:"-"`
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
func ArtistByMBID(ctx context.Context, db database.Querier, mbid string) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, "SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed' AND json_extract(source_ids, '$.musicbrainz') = ?", mbid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ArtistOnNeverAdd says whether the artist is on the Never-add list.
func ArtistOnNeverAdd(ctx context.Context, db database.Querier, mbid string) (bool, error) {
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
	return s.searchArtistsKept(ctx, p, term)
}

// Name search keeps every answer (lookup_cache), so a name asked before is
// answered from disk at once and refreshed behind when old; an unseen name
// waits at most ArtistSearchWait for the outside search, which then keeps
// running in the background and fills the store for the next ask.
const (
	artistSearchPrefix = "artist_search:"
	artistSearchFresh  = 24 * time.Hour
	artistSearchKeep   = 30 * 24 * time.Hour
	artistSearchLimit  = 4
	artistSearchTotal  = 45 * time.Second
)

// ArtistSearchWait is how long a visitor waits for an unseen name.
var ArtistSearchWait = 2 * time.Second

// ErrSearchFilling says an unseen name is still being searched; ask again.
var ErrSearchFilling = errors.New("name search still running")

type keptSearch struct {
	At      time.Time         `json:"at"`
	Artists []metadata.Artist `json:"artists"`
}

type searchFlight struct {
	done chan struct{}
	out  []metadata.Artist
	err  error
}

var artistSearches = struct {
	sync.Mutex
	m     map[string]*searchFlight
	slots chan struct{}
}{m: map[string]*searchFlight{}, slots: make(chan struct{}, artistSearchLimit)}

func (s *Service) searchArtistsKept(ctx context.Context, p metadata.ArtistProvider, term string) ([]metadata.Artist, error) {
	key := artistSearchPrefix + strings.Join(strings.Fields(strings.ToLower(term)), " ")
	now := s.now()
	if s.Store != nil {
		if b, ok, err := (metadata.LookupCache{Store: s.Store}).Get(ctx, key, now); err == nil && ok {
			var k keptSearch
			if json.Unmarshal(b, &k) == nil && k.Artists != nil {
				if now.Sub(k.At) >= artistSearchFresh {
					s.artistSearch(ctx, p, key, term)
				}
				return k.Artists, nil
			}
		}
	}
	f := s.artistSearch(ctx, p, key, term)
	if f == nil {
		return nil, ErrSearchFilling
	}
	t := time.NewTimer(ArtistSearchWait)
	defer t.Stop()
	select {
	case <-f.done:
		return f.out, f.err
	case <-t.C:
		return nil, ErrSearchFilling
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// artistSearch joins or starts the background search of term (nil when
// every slot is busy); a found answer is kept.
func (s *Service) artistSearch(ctx context.Context, p metadata.ArtistProvider, key, term string) *searchFlight {
	artistSearches.Lock()
	defer artistSearches.Unlock()
	if f, ok := artistSearches.m[key]; ok {
		return f
	}
	select {
	case artistSearches.slots <- struct{}{}:
	default:
		return nil
	}
	f := &searchFlight{done: make(chan struct{})}
	artistSearches.m[key] = f
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), artistSearchTotal)
	go func() {
		defer cancel()
		f.out, f.err = p.SearchArtists(bg, term)
		if f.err == nil && f.out != nil && s.Store != nil {
			now := s.now()
			if b, err := json.Marshal(keptSearch{At: now, Artists: f.out}); err == nil {
				_ = metadata.LookupCache{Store: s.Store}.Put(bg, key, b, now, now.Add(artistSearchKeep))
			}
		}
		artistSearches.Lock()
		delete(artistSearches.m, key)
		artistSearches.Unlock()
		<-artistSearches.slots
		close(f.done)
	}()
	return f
}

// applyArtist copies looked-up details onto an artist Title.
func applyArtist(t *mediafiles.Title, a metadata.Artist) {
	t.Name, t.SortName = a.Name, SortName(a.Name)
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
		return 0, 0, ErrUnknownWhyAdded
	}
	keep := opt.KeepUpToDate == nil || *opt.KeepUpToDate

	var lf mediafiles.LibraryFolder
	if opt.LibraryFolderID == 0 {
		fs, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "kind = 'music' ORDER BY id LIMIT 1")
		if err != nil {
			return 0, 0, err
		}
		if len(fs) == 0 {
			return 0, 0, ErrNoMusicFolder
		}
		lf = fs[0]
	} else if err := database.Get(ctx, db, "library_folders", opt.LibraryFolderID, &lf); errors.Is(err, database.ErrNotFound) {
		return 0, 0, ErrNoMusicFolder
	} else if err != nil {
		return 0, 0, err
	}
	if lf.Kind != "music" {
		return 0, 0, ErrNotMusicFolder
	}
	if h := CheckFolder(lf.Path, s.DownloadFolders(ctx)); !h.Usable() {
		return 0, 0, &FolderError{Health: h}
	}
	a, err := p.Artist(ctx, mbid)
	if err != nil {
		return 0, 0, err
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return 0, 0, err
	}
	now := s.now()
	var t mediafiles.Title
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
			if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.MusicDefaultName).Scan(&prID); err != nil {
				return fmt.Errorf("pick rules: %w", err)
			}
		}
		choice := opt.Albums
		t = mediafiles.Title{Kind: "artist", Aliases: []string{}, SourceIDs: map[string]string{}, MetadataSource: "musicbrainz", KeepUpToDate: keep,
			NewPartsPolicy: policy, ReleaseGate: GateReleased, MusicTypes: opt.AlbumTypes, MusicSecondary: opt.SecondaryTypes,
			MusicStatuses: opt.ReleaseStatuses, WhyAdded: opt.WhyAdded, Artwork: []string{}, State: "active", GateDriver: "none",
			AlbumChoice: &choice, RefreshedAt: &now}
		applyArtist(&t, a)
		if _, err := database.Insert(tx, "titles", &t, by); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: t.ID, Label: "Main", PickRulesID: prID, LibraryFolderID: lf.ID, FolderName: mediafiles.ArtistFolder(ns, mediafiles.TrackFacts{Artist: t.Name}), KeepUpToDate: keep, IsMain: true}
		if opt.Adopt != nil && opt.Adopt.FolderName != "" {
			v.FolderName = opt.Adopt.FolderName
		}
		if _, err := database.Insert(tx, "versions", v, by); err != nil {
			return err
		}
		var err error
		sync, err = syncAlbumsTx(ctx, tx, t, a, choice, now, by)
		return err
	})
	if err != nil {
		return 0, 0, err
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleAdded, TitleID: &t.ID, Summary: "Added " + t.Name,
		Data: map[string]any{"why_added": t.WhyAdded, "albums": opt.Albums, "new_albums": opt.NewAlbums, "album_types": opt.AlbumTypes,
			"secondary_types": opt.SecondaryTypes, "release_statuses": opt.ReleaseStatuses, "albums_added": sync.AlbumsAdded},
		GroupKey: GroupKey(t.ID)})
	// The picture Job finds the artist's pictures at once, apart from the
	// MusicBrainz refresh, so the add waits on neither.
	if _, _, err := s.Queue.Enqueue(ctx, KindArtistPictures, &domain.Subject{Type: "title", ID: t.ID}, nil,
		KindArtistPictures+":"+strconv.FormatInt(t.ID, 10), by); err != nil {
		log.Printf("library: queue pictures for artist %d: %v", t.ID, err)
	}
	then := []any{KindScanTitle}
	if opt.SearchAfter {
		then = append(then, stepSearchWanted)
	}
	jobID, _, err := s.Queue.Enqueue(ctx, KindRefreshTitle, &domain.Subject{Type: "title", ID: t.ID},
		map[string]any{"then": then, "new": true}, "refresh_title:"+strconv.FormatInt(t.ID, 10), by)
	return t.ID, jobID, err
}

// artistPicture finds an artist's picture and keeps it as the artist's
// artwork. Nothing found, or the picture source down, changes nothing and
// fails nothing.
func (s *Service) artistPicture(ctx context.Context, titleID int64, mbid string, by domain.Actor) {
	if s.Artwork == nil || s.ImageServer == "" || !metadata.ValidMBID(mbid) {
		return
	}
	// Only the metadata server's stored photo (never Deezer or Wikidata).
	if name := s.serverPicture(ctx, titleID, "artist", mbid, "500.webp"); name != "" {
		s.fillArt(ctx, titleID, name, by)
	}
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
func newAlbumKeep(t mediafiles.Title, release *time.Time) bool {
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
	err := tx.QueryRow("SELECT count(*) FROM slots WHERE item_id IN (SELECT id FROM items WHERE part_id = ?) AND ("+HoldingSlots+")", partID).Scan(&n)
	return n > 0, err
}

// statusOK reports whether an album's stored Pressings allow it: it has
// none yet, or one with a counted status.
func statusOK(tx *sql.Tx, t mediafiles.Title, partID int64) (bool, error) {
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
