package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// exploreNewMusic answers GET /api/v1/explore/new/music: New & Popular's
// Music tab. building says the shared lists are still on their way (ask
// again shortly); kid is true for an account with an age limit.
func (h *Handlers) exploreNewMusic(w http.ResponseWriter, r *http.Request) {
	if p, _ := parts.ForTitleKind("artist"); !h.PartIsOn(p) {
		webserver.Fail(w, http.StatusNotFound, "music is turned off")
		return
	}
	empty := map[string]any{"kid": false, "building": false, "hero": []MusicCard{}, "rows": []MusicRow{}, "genres": []string{}}
	// Music has no ratings, so a Kid limit shows none of it (as on Explore → Music).
	if h.KidMax(r) != "" {
		empty["kid"] = true
		webserver.WriteJSON(w, http.StatusOK, empty)
		return
	}
	if !h.Limit(h.Person(r).UserID, time.Now()) {
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusTooManyRequests, "too many Explore requests; try again in a minute")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	h.landAlbumFollows(ctx, now)
	// The row template engine fills New & Popular → Music when it has its
	// catalog (kill switch music_rows_tpl_off keeps the list rows).
	if h.musicTplOn() {
		if hero, rows, ok := h.buildTplWorld(ctx, r, "new", now); ok {
			h.ExploreCaching(w, r, "private, no-cache")
			h.WriteMusicJSON(w, r, map[string]any{"kid": false, "building": false, "hero": hero, "rows": rows, "genres": musicNewTopics(rows, hero)})
			return
		}
	}
	d := h.warmMusicNew()
	if d == nil {
		empty["building"] = true
		h.ExploreCaching(w, r, "private, no-cache")
		webserver.WriteJSON(w, http.StatusOK, empty)
		return
	}
	db := h.Store.Read()
	in := musicNewInput{data: d, owned: map[string]int64{}, images: map[string]string{}, follows: map[string]bool{}}
	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'artist' AND state != 'removed' ORDER BY id")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	byTitle := map[int64]mediafiles.Title{}
	for _, t := range titles {
		byTitle[t.ID] = t
		if id := strings.ToLower(t.SourceIDs["musicbrainz"]); id != "" {
			in.owned[id] = t.ID
			in.images[id] = artistImage(t)
		}
	}
	ps, err := database.Select[mediafiles.Part](ctx, db, "parts", `kind = 'album' AND dropped_at IS NULL AND release_date >= ? AND release_date <= ?
		AND lower(coalesce(release_type, '')) IN ('album', 'ep', 'single') AND secondary_types = '[]' AND coalesce(release_date_precision, 'day') = 'day'
		AND title_id IN (SELECT id FROM titles WHERE kind = 'artist' AND state != 'removed') ORDER BY release_date DESC, id`,
		now.AddDate(0, 0, -musicNewLibrary).Format("2006-01-02"), now.AddDate(0, 0, musicNewAhead).Format("2006-01-02T15:04:05Z"))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, p := range ps {
		t := byTitle[p.TitleID]
		rg := strings.ToLower(p.SourceIDs["musicbrainz"])
		c := MusicCard{Key: "album:" + rg, Kind: "album", MBID: rg, Name: p.Name, Artist: t.Name, ArtistMBID: strings.ToLower(t.SourceIDs["musicbrainz"]),
			TitleID: t.ID, PartID: p.ID, Date: dayOf(p.ReleaseDate), Genres: []string{}, ArtistImage: artistImage(t), Badge: strings.ToLower(derefStr(p.ReleaseType))}
		if rg != "" {
			c.Image = "/api/v1/music-trending/art/album/" + rg
		} else {
			c.Key = "part:" + strings.TrimSpace(p.Name)
		}
		in.library = append(in.library, c)
	}
	if rows, err := db.QueryContext(ctx, "SELECT mbid FROM album_follows WHERE user_id = ?", h.Person(r).UserID); err == nil {
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				in.follows[id] = true
			}
		}
		rows.Close()
	}
	hero, rows := h.musicTrendingRows(ctx, r, in, now)
	hero, rows = h.musicNewListRows(ctx, r, in, hero, rows, now)
	var names []nameKey
	for _, c := range hero {
		names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""}, nameKey{"artist", c.ArtistMBID, c.Artist, "", ""})
	}
	for _, row := range rows {
		for _, c := range row.Items {
			names = append(names, nameKey{"album", c.MBID, c.Name, c.Artist, ""})
		}
	}
	h.rememberMusicNames(names)
	h.ExploreCaching(w, r, "private, no-cache")
	h.WriteMusicJSON(w, r, map[string]any{"kid": false, "building": false, "hero": hero, "rows": rows, "genres": musicNewTopics(rows, hero)})
}

