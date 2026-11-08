package requests

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Errors from asking for music.
var (
	ErrNoSuchAlbum      = errors.New("the artist has no such album")
	ErrAlbumNotIncluded = errors.New("the artist is in the Library and its album kinds leave this album out; ask the owner or an admin to change the artist's album kinds")
	// ErrAlbumNotListedYet refuses an album of an artist in the Library
	// that the Library has not listed yet (its next refresh adds it).
	ErrAlbumNotListedYet = errors.New("the Library has not listed this album for the artist yet; try again after the artist's next refresh")
)

// AskArtist asks for an artist for p by "mbid:<id>": the whole artist (every
// album its kinds include) when albums is empty, else the albums with those
// MusicBrainz ids. An open ask for the same artist joins it (a waiting one
// takes the new albums too); an artist already in the Library is asked only
// for the albums it does not keep up to date yet.
func (s *Service) AskArtist(ctx context.Context, p Person, id string, albums []string) (Result, error) {
	if p.MaxRating != "" {
		return Result{}, overLimit(p, "artist", "")
	}
	mbid, ok := library.ArtistMBID(id)
	if !ok {
		return Result{}, library.ErrBadArtistID
	}
	return s.askArtist(ctx, p, mbid, albums, nil, false)
}

// AskArtistEditions asks for albums as AskArtist does, with an edition
// (a MusicBrainz release id) per album where the person picked one; the
// approval chooses that edition for the album.
func (s *Service) AskArtistEditions(ctx context.Context, p Person, id string, albums []string, editions map[string]string) (Result, error) {
	if p.MaxRating != "" {
		return Result{}, overLimit(p, "artist", "")
	}
	mbid, ok := library.ArtistMBID(id)
	if !ok {
		return Result{}, library.ErrBadArtistID
	}
	return s.askArtist(ctx, p, mbid, albums, editions, false)
}

func (s *Service) askArtist(ctx context.Context, p Person, mbid string, albums []string, editions map[string]string, retried bool) (Result, error) {
	if s.Library == nil || s.Library.Artists == nil {
		return Result{}, library.ErrNoMusicSource
	}
	ar, err := s.Library.Artists.Artist(ctx, mbid)
	if err != nil {
		return Result{}, err
	}
	want, err := pickAlbums(albums, ar.Albums)
	if err != nil {
		return Result{}, err
	}
	for i := range want {
		want[i].Edition = strings.ToLower(editions[want[i].MBID])
	}
	db := s.Store.Read()
	tid, err := library.ArtistByMBID(ctx, db, ar.MBID)
	if err != nil {
		return Result{}, err
	}
	var missing []domain.AskAlbum
	if tid != 0 {
		// An album the artist's kinds leave out is refused before anything
		// else, even when a whole-artist ask is open.
		if missing, err = albumsNotKept(ctx, db, tid, want); err != nil {
			return Result{}, err
		}
	}
	open, err := openArtistAsks(ctx, db, ar.MBID)
	if err != nil {
		return Result{}, err
	}
	for _, o := range open {
		if coversAlbums(o.Albums, want) {
			return s.join(ctx, p, o.ID)
		}
	}
	for _, o := range open {
		if o.State == domain.AskAsked {
			res, err := s.joinWiderArtist(ctx, p, o, want)
			if errors.Is(err, ErrNotOpen) && !retried {
				return s.askArtist(ctx, p, mbid, albums, editions, true)
			}
			return res, err
		}
	}
	if tid != 0 {
		if len(missing) == 0 {
			ready, err := musicReady(ctx, db, tid, albumIDs(want), s.now())
			if err != nil {
				return Result{}, err
			}
			return Result{}, &InLibraryError{TitleID: tid, Ready: ready, Music: true}
		}
		want = missing
	}
	if never, err := library.ArtistOnNeverAdd(ctx, db, ar.MBID); err != nil {
		return Result{}, err
	} else if never {
		return Result{}, library.ErrArtistNeverAdd
	}
	if err := s.checkLimit(ctx, db, p, KindMusic); err != nil {
		return Result{}, err
	}
	now := s.now()
	a := domain.Ask{Kind: "artist", Ref: "mbid:" + ar.MBID, SourceIDs: map[string]string{"musicbrainz": ar.MBID}, Seasons: []int{}, Albums: want,
		Name: ar.Name, State: domain.AskAsked, AskedBy: p.UserID, AskedAt: now}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		// Look again inside the write, as for shows.
		open, err := openArtistAsks(ctx, tx, ar.MBID)
		if err != nil {
			return err
		}
		for _, o := range open {
			if coversAlbums(o.Albums, want) || o.State == domain.AskAsked {
				return errAskedMeanwhile
			}
		}
		if err := s.checkLimit(ctx, tx, p, KindMusic); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "asks", &a, p.actor()); err != nil {
			return err
		}
		_, err = tx.Exec(askArtistSQL, a.ID, p.UserID, database.FormatTime(now))
		return err
	})
	if errors.Is(err, errAskedMeanwhile) && !retried {
		return s.askArtist(ctx, p, mbid, albums, editions, true)
	}
	if err != nil {
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskMade, Summary: p.Name + " asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "artist": a.Ref, "albums": albumWords(a.Albums), "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
	if p.AutoApprove {
		if got, err := s.decide(ctx, a.ID, p, ApproveOptions{}, true); err == nil {
			return Result{Ask: got}, nil
		} else {
			log.Printf("asks: auto-approve %s: %v", a.Name, err)
		}
	}
	s.tellStaff(ctx, a, p)
	s.changed(ctx, a.ID)
	return Result{Ask: a}, nil
}

