package music

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

// musicCursorSecret is the cursor signing key: 32 random bytes kept in the
// settings table, made on first use.
func (h *Handlers) musicCursorSecret(ctx context.Context) ([]byte, error) {
	h.exploreCache.cursorMu.Lock()
	defer h.exploreCache.cursorMu.Unlock()
	if h.exploreCache.cursorKey != nil {
		return h.exploreCache.cursorKey, nil
	}
	const key = "music_cursor_key"
	var v string
	err := h.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	if err == sql.ErrNoRows || err == nil && v == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		v = base64.RawURLEncoding.EncodeToString(b)
		now := database.FormatTime(time.Now())
		by, _ := json.Marshal(domain.System)
		err = h.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?) ON CONFLICT (grp, key) DO NOTHING`, key, v, now, string(by), now, string(by))
			return err
		})
		if err == nil {
			err = h.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
		}
	}
	if err != nil {
		return nil, err
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil || len(b) < 32 {
		return nil, fmt.Errorf("music cursor key unreadable")
	}
	h.exploreCache.cursorKey = b
	return b, nil
}

func (h *Handlers) musicCursorSign(ctx context.Context, c musicCursor) (string, error) {
	k, err := h.musicCursorSecret(ctx)
	if err != nil {
		return "", err
	}
	body := fmt.Sprintf("%d|%s|%s|%d|%d", c.Member, c.Date, c.Region, c.Version, c.Offset)
	m := hmac.New(sha256.New, k)
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString([]byte(body)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

func (h *Handlers) musicCursorOpen(ctx context.Context, s string) (musicCursor, bool) {
	var c musicCursor
	k, err := h.musicCursorSecret(ctx)
	if err != nil {
		return c, false
	}
	body64, sig64, ok := strings.Cut(s, ".")
	if !ok {
		return c, false
	}
	body, err1 := base64.RawURLEncoding.DecodeString(body64)
	sig, err2 := base64.RawURLEncoding.DecodeString(sig64)
	if err1 != nil || err2 != nil {
		return c, false
	}
	m := hmac.New(sha256.New, k)
	m.Write(body)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return c, false
	}
	f := strings.Split(string(body), "|")
	if len(f) != 5 {
		return c, false
	}
	c.Member, err1 = strconv.ParseInt(f[0], 10, 64)
	c.Date, c.Region = f[1], f[2]
	c.Version, err2 = strconv.Atoi(f[3])
	off, err3 := strconv.Atoi(f[4])
	c.Offset = off
	return c, err1 == nil && err2 == nil && err3 == nil && off > 0 && off%musicscore.PlanBatch == 0
}

// exploreMusicPage answers GET /api/v1/explore/music with the day's page plan:
// page 1 (rows 1–6, the Spotlight and the asks row) or, with ?cursor=, the
// next 6 rows of the same pinned plan.
func (h *Handlers) exploreMusicPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	if h.exploreCache.now != nil {
		now = h.exploreCache.now().UTC()
	}
	member := h.Person(r).UserID
	region := strings.ToUpper(r.URL.Query().Get("region"))
	if region == "" || region == "GLOBAL" || !isCountry(region) {
		region = "global"
	}
	key := musicscore.PlanKey{Member: member, Date: musicscore.Day(now), View: region}
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	acts, err := h.musicActsOf(ctx, member)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	offset := 0
	var p *musicscore.Plan
	if cur := r.URL.Query().Get("cursor"); cur != "" {
		c, ok := h.musicCursorOpen(ctx, cur)
		if ok && c.Member == member && c.Region == region {
			// The plan's own day: a last good plan of yesterday pages on.
			p, ok = h.exploreCache.plans.Get(musicscore.PlanKey{Member: member, Date: c.Date, View: region}, c.Version, now)
		}
		if !ok || p == nil {
			webserver.Fail(w, http.StatusBadRequest, "these rows expired; load Music again")
			return
		}
		offset = c.Offset
	} else {
		h.rememberMusicAsk(member, region, now)
		p = h.musicPlanFor(ctx, member, region, key, lib, acts, now)
	}
	ord := p.Meta.(*musicOrder)
	var rows []musicscore.Row
	more := false
	if p.Version == 0 {
		// A plan cut short at the visitor's deadline is not kept: no cursor.
		rows = p.All(nil)
	} else if offset == 0 && p.Total() <= musicFew {
		rows = p.All(nil)
	} else {
		rows, more = p.Page(offset/musicscore.PlanBatch, nil)
	}
	state := func(c MusicCard) MusicCard {
		c.State = ""
		if c.MBID != "" && c.Kind == "album" {
			if s := acts.albumState[c.MBID]; s != "" {
				c.State = s
			}
		}
		if c.State == "" && c.ArtistMBID != "" {
			if s := acts.artistState[c.ArtistMBID]; s != "" {
				c.State = s
			}
		}
		if c.Kind == "album" && c.PartID != 0 && lib.hadParts[c.PartID] {
			c.State = "have"
		}
		return c
	}
	out := make([]MusicRow, 0, len(rows)+1)
	head := 0
	for _, row := range rows {
		m := ord.v7.metas[row.ID]
		m.Items = make([]MusicCard, 0, len(row.Items))
		for _, it := range row.Items {
			m.Items = append(m.Items, state(ord.v7.cards[row.ID][it.Key]))
		}
		out = append(out, m)
		if offset == 0 && (row.ID == "new_from" || row.ID == "genres") && len(out) == head+1 {
			head++
		}
	}
	if offset == 0 {
		asksRow := MusicRow{ID: "your_asks", Type: "your_asks", Title: "Your asks in progress", Reason: "Albums you asked for, newest first", Shape: "square"}
		for _, m := range acts.mine {
			c := MusicCard{Kind: "album", MBID: m.albumMBID, Name: m.album, Artist: m.artist, ArtistMBID: m.artistMBID, State: m.state, Date: dayOfText(m.at), Genres: []string{}}
			c.Key = "album:" + m.albumMBID
			if m.albumMBID == "" {
				c.Kind, c.MBID, c.Name, c.Key = "artist", m.artistMBID, m.artist, "artist:"+m.artistMBID
			} else {
				c.Image = "/api/v1/music-trending/art/album/" + m.albumMBID
			}
			if len(asksRow.Items) < musicscore.RowMax {
				asksRow.Items = append(asksRow.Items, c)
			}
		}
		if len(asksRow.Items) > 0 {
			out = append(out[:head], append([]MusicRow{asksRow}, out[head:]...)...)
		}
	}
	sinkAsked(out)
	next := ""
	if more {
		next, err = h.musicCursorSign(ctx, musicCursor{Member: member, Date: p.Date, Region: region, Version: p.Version, Offset: offset + musicscore.PlanBatch})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	h.rememberMusicCards(out)
	if more {
		// The next page's albums and artists are fetched ahead while this
		// one is read (nothing while Music's loop is off).
		nextPage := offset/musicscore.PlanBatch + 1
		go func() { h.queueAhead(musicPagesAhead(p, nextPage, 1)) }()
	}
	h.ExploreCaching(w, r, "private, no-cache")
	if offset > 0 {
		h.WriteMusicJSON(w, r, map[string]any{"v2": true, "v7": true, "rows": out, "next": next, "total": p.Total()})
		return
	}
	var hero *MusicCard
	if ord.Hero != nil {
		hv := state(*ord.Hero)
		hero = &hv
	}
	alts := make([]MusicCard, 0, len(ord.Alts))
	for _, c := range ord.Alts {
		alts = append(alts, state(c))
	}
	regions := ord.Regions
	if regions == nil {
		regions = []string{}
	}
	if hero != nil {
		h.rememberMusicCards(nil, append([]MusicCard{*hero}, alts...)...)
	}
	h.WriteMusicJSON(w, r, map[string]any{"v2": true, "v7": true, "kid": false, "cold": ord.Cold, "partial": p.Partial, "genres": []string{}, "genre": "", "region": region, "regions": regions, "hero": hero, "hero_alts": alts, "rows": out, "next": next, "total": p.Total()})
}

// prewarmMusicPlan runs a second after Music starts and after each Music
// read-ahead prebuild: one household build (member 0, worldwide) fills
// layer 1 (memory and disk) with the artist data, similarity and listeners
// the v7 families read, so a member's first page of the day needs few or no
// metadata requests. The household plan is kept like a visit's.
func (h *Handlers) prewarmMusicPlan(ctx context.Context) {
	if h.Store == nil {
		return
	}
	now := time.Now().UTC()
	lib, err := h.musicLibrary(ctx, now)
	if err != nil {
		return
	}
	acts, err := h.musicActsOf(ctx, 0)
	if err != nil {
		return
	}
	key := musicscore.PlanKey{Member: 0, Date: musicscore.Day(now), View: "global"}
	p := h.buildMusicPlan(ctx, 0, "global", key, lib, acts, now)
	// The member and region pairs actually asked for lately are built ahead
	// too, so their first visit of the day reads a ready plan.
	defer h.prewarmMusicAsked(ctx, key.Date, lib, now)
	if p != nil && p.Meta != nil {
		// The first pages' albums and artists: details and pictures ahead.
		// Every row of the plan: list rows show art at once on any page.
		h.queueAhead(musicPagesAhead(p, 0, musicscore.PlanMax/musicscore.PlanBatch))
	}
}

// prewarmMusicAsked builds today's plan of each recently asked member and
// region that has none yet.
func (h *Handlers) prewarmMusicAsked(ctx context.Context, date string, lib *musicLib, now time.Time) {
	for _, k := range h.musicAsked(now) {
		if ctx.Err() != nil {
			return
		}
		pk := musicscore.PlanKey{Member: k.member, Date: date, View: k.region}
		if _, ok := h.exploreCache.plans.Current(pk, now); ok || (k.member == 0 && k.region == "global") {
			continue
		}
		if acts, err := h.musicActsOf(ctx, k.member); err == nil {
			h.buildMusicPlan(ctx, k.member, k.region, pk, lib, acts, now)
		}
	}
}

// musicWait is the longest a visitor waits for a plan build; past it the
// last good plan (or, with none, what the build has so far) is answered.
var musicWait = 1500 * time.Millisecond

// musicPartialRetry is how soon a plan that lost a source is rebuilt
// behind while it keeps being served.
const musicPartialRetry = time.Minute

// musicPlanFor answers page 1's plan without building inline while any
// earlier plan exists: the current plan; else the last good plan (stale,
// partial or yesterday's, in memory or on disk) with a rebuild behind;
// else a build cut at musicWait.
func (h *Handlers) musicPlanFor(ctx context.Context, member int64, region string, key musicscore.PlanKey, lib *musicLib, acts *musicActs, now time.Time) *musicscore.Plan {
	if p, ok := h.exploreCache.plans.Current(key, now); ok {
		if p.Partial && now.Sub(p.Built) >= musicPartialRetry {
			h.refreshMusicPlan(member, region, key, now)
		}
		return p
	}
	if old, ok := h.exploreCache.plans.Newest(member, region, now); ok {
		done := h.refreshMusicPlan(member, region, key, now)
		if p := h.waitMusicPlan(done, key, now); p != nil {
			return p
		}
		return old
	}
	if snap, current, ok := h.loadMusicSnap(ctx, member, region, key.Date, lib, now); ok {
		// Kept on disk (a restart): served at once, rebuilt behind when old
		// (the rebuild is answered when it is done within the deadline).
		if !current {
			if p := h.waitMusicPlan(h.refreshMusicPlan(member, region, key, now), key, now); p != nil {
				return p
			}
		}
		ord := snap.order()
		p := musicscore.NewPlan(ord.v7.rows, h.musicHeadRows(), musicPlanMax, member, key.Date, ord.v7.seen)
		p.Meta = ord
		h.exploreCache.plans.Put(key, p, now)
		if !current {
			h.exploreCache.plans.MarkStaleKey(key)
			h.refreshMusicPlan(member, region, key, now)
		}
		return p
	}
	// Nothing kept at all: the full build runs behind; the visitor gets it
	// if it is done in time, else a build cut at the deadline.
	done := h.refreshMusicPlan(member, region, key, now)
	dctx, cancel := context.WithTimeout(ctx, musicWait)
	defer cancel()
	// Most builds read kept data and are done well within the deadline; only
	// a slow one gets a second, cut build beside it.
	grace := time.NewTimer(musicWait / 3)
	defer grace.Stop()
	select {
	case <-done:
		if p, ok := h.exploreCache.plans.Current(key, now); ok {
			return p
		}
	case <-grace.C:
	}
	cut := make(chan *musicscore.Plan, 1)
	go func() {
		ord := h.buildMusicOrder(context.WithValue(dctx, musicMissKey{}, &musicMiss{}), member, region, lib, acts, now)
		if ord.v7 == nil {
			ord.v7 = &musicCand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}
		}
		p := musicscore.NewPlan(ord.v7.rows, h.musicHeadRows(), musicPlanMax, member, key.Date, ord.v7.seen)
		p.Meta = ord
		p.Partial = true
		cut <- p
	}()
	// The full build is preferred while the deadline lasts.
	select {
	case <-done:
		if p, ok := h.exploreCache.plans.Current(key, now); ok {
			return p
		}
	case <-dctx.Done():
	}
	t := time.NewTimer(300 * time.Millisecond)
	defer t.Stop()
	select {
	case p := <-cut:
		return p
	case <-t.C:
		p := musicscore.NewPlan(nil, nil, musicPlanMax, member, key.Date, nil)
		p.Meta = &musicOrder{v7: &musicCand{metas: map[string]MusicRow{}, cards: map[string]map[string]MusicCard{}}}
		p.Partial = true
		return p
	}
}

// waitMusicPlan waits up to musicWait for a rebuild and answers its plan.
func (h *Handlers) waitMusicPlan(done <-chan struct{}, key musicscore.PlanKey, now time.Time) *musicscore.Plan {
	t := time.NewTimer(musicWait)
	defer t.Stop()
	select {
	case <-done:
		if p, ok := h.exploreCache.plans.Current(key, now); ok {
			return p
		}
	case <-t.C:
	}
	return nil
}

// musicRecent is how long an asked member and region stay prewarmed.
const (
	musicRecent    = 7 * 24 * time.Hour
	musicRecentMax = 16
)

type musicAskKey struct {
	member int64
	region string
}

// rememberMusicAsk notes that member asked for region's rows, so the
// prewarm builds that plan ahead next time.
func (h *Handlers) rememberMusicAsk(member int64, region string, now time.Time) {
	h.exploreCache.asked.Store(musicAskKey{member, region}, now)
}

// musicAsked answers the recently asked member and region pairs, newest
// first, at most musicRecentMax.
func (h *Handlers) musicAsked(now time.Time) []musicAskKey {
	type at struct {
		k musicAskKey
		t time.Time
	}
	var l []at
	h.exploreCache.asked.Range(func(k, v any) bool {
		if t := v.(time.Time); now.Sub(t) < musicRecent {
			l = append(l, at{k.(musicAskKey), t})
		} else {
			h.exploreCache.asked.Delete(k)
		}
		return true
	})
	sort.Slice(l, func(i, j int) bool { return l[i].t.After(l[j].t) })
	out := make([]musicAskKey, 0, min(len(l), musicRecentMax))
	for _, e := range l[:min(len(l), musicRecentMax)] {
		out = append(out, e.k)
	}
	return out
}