// landAlbumFollows tells people waiting for an album that it is out (an
// in-app message), then ends those follows. At most every 10 minutes.
func (h *Handlers) landAlbumFollows(ctx context.Context, now time.Time) {
	musicNew.mu.Lock()
	if now.Sub(musicNew.landed) < 10*time.Minute {
		musicNew.mu.Unlock()
		return
	}
	musicNew.landed = now
	musicNew.mu.Unlock()
	today := now.Format("2006-01-02")
	_ = h.Store.Write(ctx, func(tx *sql.Tx) error {
		rows, err := tx.Query("SELECT user_id, mbid, name, artist FROM album_follows WHERE release_date <= ?", today)
		if err != nil {
			return err
		}
		type land struct {
			user               int64
			mbid, name, artist string
		}
		var all []land
		for rows.Next() {
			var l land
			if rows.Scan(&l.user, &l.mbid, &l.name, &l.artist) == nil {
				all = append(all, l)
			}
		}
		rows.Close()
		at := database.FormatTime(now)
		for _, l := range all {
			if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text) VALUES (?, ?, 'ready', ?)", at, l.user, l.name+" by "+l.artist+" is out. Ask for it from New & Popular → Music."); err != nil {
				return err
			}
		}
		_, err = tx.Exec("DELETE FROM album_follows WHERE release_date <= ?", today)
		return err
	})
}

// Album follows: an album not out yet, by MusicBrainz release group.

func (h *Handlers) albumFollowState(w http.ResponseWriter, r *http.Request, mbid string) {
	var n int
	if err := h.Store.Read().QueryRowContext(r.Context(), "SELECT count(*) FROM album_follows WHERE user_id = ? AND mbid = ?", h.Person(r).UserID, mbid).Scan(&n); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"mbid": mbid, "following": n > 0})
}

func albumMBID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.ToLower(r.PathValue("mbid"))
	if !metadata.ValidMBID(id) {
		webserver.Fail(w, http.StatusBadRequest, "mbid must be a MusicBrainz release group id")
		return "", false
	}
	return id, true
}

func (h *Handlers) getAlbumFollow(w http.ResponseWriter, r *http.Request) {
	if id, ok := albumMBID(w, r); ok {
		h.albumFollowState(w, r, id)
	}
}

func (h *Handlers) putAlbumFollow(w http.ResponseWriter, r *http.Request) {
	id, ok := albumMBID(w, r)
	if !ok {
		return
	}
	if h.KidMax(r) != "" {
		webserver.Fail(w, http.StatusForbidden, "music is not shown on this account")
		return
	}
	var body struct {
		Name        string `json:"name"`
		Artist      string `json:"artist"`
		ReleaseDate string `json:"release_date"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		webserver.Fail(w, http.StatusBadRequest, `send {"name": "...", "artist": "...", "release_date": "YYYY-MM-DD"}`)
		return
	}
	body.Name, body.Artist = strings.TrimSpace(body.Name), strings.TrimSpace(body.Artist)
	days, okDate := daysFrom(body.ReleaseDate, time.Now().UTC())
	if body.Name == "" || body.Artist == "" || len(body.Name) > 300 || len(body.Artist) > 300 || !okDate {
		webserver.Fail(w, http.StatusBadRequest, "name, artist and release_date (YYYY-MM-DD) are needed")
		return
	}
	if days >= 0 {
		webserver.Fail(w, http.StatusConflict, "that album is out already")
		return
	}
	if err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO album_follows (user_id, mbid, name, artist, release_date, created_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, h.Person(r).UserID, id, body.Name, body.Artist, body.ReleaseDate, database.FormatTime(time.Now()))
		return err
	}); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.albumFollowState(w, r, id)
}

func (h *Handlers) deleteAlbumFollow(w http.ResponseWriter, r *http.Request) {
	id, ok := albumMBID(w, r)
	if !ok {
		return
	}
	if err := h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM album_follows WHERE user_id = ? AND mbid = ?", h.Person(r).UserID, id)
		return err
	}); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.albumFollowState(w, r, id)
}