// pickAlbums checks the asked-for album ids against the artist's albums and
// gives them with their names and years, oldest first; empty stays empty
// (the whole artist).
func pickAlbums(asked []string, known []metadata.Album) ([]domain.AskAlbum, error) {
	out := []domain.AskAlbum{}
	type dated struct {
		al   domain.AskAlbum
		date string
	}
	var picked []dated
	for _, id := range asked {
		id = strings.ToLower(strings.TrimSpace(id))
		i := slices.IndexFunc(known, func(k metadata.Album) bool { return k.MBID == id })
		if i < 0 {
			return nil, ErrNoSuchAlbum
		}
		if slices.ContainsFunc(picked, func(d dated) bool { return d.al.MBID == id }) {
			continue
		}
		picked = append(picked, dated{AskAlbum(known[i]), known[i].ReleaseDate})
	}
	slices.SortStableFunc(picked, func(a, b dated) int { return strings.Compare(a.date+a.al.MBID, b.date+b.al.MBID) })
	for _, d := range picked {
		out = append(out, d.al)
	}
	return out, nil
}

// AskAlbum names a looked-up album as an ask keeps it.
func AskAlbum(al metadata.Album) domain.AskAlbum {
	y := 0
	if len(al.ReleaseDate) >= 4 {
		y, _ = strconv.Atoi(al.ReleaseDate[:4])
	}
	return domain.AskAlbum{MBID: al.MBID, Name: al.Name, Year: y}
}

func albumIDs(as []domain.AskAlbum) []string {
	out := []string{}
	for _, a := range as {
		out = append(out, a.MBID)
	}
	return out
}

// coversAlbums says whether an ask for have already asks for want (empty
// is the whole artist).
func coversAlbums(have, want []domain.AskAlbum) bool {
	if len(have) == 0 {
		return true
	}
	if len(want) == 0 {
		return false
	}
	for _, w := range want {
		if !slices.ContainsFunc(have, func(h domain.AskAlbum) bool { return h.MBID == w.MBID }) {
			return false
		}
	}
	return true
}

