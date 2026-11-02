package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbdump"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
	upnet "github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

// openPictures opens the picture store and its sources: Apple Music (with
// its key), Deezer, the Cover Art Archive, Spotify (with its key),
// fanart.tv and YouTube stills of the artist's checked videos.
func (sp *speed) openPictures(o *api.Options, pace *hostPace, upstream *budget.Budget, userAgent string) error {
	res := &imgstore.Resolver{HTTP: pace.paced(publicClient(upstream, 20*time.Second)), UserAgent: userAgent, On: sp.ctl.Provider,
		Info: sp.subject, Videos: sp.videoIDs, Keys: imageKeys(os.Getenv)}
	sp.res = res
	// Deezer is matched by IDs from the slim MusicBrainz copy.
	res.MB = func(ctx context.Context, kind, mbid string) ([]byte, string, bool) {
		if o.Slim == nil {
			return nil, "", false
		}
		return o.Slim(ctx, kind, mbid)
	}
	// Deezer, Spotify and the Cover Art Archive go over their own copy of
	// the 7-IP egress pool (own paces, apart from MusicBrainz's); Apple
	// stays on this IP.
	if pool, err := upnet.ParseEgress(os.Getenv(upnet.EgressEnv)); err != nil {
		log.Printf("speed: picture pool off: %v", err)
	} else if c := pool.Client(20 * time.Second); c != nil {
		res.PoolHTTP = c
		// The Cover Art Archive too (asks and picture addresses).
		res.HTTP = poolForCAA(res.HTTP, c, poolMeter(pace, upstream))
		log.Printf("speed: Deezer, Spotify and the Cover Art Archive over %d egress paths", pool.N())
	}
	if am, err := imgstore.LoadAppleMusic(os.Getenv, os.ReadFile); err != nil {
		log.Printf("speed: Apple Music off: %v", err)
	} else {
		res.AppleMusic = am
	}
	if o.Fanart != nil {
		fan := o.Fanart
		sp.fanart = fan
		art := func(ctx context.Context, kind, id string) ([]byte, error) {
			b, _, err := fan.Art(ctx, kind, id)
			if errors.Is(err, extras.ErrNotFound) || errors.Is(err, extras.ErrDisabled) {
				return nil, imgstore.ErrNoPicture
			}
			return b, err
		}
		res.Fanart = func(ctx context.Context, mbid string) ([]byte, error) { return art(ctx, "music", mbid) }
		res.TitleFanart = art
	}
	if wd := o.Wikidata; wd != nil {
		res.PersonPhoto = func(ctx context.Context, tmdb string) (string, error) {
			n, _ := strconv.ParseInt(tmdb, 10, 64)
			e, err := wd.Person(ctx, n)
			if errors.Is(err, wikidata.ErrNotFound) {
				return "", nil
			}
			return imgstore.CommonsFile(e.Image), err
		}
		o.PersonPicture = sp.personPicture
	}
	o.TMDBImageClient = &http.Client{Timeout: 20 * time.Second}
	var err error
	sp.imgs, err = imgstore.Open(imgstore.Options{Dir: sp.cfg.ImageDir, Chain: res.Chain, Known: sp.known,
		HTTP: poolForCAA(pace.paced(publicClient(upstream, 30*time.Second)), res.PoolHTTP, poolMeter(pace, upstream)), UserAgent: userAgent, Disk: refresh.DiskUsage,
		AllowURL: imgstore.PictureURL,
		MaxBytes: int64(sp.cfg.ImageMaxGB * (1 << 30)),
		MusicCap: musicCap(sp.slimRoot, &sp.musicUsed, func(ctx context.Context) (int64, error) { return sp.imgs.MusicBytes(ctx) }),
		WantBig: func(kind, mbid string) {
			sp.imgq.Enqueue(context.Background(), "big:"+kind+":"+mbid, bigPriority, false)
		}})
	if err != nil {
		return err
	}
	res.Keep, res.Kept, res.Shared = sp.imgs.KeepID, sp.imgs.KeptID, sp.imgs.SharedID
	res.Limits = imgstore.NewLimits(sp.imgs.SaveQuota)
	res.Limits.SetInterval("applemusic", 100*time.Millisecond)
	res.Limits.LoadQuota(sp.imgs.LoadQuota())
	// /v1/art/music answers the stored pictures; movie and tv stay fanart.tv.
	o.Fanart = &chainArt{fan: sp.fanart, imgs: sp.imgs, base: sp.cfg.PublicBase}
	o.Covers = &storeCovers{sp: sp}
	return nil
}

