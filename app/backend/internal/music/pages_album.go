package music

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func (h *Handlers) albumPage(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("part") {
	case "core":
		h.albumCore(w, r)
		return
	case "near":
		h.albumNear(w, r)
		return
	}
	// Kid members get no music pages, as with the other music routes (explicit marks are incomplete).
	if h.KidMax(r) != "" {
		webserver.Fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	mbid := strings.ToLower(r.PathValue("mbid"))
	artist := strings.ToLower(r.URL.Query().Get("artist"))
	if artist != "" && !metadata.ValidMBID(artist) {
		artist = "" // a bad ?artist= never blocks the page: the album names its own artist
	}
	if !metadata.ValidMBID(mbid) {
		webserver.Fail(w, http.StatusBadRequest, "That album link is not valid.")
		return
	}
	if !h.musicPageOpen(w) {
		return
	}
	kid := h.KidMax(r) != ""
	ctx := metadata.MemberLookup(r.Context())
	// The pressings need only the album id: ask them while the artist is
	// looked up (a bare link's artist too). Both go on after the page stopped waiting (kept for the
	// next ask).
	psFlight := PageFlight(h, ctx, "pressings:"+mbid, func(ctx context.Context) ([]metadata.Pressing, error) {
		return h.lib().Artists.Pressings(ctx, mbid)
	})
	if artist == "" {
		// A bare album link: find the artist from the album itself.
		if artist = h.albumArtistOf(ctx, mbid); artist == "" {
			webserver.Fail(w, http.StatusNotFound, "We could not find this album.")
			return
		}
	}
	by := time.Now().Add(musicPageWait)
	simDone := make(chan []PageArtist, 1)
	go func() {
		if r.URL.Query().Get("similar") == "0" {
			simDone <- []PageArtist{}
			return
		}
		simDone <- h.similarArtists(ctx, artist, 14)
	}()
	art, err, answered := WaitFlight[metadata.Artist](ctx, PageFlight(h, ctx, "album-artist:"+artist+"|"+mbid, func(ctx context.Context) (metadata.Artist, error) {
		art, err := h.lib().Artists.Artist(metadata.QuickArtist(ctx, 1500*time.Millisecond), artist)
		if err == nil && art.Partial && !hasAlbum(art, mbid) {
			art, err = h.lib().Artists.Artist(metadata.WholeArtist(ctx), artist)
		}
		return art, err
	}), by)
	if errors.Is(err, metadata.ErrArtistNotFound) {
		if h.albumWithRealArtist(ctx, w, r, mbid, artist) {
			return
		}
		webserver.Fail(w, http.StatusNotFound, "We could not find this album.")
		return
	}
	// A slow or failing source never fails the page: the album is drawn
	// from the names already kept and the page says it is incomplete.
	incomplete := !answered || err != nil
	if incomplete {
		art = metadata.Artist{MBID: artist, Name: h.knownName("artist", artist)}
	}
	side, cancel := context.WithTimeout(ctx, max(time.Until(by), musicSideFloor))
	defer cancel()
	dz := h.deezerProfile(side, artist, art)
	albums, t, err := h.pageAlbums(ctx, r, art, dz)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if incomplete && t != nil && art.Name == "" {
		art.Name = t.Name
	}
	var me *PageAlbum
	for i := range albums {
		if albums[i].MBID == mbid {
			me = &albums[i]
		}
	}
	if me == nil && incomplete {
		x := PageAlbum{MBID: mbid, Name: h.knownName("album", mbid), SecondaryTypes: []string{}, Image: artPath("album", mbid), Shelf: "albums"}
		me = &x
	}
	if me == nil {
		if h.albumWithRealArtist(ctx, w, r, mbid, artist) {
			return
		}
		webserver.Fail(w, http.StatusNotFound, "We could not find this album.")
		return
	}
	if kid && me.Explicit {
		webserver.Fail(w, http.StatusForbidden, aboveLimit)
		return
	}
	v := AlbumPageView{PageAlbum: *me, Artist: PageArtist{MBID: artist, Name: art.Name, Image: artPath("artist", artist)}, Tracks: []PageTrack{},
		Editions: []PageEdition{}, MoreBy: []PageAlbum{}, Kid: kid}
	if t != nil {
		v.TitleID = t.ID
		if img := artistImage(*t); img != "" {
			v.Artist.Image = img
		}
	}
	ps, psErr, psAnswered := WaitFlight[[]metadata.Pressing](ctx, psFlight, by)
	if !psAnswered && psErr == nil {
		psErr = context.DeadlineExceeded // still asking: the next ask joins it
	}
	if incomplete || psErr != nil && len(ps) == 0 {
		v.Incomplete = true
		if len(ps) == 0 {
			if v.Gap = gapOf(psErr); v.Gap == nil {
				v.Gap = gapOf(err)
			}
		}
	}
	main := mainPressing(ps)
	// Deezer's album: label and explicit marks per track.
	var dzInfo metadata.DeezerAlbumInfo
	// The same name with the same words in brackets first, else the same base name.
	pick := int64(0)
	for _, d := range dz.Albums {
		if fullAlbumKey(d.Title) == fullAlbumKey(me.Name) {
			pick = d.ID
			break
		}
		if pick == 0 && metadata.SongKey(d.Title) == metadata.SongKey(me.Name) {
			pick = d.ID
		}
	}
	if pick != 0 {
		dzInfo = h.deezerAlbum(side, pick)
	}
	v.Label = dzInfo.Label
	have := map[string]string{}
	if me.PartID != 0 {
		have = h.trackStates(ctx, me.PartID)
	}
	if main != nil {
		for _, tr := range main.Tracks {
			pt := PageTrack{Disc: tr.Disc, Number: tr.Number, Name: tr.Name, LengthMS: tr.LengthMS, RecordingID: tr.RecordingID, Explicit: dzInfo.Tracks[metadata.SongKey(tr.Name)]}
			if me.PartID != 0 {
				pt.Have = "missing"
				if s, ok := have[strings.ToLower(tr.RecordingID)]; ok {
					pt.Have = s
				} else if s, ok := have["n:"+metadata.SongKey(tr.Name)]; ok {
					pt.Have = s
				}
			}
			v.LengthMS += tr.LengthMS
			v.Tracks = append(v.Tracks, pt)
		}
		if dzInfo.Explicit {
			v.Explicit = true
		}
	}
	v.Editions = editions(ps)
	for i := range v.Editions {
		v.Editions[i].List = editionTracks(ps, v.Editions[i].MBID, have, me.PartID != 0)
	}
	for _, al := range albums {
		if al.MBID != mbid && (al.Shelf == "albums" || al.Shelf == "singles") && !(kid && al.Explicit) && len(v.MoreBy) < 20 {
			v.MoreBy = append(v.MoreBy, al)
		}
	}
	select {
	case v.Similar = <-simDone:
	case <-time.After(max(time.Until(by), 0)):
		v.Similar = []PageArtist{}
	}
	// The lower row's covers (More by, Fans also like) are fetched now, so
	// they arrive with the page instead of one by one.
	h.warmMusicArt(ArtistPageView{Image: v.Artist.Image, Albums: v.MoreBy, Similar: v.Similar})
	musicPageCaching(w, v.Incomplete)
	h.WriteMusicJSON(w, r, v)
}

// mainPressing picks the edition to list: the earliest official one with
// the most usual track count.
func hasAlbum(art metadata.Artist, mbid string) bool {
	for _, al := range art.Albums {
		if strings.EqualFold(al.MBID, mbid) {
			return true
		}
	}
	return false
}

func mainPressing(ps []metadata.Pressing) *metadata.Pressing {
	count := map[int]int{}
	for _, p := range ps {
		if len(p.Tracks) > 0 {
			count[len(p.Tracks)]++
		}
	}
	var best *metadata.Pressing
	score := func(p metadata.Pressing) (int, int, string) {
		off := 0
		if p.Status == "official" || p.Status == "" {
			off = 2
		}
		if p.Disambiguation == "" {
			off++ // the plain edition before a deluxe or remaster
		}
		d := p.Date
		if d == "" {
			d = "9999"
		}
		return off, count[len(p.Tracks)], d
	}
	for i := range ps {
		if len(ps[i].Tracks) == 0 {
			continue
		}
		if best == nil {
			best = &ps[i]
			continue
		}
		ao, ac, ad := score(ps[i])
		bo, bc, bd := score(*best)
		if ao > bo || ao == bo && (ac > bc || ac == bc && ad < bd) {
			best = &ps[i]
		}
	}
	return best
}

// trackStates reads the main Version's state per track of one album, by
// recording id and by "n:"+name.
func (h *Handlers) trackStates(ctx context.Context, partID int64) map[string]string {
	out := map[string]string{}
	rows, err := h.Store.Read().QueryContext(ctx, `SELECT COALESCE(i.recording_id, ''), COALESCE(i.name, ''), COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = i.title_id ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.part_id = ? AND i.kind = 'track'`, partID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rec, name, st string
		if rows.Scan(&rec, &name, &st) != nil {
			continue
		}
		hv := "missing"
		switch mediafiles.SlotState(st) {
		case mediafiles.SlotInLibrary, mediafiles.SlotBetterWanted:
			hv = "in_library"
		case mediafiles.SlotChosen, mediafiles.SlotDownloading, mediafiles.SlotImporting, mediafiles.SlotSearching:
			hv = "downloading"
		}
		if rec != "" {
			out[strings.ToLower(rec)] = hv
		}
		out["n:"+metadata.SongKey(name)] = hv
	}
	return out
}

// editionWords mark a disambiguation that names an edition a person
// would choose (deluxe, remaster); others name a pressing plant, a
// country or a catalogue number and are left off.
var editionWords = []string{"deluxe", "edition", "remaster", "anniversary", "expanded", "collector", "bonus", "special", "version", "mono", "stereo",
	"clean", "explicit", "box", "super", "complete", "tour", "reissue", "japan", "live", "acoustic", "instrumental", "demo"}

func editionLabel(p metadata.Pressing) string {
	l := p.Name
	d := strings.ToLower(p.Disambiguation)
	for _, w := range editionWords {
		if strings.Contains(d, w) {
			return l + " (" + p.Disambiguation + ")"
		}
	}
	return l
}

// editions lists the album's official editions: one per name and track
// count (pressings of the same edition fold into one, with the earliest
// date), the earliest first.
func editions(ps []metadata.Pressing) []PageEdition {
	out := []PageEdition{}
	at := map[string]int{}
	for _, p := range ps {
		if p.Status != "" && p.Status != "official" || len(p.Tracks) == 0 {
			continue
		}
		l := editionLabel(p)
		k := fullAlbumKey(l) + "|" + strconv.Itoa(len(p.Tracks))
		if i, ok := at[k]; ok {
			if p.Date != "" && (out[i].Date == "" || p.Date < out[i].Date) {
				out[i].Date, out[i].MBID = p.Date, p.MBID
			}
			continue
		}
		at[k] = len(out)
		out = append(out, PageEdition{MBID: p.MBID, Label: l, Tracks: len(p.Tracks), Date: p.Date})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Date, out[j].Date
		if a == "" || b == "" {
			return a != ""
		}
		return a < b
	})
	return out
}