// widenAlbums joins two album lists; either being the whole artist gives
// the whole artist.
func widenAlbums(a, b []domain.AskAlbum) []domain.AskAlbum {
	if len(a) == 0 || len(b) == 0 {
		return []domain.AskAlbum{}
	}
	out := slices.Clone(a)
	for _, x := range b {
		if !slices.ContainsFunc(out, func(h domain.AskAlbum) bool { return h.MBID == x.MBID }) {
			out = append(out, x)
		}
	}
	return out
}

// openArtistAsks finds the open asks (waiting or approved) for an artist,
// oldest first.
func openArtistAsks(ctx context.Context, q database.Lister, mbid string) ([]domain.Ask, error) {
	return database.Select[domain.Ask](ctx, q, "asks", "kind = 'artist' AND state IN ('asked', 'approved') AND json_extract(source_ids, '$.musicbrainz') = ? ORDER BY id", mbid)
}

// joinWiderArtist adds p to a waiting artist ask for fewer albums and
// widens it to the albums p asked for too (one against the music limit, as
// for shows).
func (s *Service) joinWiderArtist(ctx context.Context, p Person, o domain.Ask, want []domain.AskAlbum) (Result, error) {
	return s.joinWiderWith(ctx, p, o, KindMusic, func(a *domain.Ask) { a.Albums = widenAlbums(a.Albums, want) },
		func(a domain.Ask) map[string]any { return map[string]any{"albums": albumWords(a.Albums)} })
}

// albumsNotKept lists which of the wanted albums (empty: every album the
// artist's kinds include) an artist in the Library does not keep up to
// date. An asked-for album the kinds leave out is refused.
func albumsNotKept(ctx context.Context, db querier, titleID int64, want []domain.AskAlbum) ([]domain.AskAlbum, error) {
	var artistKeep bool
	if err := db.QueryRowContext(ctx, albumsNotKeptSQL, titleID).Scan(&artistKeep); err != nil {
		return nil, err
	}
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL ORDER BY release_date IS NULL, release_date, id", titleID)
	if err != nil {
		return nil, err
	}
	out := []domain.AskAlbum{}
	if len(want) == 0 {
		for _, p := range parts {
			if p.Included && (!p.KeepUpToDate || !artistKeep) {
				out = append(out, partAlbum(p))
			}
		}
		return out, nil
	}
	for _, w := range want {
		i := slices.IndexFunc(parts, func(p mediafiles.Part) bool { return p.SourceIDs["musicbrainz"] == w.MBID })
		if i < 0 {
			// An album the Library does not know yet could never be kept
			// up to date by approving: refused until a refresh lists it.
			return nil, ErrAlbumNotListedYet
		}
		if !parts[i].Included {
			return nil, ErrAlbumNotIncluded
		}
		if !parts[i].KeepUpToDate || !artistKeep {
			out = append(out, w)
		}
	}
	return out, nil
}

func partAlbum(p mediafiles.Part) domain.AskAlbum {
	a := domain.AskAlbum{MBID: p.SourceIDs["musicbrainz"], Name: p.Name}
	if p.ReleaseDate != nil {
		a.Year = p.ReleaseDate.Year()
	}
	return a
}

// musicReady says whether every asked-for album (empty: every included,
// kept up to date album that is out) has all its tracks in the main
// Version, with at least one such album and each album at least one track.
func musicReady(ctx context.Context, db querier, titleID int64, albums []string, now time.Time) (bool, error) {
	where, args := "title_id = ? AND kind = 'album' AND dropped_at IS NULL", []any{titleID}
	if len(albums) == 0 {
		where += " AND included = 1 AND keep_up_to_date = 1 AND release_date IS NOT NULL AND release_date <= ?"
		args = append(args, database.FormatTime(now))
	} else {
		where += " AND json_extract(source_ids, '$.musicbrainz') IN (?" + strings.Repeat(", ?", len(albums)-1) + ")"
		for _, id := range albums {
			args = append(args, id)
		}
	}
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", where, args...)
	if err != nil || len(parts) == 0 || (len(albums) > 0 && len(parts) < len(albums)) {
		return false, err
	}
	for _, p := range parts {
		var tracks, ready int
		err := db.QueryRowContext(ctx, musicReadySQL, p.ID).Scan(&tracks, &ready)
		if err != nil {
			return false, err
		}
		if tracks == 0 || ready < tracks {
			return false, nil
		}
	}
	return true, nil
}