// storeCovers answers /v1/music/cover from the picture store: the album's
// cover as our own addresses (500 as url, every size under sizes),
// queued at P0 when not stored (answered "later" without waiting). The names an ask brings are never used:
// a cover is only ever searched by the catalog's or MusicBrainz's own names
// (the album's stored answer, asked for first when missing).
type storeCovers struct{ sp *speed }

func (c *storeCovers) Album(ctx context.Context, mbid, _, _ string) ([]byte, string, error) {
	sp := c.sp
	cache := "hit"
	rec, ok := sp.imgs.Record(ctx, imgstore.KindAlbum, mbid)
	if !ok && sp.missed(ctx, imgstore.KindAlbum, mbid) {
		// No source had it: asked again by the weekly pass only.
		return nil, "miss", api.ErrNoCover
	}
	if !ok {
		cache = "miss"
		anon := api.Anonymous(ctx)
		// An anonymous ask for a release group the slim copy lacks is no
		// album: answered at once, nothing queued.
		if anon && sp.slimAbsent != nil && sp.slimAbsent(ctx, "release-group", mbid) {
			return nil, cache, api.ErrNoCover
		}
		// A page open never waits on the sources: the fetch is queued at
		// P0 and the app asks again (202, Retry-After). Anonymous asks go
		// at P4, and every ask is tagged with its caller so the lane is
		// shared fairly.
		p := fetchq.P0Waiting
		if anon {
			p = fetchq.P4Deep
		}
		sp.imgq.Enqueue(fetchq.WithInstall(ctx, api.CallerOf(ctx)), "img:"+imgstore.KindAlbum+":"+mbid, p, false)
	}
	if !ok {
		if st, _ := sp.imgq.State(ctx, "img:"+imgstore.KindAlbum+":"+mbid); st == "dead" {
			return nil, cache, api.ErrNoCover
		}
		return nil, cache, api.ErrCoverLater
	}
	at := func(size string) string {
		return sp.cfg.PublicBase + "/v1/img/album/" + mbid + "/" + size + ".webp?v=" + rec.Version
	}
	info := rec.Info()
	b, err := json.Marshal(map[string]any{"mbid": mbid, "source": rec.Provider, "fetched_at": info.FetchedAt, "url": at("500"),
		"sizes": map[string]string{"250": at("250"), "500": at("500"), "1200": at("1200")}, "picture": info})
	return b, cache, err
}

// imageKeys reads the picture sources' keys from the environment (values
// are never logged): YOUTUBE_API_KEY, SPOTIFY_CLIENT_ID and
// SPOTIFY_CLIENT_SECRET. The fanart.tv source reads its own; the APPLE_*
// settings are read by imgstore.LoadAppleMusic.
func imageKeys(env func(string) string) imgstore.Keys {
	g := func(n string) string { return strings.TrimSpace(env(n)) }
	return imgstore.Keys{YouTube: g("YOUTUBE_API_KEY"), SpotifyID: g("SPOTIFY_CLIENT_ID"), SpotifySecret: g("SPOTIFY_CLIENT_SECRET")}
}