// addArtistForAsk adds the asked-for artist (or finds it in the Library)
// and keeps the asked-for albums up to date. A new artist's album kinds
// are the music settings' widened by the kinds of the albums asked for, so
// every asked-for album is included; nothing is searched for yet.
func (s *Service) addArtistForAsk(ctx context.Context, a domain.Ask, set Settings, opt ApproveOptions, by Person) (int64, error) {
	mbid := a.SourceIDs["musicbrainz"]
	db := s.Store.Read()
	tid, err := library.ArtistByMBID(ctx, db, mbid)
	if err != nil {
		return 0, err
	}
	added := false // a new artist is searched at the end of its own add
	if tid == 0 {
		defs, err := library.ReadMusicDefaults(ctx, db)
		if err != nil {
			return 0, err
		}
		o := library.ArtistAddOptions{LibraryFolderID: opt.LibraryFolderID, PickRulesID: opt.PickRulesID, Albums: library.AlbumsAll, WhyAdded: "ask",
			NewAlbums: defs.NewAlbums, AlbumTypes: defs.AlbumTypes, SecondaryTypes: defs.SecondaryTypes, ReleaseStatuses: defs.ReleaseStatuses}
		if len(a.Albums) > 0 {
			o.Albums = "none"
			if s.Library.Artists == nil {
				return 0, library.ErrNoMusicSource
			}
			ar, err := s.Library.Artists.Artist(ctx, mbid)
			if err != nil {
				return 0, err
			}
			o.AlbumTypes, o.SecondaryTypes = slices.Clone(o.AlbumTypes), slices.Clone(o.SecondaryTypes)
			for _, al := range ar.Albums {
				if !slices.ContainsFunc(a.Albums, func(w domain.AskAlbum) bool { return w.MBID == al.MBID }) {
					continue
				}
				if al.PrimaryType != "" && !slices.Contains(o.AlbumTypes, al.PrimaryType) {
					o.AlbumTypes = append(o.AlbumTypes, al.PrimaryType)
				}
				extra := al.SecondaryTypes
				if len(extra) == 0 {
					extra = []string{"studio"}
				}
				for _, x := range extra {
					if !slices.Contains(o.SecondaryTypes, x) {
						o.SecondaryTypes = append(o.SecondaryTypes, x)
					}
				}
			}
		}
		if o.LibraryFolderID == 0 {
			o.LibraryFolderID = set.MusicLibraryFolderID
		}
		if o.PickRulesID == 0 {
			o.PickRulesID = set.MusicPickRulesID
		}
		// A new artist's tracks come with its MusicBrainz refresh: the search
		// waits for that (and the scan) in the add's own chain.
		o.SearchAfter = set.SearchOnApprove
		var dup *library.ArtistAlreadyAddedError
		tid, _, err = s.Library.AddArtist(ctx, "mbid:"+mbid, o, by.actor())
		switch {
		case errors.As(err, &dup):
			tid = dup.TitleID
		case err != nil:
			return 0, err
		case len(a.Albums) == 0:
			return tid, nil
		default:
			added = true
		}
	}
	var artistKeep bool
	if err := db.QueryRowContext(ctx, addArtistForAskSQL, tid).Scan(&artistKeep); err != nil {
		return 0, err
	}
	if !artistKeep {
		on := true
		if _, err := s.Library.Edit(ctx, tid, library.TitleChange{KeepUpToDate: &on}, false, by.actor()); err != nil {
			return 0, err
		}
	}
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL ORDER BY id", tid)
	if err != nil {
		return 0, err
	}
	// An artist already in the Library may leave an approved album out by
	// its album kinds: approving takes just that album in (the kinds stay).
	var widen []int64
	for _, p := range parts {
		if !p.Included && slices.ContainsFunc(a.Albums, func(w domain.AskAlbum) bool { return w.MBID == p.SourceIDs["musicbrainz"] }) {
			widen = append(widen, p.ID)
		}
	}
	// Albums taken in get their tracks with the refresh that follows: an
	// artist already in the Library is searched after it.
	searchAfter := set.SearchOnApprove && !added && len(widen) > 0
	if len(widen) > 0 {
		if err := s.Library.IncludeAlbumsThen(ctx, tid, widen, searchAfter, by.actor()); err != nil {
			return 0, err
		}
		if parts, err = database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL ORDER BY id", tid); err != nil {
			return 0, err
		}
	}
	on := true
	for _, p := range parts {
		asked := len(a.Albums) == 0 && p.Included ||
			slices.ContainsFunc(a.Albums, func(w domain.AskAlbum) bool { return w.MBID == p.SourceIDs["musicbrainz"] })
		// Albums taken in above are already kept up to date (in the same
		// write); they still get the edition picked on the album page.
		if !asked || p.KeepUpToDate && !slices.Contains(widen, p.ID) {
			continue
		}
		if !p.KeepUpToDate {
			if _, err := s.Library.SetAlbum(ctx, p.ID, &on, 0, by.actor()); err != nil {
				return 0, err
			}
		}
		if ed := askedEdition(p.SourceIDs["musicbrainz"], a.Albums); ed != "" {
			// The edition picked on the album page; an edition the source no
			// longer lists leaves the usual choice.
			if _, err := s.Library.ChooseEdition(ctx, p.ID, ed, by.actor()); err != nil {
				log.Printf("asks: edition %s of %s: %v", ed, p.Name, err)
			}
		}
	}
	// An artist already in the Library has its kept albums' tracks: look
	// for them at once (a new artist and widened albums wait as above).
	if set.SearchOnApprove && !added && len(widen) == 0 && s.Library.Queue != nil {
		if err := s.Library.ScanThenSearch(ctx, tid, by.actor()); err != nil {
			log.Printf("asks: search for artist %d: %v", tid, err)
		}
	}
	return tid, nil
}

// readyArtists finds approved artist asks whose asked-for albums are all
// in the Library (every track has a file).
func (s *Service) readyArtists(ctx context.Context) ([]domain.Ask, error) {
	as, err := database.Select[domain.Ask](ctx, s.Store.Read(), "asks", "kind = 'artist' AND state = 'approved' AND title_id IS NOT NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []domain.Ask
	for _, a := range as {
		ok, err := musicReady(ctx, s.Store.Read(), *a.TitleID, albumIDs(a.Albums), s.now())
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// albumWords says which albums an ask is for: "every album", "Harbour
// Lights", "A and B", "A, B and C" or "A, B and 3 more albums".
func albumWords(albums []domain.AskAlbum) string {
	n := len(albums)
	switch {
	case n == 0:
		return "every album"
	case n == 1:
		return albums[0].Name
	case n <= 3:
		names := make([]string, n)
		for i, a := range albums {
			names[i] = a.Name
		}
		return strings.Join(names[:n-1], ", ") + " and " + names[n-1]
	}
	return albums[0].Name + ", " + albums[1].Name + " and " + strconv.Itoa(n-2) + " more albums"
}

// askedEdition is the edition the ask picked for this album, or "".
func askedEdition(album string, as []domain.AskAlbum) string {
	i := slices.IndexFunc(as, func(w domain.AskAlbum) bool { return strings.EqualFold(w.MBID, album) })
	if i < 0 {
		return ""
	}
	return as[i].Edition
}