// subject reads what the chains need about a picture: for an artist (and
// its wide background and logo) its stored MusicBrainz answer's name, home
// country and links; for an album its catalog names, else its stored
// answer, and its first artist's home country.
func (sp *speed) subject(ctx context.Context, kind, mbid string) imgstore.Subject {
	if kind != imgstore.KindAlbum {
		return sp.artistSubject(ctx, mbid)
	}
	var s imgstore.Subject
	if a, ok := sp.catalogAlbum(mbid); ok {
		s.Name, s.Title, s.Year = a.Artist, a.Title, a.Year
	}
	if sp.handler == nil {
		s.Known = s.Name != "" && s.Title != ""
		return s
	}
	b, stored := sp.handler.StoredData(ctx, "release-group", mbid)
	var d struct {
		Source struct {
			Title  string `json:"title"`
			Date   string `json:"first-release-date"`
			Credit []struct {
				Name   string `json:"name"`
				Artist struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"artist-credit"`
		} `json:"source_data"`
	}
	if stored && json.Unmarshal(b, &d) == nil && len(d.Source.Credit) > 0 {
		c := d.Source.Credit[0]
		if s.Title == "" {
			s.Title, s.Name = d.Source.Title, c.Artist.Name
			if s.Name == "" {
				s.Name = c.Name
			}
			if len(d.Source.Date) >= 4 {
				s.Year, _ = strconv.Atoi(d.Source.Date[:4])
			}
		}
		if c.Artist.ID != "" {
			s.Country = sp.artistSubject(ctx, strings.ToLower(c.Artist.ID)).Country
		}
	}
	s.Known = s.Name != "" && s.Title != ""
	return s
}

func (sp *speed) artistSubject(ctx context.Context, mbid string) imgstore.Subject {
	if sp.brief != nil {
		// The slim copy's brief read: no full answer (release groups) built.
		if b, ok := sp.brief(ctx, mbid); ok {
			return imgstore.Subject{Known: true, Name: b.Name, Country: b.Country, Links: b.Links}
		}
	}
	if sp.handler == nil {
		return imgstore.Subject{}
	}
	b, ok := sp.handler.StoredData(ctx, "artist", mbid)
	if !ok {
		return imgstore.Subject{}
	}
	var d struct {
		Source struct {
			Name    string `json:"name"`
			Country string `json:"country"`
			Area    struct {
				Codes []string `json:"iso-3166-1-codes"`
			} `json:"area"`
		} `json:"source_data"`
	}
	_ = json.Unmarshal(b, &d)
	c := d.Source.Country
	if c == "" && len(d.Source.Area.Codes) > 0 {
		c = d.Source.Area.Codes[0]
	}
	return imgstore.Subject{Known: true, Name: d.Source.Name, Country: c, Links: sp.links(ctx, mbid)}
}

// known reports whether the stored answer the picture's chain reads is
// there: a cover's, photo's or wide background's MusicBrainz answer (a
// logo, a title's pictures and a person's photo go by ID only).
func (sp *speed) known(ctx context.Context, kind, mbid string) bool {
	switch kind {
	case imgstore.KindAlbum, imgstore.KindArtist, imgstore.KindWide:
		return sp.subject(ctx, kind, mbid).Known
	}
	return true
}

// videoIDs answers the artist's stored music-video IDs, best first.
func (sp *speed) videoIDs(ctx context.Context, mbid string) []string {
	if sp.vids == nil {
		return nil
	}
	b, ok := sp.vids.Get(ctx, mbid)
	if !ok {
		return nil
	}
	var d struct {
		Videos []struct {
			ID string `json:"youtube_id"`
		} `json:"videos"`
	}
	_ = json.Unmarshal(b, &d)
	out := make([]string, 0, len(d.Videos))
	for _, v := range d.Videos {
		out = append(out, v.ID)
	}
	return out
}

// answerKind is the MusicBrainz answer a picture's chain reads.
func answerKind(kind string) string {
	if kind == imgstore.KindAlbum {
		return "release-group"
	}
	return "artist"
}

// runImage runs a picture-queue job: "img:{kind}:{mbid}" walks the
// picture's chain, "big:{kind}:{mbid}" makes a cover's or photo's big size,
// "hv:{mbid}" brings an artist's music videos up to date and "hvsb:{id}"
// runs a video's storyboard check. A picture whose stored answer is
// missing asks for it at the job's own priority; a background job then
// waits for it (so Apple and Deezer can search by its names), while one
// someone waits on runs with the sources that go by ID.
func (sp *speed) runImage(ctx context.Context, key string, p int) (err error) {
	t0 := time.Now()
	ctx, tr := imgstore.WithTrace(ctx)
	defer func() {
		job, _, _ := strings.Cut(key, ":")
		out := "ok"
		switch {
		case errors.Is(err, fetchq.ErrNotFound):
			out = "none"
		case err != nil && strings.Contains(err.Error(), "(mb:"):
			out = "waitmb"
		case errors.Is(err, fetchq.ErrLater) || errors.Is(err, fetchq.ErrConnection):
			out = "later"
		case err != nil:
			out = "err"
		}
		sp.jobs.add(fmt.Sprintf("%s-p%d", job, p), out, time.Since(t0), tr)
	}()
	if isVideoJob(key) {
		defer sp.videoDone()
	}
	if mbid, ok := strings.CutPrefix(key, "hv:"); ok {
		return sp.runVideos(ctx, mbid, p)
	}
	if id, ok := strings.CutPrefix(key, "hvsb:"); ok {
		return sp.vids.DeepCheck(ctx, id)
	}
	job, rest, _ := strings.Cut(key, ":")
	kind, mbid, _ := strings.Cut(rest, ":")
	if job == "art" {
		job, kind, mbid = "img", imgstore.KindArtist, rest // queued by an older build
	}
	ctx = imageJobCtx(ctx, p)
	if job == "img" && p > fetchq.P0Waiting && p < fetchq.P4Deep && sp.isFresh(ctx, kind, mbid) {
		// Stored and not due a re-check (queued before it was stored, or
		// by an older build): nothing to ask the sources.
		return nil
	}
	if job == "big" {
		err = sp.imgs.MakeBig(ctx, kind, mbid)
	} else {
		if !sp.known(ctx, kind, mbid) && sp.handler != nil {
			ak := api.QueueKey(answerKind(kind), mbid)
			st, _ := sp.mbq.State(ctx, ak)
			if st != "dead" {
				sp.mbq.Enqueue(ctx, ak, max(p, fetchq.P1Finish), false)
				if p > fetchq.P1Finish {
					// Never dies of the wait while the answer is queued.
					return sp.mbq.WaitOn(ak)
				}
			}
		}
		err = sp.imgs.Fetch(ctx, kind, mbid)
	}
	switch {
	case errors.Is(err, imgstore.ErrNoPicture):
		return fetchq.ErrNotFound
	case errors.Is(err, imgstore.ErrRetry) && upnet.ConnError(err):
		return fetchq.ErrConnection
	case errors.Is(err, imgstore.ErrRetry):
		if job != "big" && sp.handler != nil && !sp.known(ctx, kind, mbid) {
			return sp.mbq.WaitOn(api.QueueKey(answerKind(kind), mbid))
		}
		// A source was busy or paused (not a wait on another job): say so.
		return fmt.Errorf("%w: a picture source was busy or paused", fetchq.ErrLater)
	}
	return err
}

// imageNow fetches a missing picture at P0 for someone waiting, under the
// asking install's fair share, and waits at most 3 s for it. Without its
// stored answer the picture comes from the sources that go by ID (the
// archive for a cover; MusicBrainz is paced too slowly to wait on) and is
// looked at again within a day, once the answer is there.
func (sp *speed) imageNow(ctx context.Context, kind, mbid string) bool {
	if sp.missed(ctx, kind, mbid) {
		return false
	}
	done := sp.imgq.Enqueue(ctx, "img:"+kind+":"+mbid, fetchq.P0Waiting, true)
	if done == nil {
		return false
	}
	t := time.NewTimer(3 * time.Second)
	defer t.Stop()
	select {
	case err := <-done:
		return err == nil
	case <-t.C:
	case <-ctx.Done():
	}
	return false
}

// refreshPictures re-checks stored pictures whose time has come (Spotify,
// YouTube stills and Wikimedia every 30 days, the rest every 90) and makes
// the big sizes still missing, at low priority: every 10 minutes, at most
// 500 re-checks and 1,500 big sizes (the archive's file hosts allow about
// that many in ten minutes).
func (sp *speed) refreshPictures(ctx context.Context) {
	t := time.NewTimer(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		sp.queueDueMisses(ctx)
		for _, k := range sp.imgs.Due(ctx, 500) {
			sp.imgq.Enqueue(ctx, "img:"+k.Kind+":"+k.ID, fetchq.P4Deep, false)
		}
		// Backfill: artist photos first (BigsDue), then covers.
		for _, k := range sp.imgs.BigsDue(ctx, 1500) {
			sp.imgq.Enqueue(ctx, "big:"+k.Kind+":"+k.ID, bigPriority, false)
		}
		t.Reset(10 * time.Minute)
	}
}

// pictures answers the stored picture records a text answer carries: an
// artist's photo, wide background and logo; an album's cover.
func (sp *speed) pictures(ctx context.Context, kind, mbid string) map[string]imgstore.Info {
	slots := map[string]string{"cover": imgstore.KindAlbum}
	if kind == "artist" {
		slots = map[string]string{"photo": imgstore.KindArtist, "wide": imgstore.KindWide, "logo": imgstore.KindLogo}
	}
	out := map[string]imgstore.Info{}
	for name, k := range slots {
		if r, ok := sp.imgs.Record(ctx, k, mbid); ok {
			out[name] = r.Info()
		}
	}
	return out
}

// personPicture answers our address for a person's Commons photo (the
// store fetches it on the first ask), "" for any other photo.
func (sp *speed) personPicture(ctx context.Context, tmdb, image string) string {
	if imgstore.CommonsFile(image) == "" || !imgstore.ValidID(imgstore.KindPerson, tmdb) {
		return ""
	}
	a := sp.cfg.PublicBase + "/v1/img/person/" + tmdb + "/500.webp"
	if v := sp.imgs.Version(ctx, imgstore.KindPerson, tmdb); v != "" {
		a += "?v=" + v
	}
	return a
}

// chainArt answers /v1/art with our own picture addresses, so no install
// contacts a source: music from the stored pictures; a movie's or show's
// fanart.tv logo, banner and first background through the store (fetched
// on the first ask).
type chainArt struct {
	fan  api.ArtSource
	imgs *imgstore.Store
	base string
}

func (c *chainArt) Art(ctx context.Context, kind, id string) ([]byte, string, error) {
	if kind != "music" {
		if c.fan == nil {
			return nil, "", extras.ErrDisabled
		}
		b, cache, err := c.fan.Art(ctx, kind, id)
		if err != nil {
			return b, cache, err
		}
		var a extras.Art
		if err := json.Unmarshal(b, &a); err != nil {
			return nil, cache, err
		}
		tid := kind + "-" + id
		at := func(k, file string) string {
			u := c.base + "/v1/img/" + k + "/" + tid + "/" + file
			if v := c.imgs.Version(ctx, k, tid); v != "" {
				u += "?v=" + v
			}
			return u
		}
		out := extras.Art{Kind: a.Kind, ID: a.ID, Source: "holyarrs"}
		if a.Logo != "" {
			out.Logo = at(imgstore.KindTitleLogo, "1200.png")
		}
		if a.Banner != "" {
			out.Banner = at(imgstore.KindTitleBanner, "1000.webp")
		}
		if len(a.Backgrounds) > 0 {
			out.Backgrounds = []string{at(imgstore.KindTitleWide, "1600.webp")}
		}
		if out.Logo == "" && out.Banner == "" && len(out.Backgrounds) == 0 {
			return nil, cache, extras.ErrNotFound
		}
		b, err = json.Marshal(out)
		return b, cache, err
	}
	addr := func(k, file string) string {
		r, ok := c.imgs.Record(ctx, k, id)
		if !ok {
			return ""
		}
		return "/v1/img/" + k + "/" + id + "/" + file + "?v=" + r.Version
	}
	a := extras.Art{Kind: "music", ID: id, Source: "holyarrs", Logo: addr(imgstore.KindLogo, "1200.png"), Thumb: addr(imgstore.KindArtist, "1200.webp")}
	if w := addr(imgstore.KindWide, "1920.webp"); w != "" {
		a.Backgrounds = []string{w}
	}
	if a.Thumb != "" {
		a.Thumbs = []string{a.Thumb}
	}
	if a.Logo == "" && a.Thumb == "" && len(a.Backgrounds) == 0 {
		return nil, "miss", extras.ErrNotFound
	}
	b, err := json.Marshal(a)
	return b, "hit", err
}

func (c *chainArt) Status() extras.Status {
	if c.fan == nil {
		return extras.Status{State: "ok"}
	}
	return c.fan.Status()
}

// pictureFlags reads the flags the picture commands share.
func pictureFlags(name string, args []string, extra func(*flag.FlagSet)) (dataDir string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("data-dir", "/data", "server data directory")
	if extra != nil {
		extra(fs)
	}
	if fs.Parse(args) != nil {
		return "", errors.New("invalid command arguments")
	}
	return *dir, nil
}

// purgeProvider deletes every stored picture one provider gave and queues
// each to be filled again (P3) from the next source; with -off the
// provider is also switched off in the kill switch first, so the refill
// skips it.
//
//	metadata purge-provider -provider spotify [-off] [-data-dir /data]
func purgeProvider(args []string) error {
	var provider *string
	var off *bool
	dir, err := pictureFlags("purge-provider", args, func(fs *flag.FlagSet) {
		provider = fs.String("provider", "", "provider name (applemusic, deezer, caa, spotify, fanart, youtube)")
		off = fs.Bool("off", false, "switch the provider off first")
	})
	if err != nil || *provider == "" {
		return errors.New("usage: purge-provider -provider NAME [-off] [-data-dir DIR]")
	}
	ctx := context.Background()
	if *off {
		if err := switchOff(filepath.Join(dir, "control.json"), *provider); err != nil {
			return err
		}
	}
	s, err := imgstore.Open(imgstore.Options{Dir: filepath.Join(dir, "img"), Logf: func(string, ...any) {}})
	if err != nil {
		return err
	}
	defer s.Close()
	keys, err := s.Purge(ctx, *provider)
	if err != nil {
		return err
	}
	q, err := fetchq.Open(filepath.Join(dir, "fetchq-images.sqlite"), fetchq.Options{Name: "images", Rate: 1, Do: func(context.Context, string, int) error { return nil }})
	if err != nil {
		return err
	}
	defer q.Close()
	for _, k := range keys {
		q.Enqueue(ctx, "img:"+k.Kind+":"+k.ID, fetchq.P3Refresh, false)
	}
	fmt.Printf("purged %d pictures from %s; queued to fill again\n", len(keys), *provider)
	return nil
}

// switchOff turns one provider off in the kill switch file.
func switchOff(path, provider string) error {
	v := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &v); err != nil {
			return errors.New("control file unreadable")
		}
	}
	p, _ := v["providers"].(map[string]any)
	if p == nil {
		p = map[string]any{}
	}
	p[provider] = false
	v["providers"] = p
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// refillFromLinks rebuilds pictures from their stored source addresses (no
// keys needed): by default only those whose files are missing, for a store
// restored from the database backup alone.
//
//	metadata refill-from-links [-all] [-data-dir /data]
func refillFromLinks(args []string) error {
	var all *bool
	dir, err := pictureFlags("refill-from-links", args, func(fs *flag.FlagSet) {
		all = fs.Bool("all", false, "rebuild every picture, not only those with missing files")
	})
	if err != nil {
		return errors.New("usage: refill-from-links [-all] [-data-dir DIR]")
	}
	pace := &hostPace{gap: sourceGap}
	s, err := imgstore.Open(imgstore.Options{Dir: filepath.Join(dir, "img"), HTTP: pace.paced(&http.Client{Timeout: 30 * time.Second}), UserAgent: "HolyArrs-Metadata/1.0 (refill)", Logf: func(string, ...any) {}})
	if err != nil {
		return err
	}
	defer s.Close()
	done, failed := 0, 0
	err = s.Refill(context.Background(), !*all, func(k imgstore.Key, err error) {
		if err != nil {
			failed++
		} else {
			done++
		}
		if (done+failed)%500 == 0 {
			fmt.Printf("refilled %d, failed %d\n", done, failed)
		}
	})
	fmt.Printf("refilled %d, failed %d\n", done, failed)
	return err
}

// imageJobCtx marks every picture-queue fetch BigLater: the big size is
// queued (P3) rather than made inline, so the fill spends the one CPU
// slot on the 500 card only and pages keep a core (2026-10-29: inline big
// sizes held both cores and page answers slowed to p50 240 ms).
func imageJobCtx(ctx context.Context, _ int) context.Context {
	return imgstore.BigLater(ctx)
}

// musicCap is the server's 90 GB guard: the slim copies under root plus
// the music pictures. Over it, no new music picture is written. The bytes
// counted go to used for the alarms check (no second count when asked).
func musicCap(root string, used *atomic.Int64, pictures func(ctx context.Context) (int64, error)) func(ctx context.Context) error {
	g := &mbdump.Guard{Root: root, Pictures: pictures}
	return func(ctx context.Context) error {
		n, err := g.Check(ctx)
		if n > 0 && used != nil {
			used.Store(n)
		}
		return err
	}
}

// poolMeter charges a pool call to the upstream budget and counts it in
// the host pace numbers, without the own address's wait.
func poolMeter(pace *hostPace, upstream *budget.Budget) func(*http.Request) error {
	return func(r *http.Request) error {
		if p := budget.Provider(r.URL.Hostname()); p != "" && upstream != nil && !upstream.Take(p) {
			return budget.ErrSpent
		}
		pace.count(r)
		return nil
	}
}

// publicClient is budget.Client over a dialer that refuses loopback,
// private and link-local addresses (upstream.BlockPrivate, on in serve).
func publicClient(b *budget.Budget, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: budget.Transport{Budget: b, Base: upnet.PublicTransport()}}
}
