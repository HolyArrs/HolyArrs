package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/control"
	"github.com/CoderLuii/holyarrs-metadata/internal/discog"
	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/herovideo"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictags"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
	"github.com/CoderLuii/holyarrs-metadata/internal/sharedfill"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"github.com/CoderLuii/holyarrs-metadata/internal/trailers"
	upnet "github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	"github.com/CoderLuii/holyarrs-metadata/internal/vmstat"
)

// speedConfig turns on the music speed system (plan 2026-10-23): the
// fetch-ahead queue, shared-list fill, picture store, hero videos, VM
// tracking and the kill switch.
type speedConfig struct {
	// MBRate is the queue's MusicBrainz rate (default 0.9 a second).
	MBRate float64 `json:"mb_rate"`
	// ImageRate is the picture jobs' start rate (default 20 a second).
	ImageRate float64 `json:"image_rate"`
	// ImageMaxGB caps the picture store (default 120 GB of the VM's 193,
	// leaving room for the databases, backups and Docker images).
	ImageMaxGB float64 `json:"image_max_gb"`
	// PublicBase is the server's public address, for picture addresses in
	// answers (default https://holyarrs-api.coderluii.dev).
	PublicBase string `json:"public_base"`
	// GenreTop is how many artists per genre are kept stored (default 50).
	GenreTop int `json:"genre_top"`
	// ControlFile is the kill switch (default <data_dir>/control.json).
	ControlFile string `json:"control_file"`
	// ImageDir is the picture store (default <data_dir>/img).
	ImageDir string `json:"image_dir"`
	// MetaBrainzRate is every MetaBrainz call together (musicbrainz.org,
	// the Cover Art Archive, ListenBrainz: one gateway, one address) a
	// second (default 1.25).
	MetaBrainzRate float64 `json:"metabrainz_rate"`
}

type speed struct {
	jobs      jobTiming
	videoJobs atomic.Int32
	// fresh reports a stored picture not due a re-check (imgs.Fresh;
	// replaced in tests).
	fresh func(ctx context.Context, kind, mbid string) bool
	// hasMiss and missesDue are imgs.HasMiss and imgs.MissesDue (replaced
	// in tests).
	hasMiss func(ctx context.Context, kind, mbid string) bool
	// brief reads an artist's name, country and links from the slim copy
	// (nil: none; the stored answer is read instead).
	brief     func(ctx context.Context, mbid string) (mbread.Brief, bool)
	missesDue func(ctx context.Context, limit int) []imgstore.Key
	// listsWaiting reports list names waiting for a match; the MusicBrainz
	// queue then leaves them a share (listsShare) of the rate.
	listsWaiting atomic.Pointer[func() bool]
	cfg          speedConfig
	slimRoot     string                                            // slim copy root, counted by the music cap
	musicUsed    atomic.Int64                                      // music data bytes at the last cap check
	slimAbsent   func(ctx context.Context, kind, mbid string) bool // api Server.SlimAbsent
	slimHas      func(ctx context.Context, kind, mbid string) bool // mbread Reader.Has (false with no copy)
	ctl          *control.Switch
	vm           *vmstat.Recorder
	imgs         *imgstore.Store
	vids         *herovideo.Service
	mbq          *fetchq.Queue
	imgq         *fetchq.Queue
	fanart       api.ArtSource
	res          *imgstore.Resolver
	fill         *sharedfill.Filler
	handler      *api.Server
	// catalog is the music catalog: its album covers and its artists'
	// wide backgrounds and logos are stored ahead (storeCatalogCovers).
	catalog  catalogAlbums
	catMu    sync.Mutex
	catNames map[string]musictags.AlbumRef
	catLast  map[string]any
	// disc is the discography store (full release-group lists, album
	// tracks); discLast its last prefetch pass.
	disc     *discog.Store
	discLast discogPassStats
}

// catalogAlbums is the music catalog (the row templates' bundle).
type catalogAlbums interface {
	Albums() []musictags.AlbumRef
	Artists() []string
	Built() time.Time
}

// listsShare is the part of the MusicBrainz rate the shared queue leaves
// to list matching while names wait (about 0.45 calls a second).
const listsShare = 0.5

func (sp *speed) listsReserve() float64 {
	if f := sp.listsWaiting.Load(); f != nil && (*f)() {
		return listsShare
	}
	return 0
}

// pacedTransport makes MusicBrainz calls made outside the provider client
// wait for the same pacing slot, so all MusicBrainz use stays within one rate.
type pacedTransport struct {
	next   http.RoundTripper
	client *providers.Client
}

func (t pacedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "musicbrainz.org" {
		if err := t.client.Pace(r.Context(), "musicbrainz"); err != nil {
			return nil, err
		}
	}
	return t.next.RoundTrip(r)
}

func openSpeed(c config, o *api.Options, upstream *budget.Budget, userAgent string, music *musictrend.Service, newMusic *musictrend.NewMusic) (*speed, error) {
	sc := *c.Speed
	if sc.MBRate <= 0 {
		sc.MBRate = 0.9 * float64(upnet.MBScale())
	}
	if sc.ImageRate <= 0 {
		sc.ImageRate = defaultImageRate
	}
	if sc.ImageMaxGB <= 0 {
		sc.ImageMaxGB = 120
	}
	if sc.PublicBase == "" {
		sc.PublicBase = "https://holyarrs-api.coderluii.dev"
	}
	if sc.GenreTop <= 0 {
		sc.GenreTop = 50
	}
	if sc.ControlFile == "" {
		sc.ControlFile = filepath.Join(c.DataDir, "control.json")
	}
	if sc.ImageDir == "" {
		sc.ImageDir = filepath.Join(c.DataDir, "img")
	}
	if sc.MetaBrainzRate > 0 {
		upnet.Shared.SetGap(time.Duration(float64(time.Second) / sc.MetaBrainzRate))
	}
	sp := &speed{cfg: sc, ctl: control.New(sc.ControlFile), slimRoot: slimRoot(c, os.Getenv)}
	ok := false
	defer func() {
		if !ok {
			sp.Close()
		}
	}()
	var err error
	if sp.vm, err = vmstat.Open(filepath.Join(c.DataDir, "vmstat.sqlite"), vmstat.Options{DiskPath: c.DataDir, Disk: refresh.DiskUsage}); err != nil {
		return nil, err
	}
	// Every picture source keeps its own rate however many picture workers
	// run: the resolver, the store and fanart.tv share one per-host pace.
	pace := &hostPace{gap: sourceGap}
	sp.jobs.extra = pace.waitsLine
	if fa, ok := o.Fanart.(*extras.Fanart); ok && fa != nil {
		fa.HTTP = pace.paced(fa.HTTP)
	}
	if err := sp.openPictures(o, pace, upstream, userAgent); err != nil {
		return nil, err
	}
	ytdlp := c.YTDLP
	if ytdlp == "" {
		ytdlp = "/usr/local/bin/yt-dlp"
	}
	y := &trailers.YTDLP{Builtin: ytdlp, Dir: filepath.Join(c.DataDir, "bin")}
	// Music videos: YouTube Music, channel feeds, frames and Wikidata, each
	// host at its own pace; the Data API (with YOUTUBE_API_KEY) spends the
	// pictures' shared YouTube quota.
	if sp.vids, err = herovideo.Open(filepath.Join(c.DataDir, "herovideo.sqlite"), herovideo.Options{
		HTTP: pace.paced(budget.Client(upstream, 30*time.Second)), UserAgent: userAgent,
		Key: sp.res.Keys.YouTube, Limits: sp.res.Limits, Artist: sp.artistSubject,
		Heatmap: y.Heatmap, VideoJSON: y.VideoJSON, On: sp.ctl.Provider,
		Deep: func(id string) { sp.imgq.Enqueue(context.Background(), "hvsb:"+id, fetchq.P4Deep, false) },
	}); err != nil {
		return nil, err
	}
	lane := func(p int) bool {
		switch p {
		case fetchq.P2Shared:
			return sp.ctl.Lane("p2")
		case fetchq.P3Refresh:
			return sp.ctl.Lane("p3")
		case fetchq.P4Deep:
			return sp.ctl.Lane("p4")
		}
		return true
	}
	if sp.mbq, err = fetchq.Open(filepath.Join(c.DataDir, "fetchq-musicbrainz.sqlite"), fetchq.Options{Name: "musicbrainz", Rate: sc.MBRate, Do: sp.runMB, Lane: lane, Reserve: sp.listsReserve,
		P0Max: func() float64 { return sp.ctl.Get().P0MaxRPS },
		// Two at once, the second kept for P0 and P1 (a waiting user's job
		// never queues behind a walk step); starts still follow MBRate and
		// every call the shared MetaBrainz pace.
		Workers: 1 + upnet.MBScale(),
		// One upstream: when a job cannot connect the queue waits as a whole.
		ConnPause: true}); err != nil {
		return nil, err
	}
	if sp.imgq, err = fetchq.Open(filepath.Join(c.DataDir, "fetchq-images.sqlite"), fetchq.Options{Name: "images", Rate: sc.ImageRate, Do: sp.runImage, Lane: lane,
		// imageWorkers at once (one kept for P0/P1), starts still at
		// ImageRate, music-video jobs capped (holdImage); artist pictures,
		// big sizes and music-video jobs alternate with album covers below P1.
		Workers: imageWorkers, Hold: sp.holdImage, HeavyPrefixes: []string{"img:artist:", "img:wide:", "img:logo:", "big:", "hv:", "hvsb:"}, HeavyEvery: 4}); err != nil {
		return nil, err
	}
	o.Control = sp.ctl
	o.Images = &api.Images{Store: sp.imgs, Want: func(kind, mbid string) { sp.wantImage(kind, mbid, fetchq.P1Finish) }, WantAhead: func(kind, mbid string) { sp.wantImage(kind, mbid, fetchq.P2Shared) }, WantNow: func(ctx context.Context, kind, mbid string) {
		if !sp.missed(ctx, imageKind(kind), mbid) {
			sp.imgq.Enqueue(ctx, "img:"+imageKind(kind)+":"+mbid, fetchq.P0Waiting, false)
		}
	}, WantRow: func(ctx context.Context, kind, mbid string) {
		if !sp.missed(ctx, kind, mbid) {
			sp.imgq.Enqueue(ctx, "img:"+kind+":"+mbid, fetchq.P2Shared, false)
		}
	}, WantBig: func(kind, mbid string) {
		sp.imgq.Enqueue(context.Background(), "big:"+kind+":"+mbid, fetchq.P1Finish, false)
	},
		Now: sp.imageNow, Redirect: func() bool { return sp.ctl.Provider("caa") }}
	o.Pictures = sp.pictures
	o.PicturePending = sp.picturePending
	o.Videos = sp.vids
	o.WantVideos = func(mbid string) { sp.wantVideos(context.Background(), mbid, fetchq.P1Finish) }
	o.OnFetched = sp.onFetched
	// Live MusicBrainz fills keep their tag (with the egress address) in
	// the picture store's source_ids, never in a served answer.
	o.LiveTag = func(kind, mbid string, t srctag.Tag) { sp.imgs.KeepID(kind, mbid, t, "mb-live", t.Ref) }
	o.StatusExtra = sp.status
	lists := map[string]func(context.Context) []sharedfill.Member{}
	if music != nil {
		lists["charts"] = func(ctx context.Context) []sharedfill.Member {
			albums, artists, _ := music.SharedMembers(ctx, sc.GenreTop, time.Now())
			return members(albums, artists)
		}
		lists["genre_tops"] = func(ctx context.Context) []sharedfill.Member {
			_, _, genres := music.SharedMembers(ctx, sc.GenreTop, time.Now())
			var out []sharedfill.Member
			for _, ids := range genres {
				out = append(out, members(nil, ids)...)
			}
			return out
		}
		lists["trending"] = func(context.Context) []sharedfill.Member { return members(nil, music.ChartArtists()) }
	}
	if newMusic != nil {
		lists["new_and_coming"] = func(context.Context) []sharedfill.Member { return members(newMusic.SharedMembers(time.Now())) }
	}
	sp.fill = &sharedfill.Filler{Lists: lists, Enqueue: sp.enqueueShared, Related: relatedFrom(o.Similar), Start: 2 * time.Minute}
	ok = true
	return sp, nil
}

// relatedFrom reads one hop of similar artists (Apple Music's, else
// ListenBrainz sessions; cached a week by the source). A source call that
// missed its cache is followed by a short pause so a pass stays gentle.
func relatedFrom(src api.SimilarArtists) func(context.Context, sharedfill.Member) []sharedfill.Member {
	if src == nil {
		return nil
	}
	return func(ctx context.Context, m sharedfill.Member) []sharedfill.Member {
		body, cache, err := src.Artists(ctx, m.MBID)
		if cache == "miss" {
			select {
			case <-ctx.Done():
			case <-time.After(250 * time.Millisecond):
			}
		}
		if err != nil {
			return nil
		}
		var d struct {
			Artists []struct {
				MBID string `json:"mbid"`
			} `json:"artists"`
		}
		_ = json.Unmarshal(body, &d)
		out := make([]sharedfill.Member, 0, len(d.Artists))
		for _, a := range d.Artists {
			if a.MBID != "" {
				out = append(out, sharedfill.Member{Kind: "artist", MBID: strings.ToLower(a.MBID)})
			}
		}
		return out
	}
}

func members(albums, artists []string) []sharedfill.Member {
	out := make([]sharedfill.Member, 0, len(albums)+len(artists))
	for _, a := range artists {
		out = append(out, sharedfill.Member{Kind: "artist", MBID: a})
	}
	for _, a := range albums {
		out = append(out, sharedfill.Member{Kind: "release-group", MBID: a})
	}
	return out
}

// links reads the url-relation targets of the stored artist answer.
func (sp *speed) links(ctx context.Context, mbid string) []string {
	if sp.handler == nil {
		return nil
	}
	b, ok := sp.handler.StoredData(ctx, "artist", mbid)
	if !ok {
		return nil
	}
	// The stored answer keeps MusicBrainz's own fields under source_data.
	var d struct {
		Source struct {
			Relations []struct {
				URL struct {
					Resource string `json:"resource"`
				} `json:"url"`
			} `json:"relations"`
		} `json:"source_data"`
	}
	_ = json.Unmarshal(b, &d)
	out := make([]string, 0, len(d.Source.Relations))
	for _, r := range d.Source.Relations {
		if r.URL.Resource != "" {
			out = append(out, r.URL.Resource)
		}
	}
	return out
}

func imageKind(kind string) string {
	if kind == "release-group" {
		return imgstore.KindAlbum
	}
	return kind
}

func (sp *speed) wantImage(kind, mbid string, p int) {
	if sp.missed(context.Background(), imageKind(kind), mbid) {
		return
	}
	sp.imgq.Enqueue(context.Background(), "img:"+imageKind(kind)+":"+mbid, p, false)
}

// missed reports a picture no source had at its last ask: only the weekly
// pass (queueDueMisses) asks for it again, never a page open or a fetch.
func (sp *speed) missed(ctx context.Context, kind, mbid string) bool {
	if sp.hasMiss != nil {
		return sp.hasMiss(ctx, kind, mbid)
	}
	return sp.imgs != nil && sp.imgs.HasMiss(ctx, kind, mbid)
}

// queueDueMisses is the weekly re-ask: misses whose week is over go back
// through the whole source chain at P4 (at most 1,000 a pass).
func (sp *speed) queueDueMisses(ctx context.Context) {
	due := sp.missesDue
	if due == nil {
		due = sp.imgs.MissesDue
	}
	for _, k := range due(ctx, 1000) {
		sp.imgq.Enqueue(ctx, "img:"+k.Kind+":"+k.ID, fetchq.P4Deep, false)
	}
}

// enqueueShared queues a shared-list member, its pictures not stored yet
// (re-checks of stored ones are the refresh loop's) and an artist's videos
// when none are stored.
func (sp *speed) enqueueShared(ctx context.Context, m sharedfill.Member) {
	sp.mbq.Enqueue(ctx, api.QueueKey(m.Kind, m.MBID), fetchq.P2Shared, false)
	kinds := []string{imageKind(m.Kind)}
	if m.Kind == "artist" {
		kinds = append(kinds, imgstore.KindWide, imgstore.KindLogo)
	}
	for _, k := range kinds {
		if sp.imgs.Version(ctx, k, m.MBID) == "" {
			// Shared-list pictures go after the catalog's (P3; catalog P2).
			sp.wantImage(k, m.MBID, fetchq.P3Refresh)
		}
	}
	if m.Kind == "artist" {
		sp.queueAlbums(ctx, m.MBID)
		if _, ok := sp.vids.Get(ctx, m.MBID); !ok {
			sp.wantVideos(ctx, m.MBID, fetchq.P2Shared)
		}
	}
}

// onFetched follows a fetched artist or album with its picture (P1, when
// not stored fresh) and an
// artist with its videos when none are stored and, when it is on a shared
// list, its albums (deep fill).
func (sp *speed) onFetched(kind, mbid string) {
	ctx := context.Background()
	switch kind {
	case "artist":
		sp.wantMissingImage(ctx, kind, mbid)
		if _, ok := sp.vids.Get(ctx, mbid); !ok {
			sp.wantVideos(ctx, mbid, fetchq.P1Finish)
		}
		sp.queueAlbums(ctx, mbid)
		if sp.disc != nil && sp.disc.NeedsArtist(ctx, mbid) {
			// Most lean artist answers hold every release group already:
			// the walk then takes it over without a call.
			sp.mbq.Enqueue(ctx, discog.ArtistKey(mbid), fetchq.P3Refresh, false)
		}
	case "release-group":
		sp.wantMissingImage(ctx, kind, mbid)
	}
}

// wantMissingImage queues a picture at P1 unless it is stored and not due
// a re-check (re-checks are refreshPictures', at P4).
func (sp *speed) wantMissingImage(ctx context.Context, kind, mbid string) {
	if !sp.isFresh(ctx, imageKind(kind), mbid) {
		sp.wantImage(kind, mbid, fetchq.P1Finish)
	}
}

func (sp *speed) isFresh(ctx context.Context, kind, mbid string) bool {
	if sp.fresh != nil {
		return sp.fresh(ctx, kind, mbid)
	}
	return sp.imgs != nil && sp.imgs.Fresh(ctx, kind, mbid)
}

// wantVideos queues an artist's music-video job on the picture queue (it
// calls no MusicBrainz).
func (sp *speed) wantVideos(ctx context.Context, mbid string, p int) {
	sp.imgq.Enqueue(ctx, "hv:"+mbid, p, false)
}

func (sp *speed) runMB(ctx context.Context, key string, p int) error {
	if strings.HasPrefix(key, "disc:") {
		if sp.disc == nil {
			return fetchq.ErrLater
		}
		return sp.disc.Run(ctx, key, p)
	}
	if mbid, ok := strings.CutPrefix(key, "hv:"); ok {
		sp.wantVideos(ctx, mbid, p) // queued here by an older build
		return nil
	}
	return sp.handler.RunJob(ctx, key, p)
}

// runVideos runs an artist's music-video job ("hv:{mbid}"). Without the
// stored artist answer (name, home country, links) it asks for it at the
// job's own priority and waits; a paused or switched-off source makes it
// wait too, so nothing is stored as "no videos". Other failures (a 429,
// 5xx or timeout) back off and are tried again.
func (sp *speed) runVideos(ctx context.Context, mbid string, p int) error {
	err := sp.vids.Collect(ctx, mbid)
	switch {
	case errors.Is(err, herovideo.ErrNoArtist):
		if sp.handler == nil {
			return fetchq.ErrLater
		}
		ak := api.QueueKey("artist", mbid)
		if st, _ := sp.mbq.State(ctx, ak); st == "dead" {
			return fetchq.ErrNotFound
		}
		sp.mbq.Enqueue(ctx, ak, max(p, fetchq.P1Finish), false)
		return sp.mbq.WaitOn(ak)
	case errors.Is(err, herovideo.ErrPaused):
		return fetchq.ErrLater
	}
	return err
}

// videoPassEvery and videoPassMax pace the music-video background work:
// each pass queues at most videoPassMax artists (P3) and storyboard checks
// (P4).
var (
	videoPassEvery = time.Hour
	videoPassMax   = 2000
)

// videoPass queues the music-video work that is due: lists 30 days old,
// channel feeds a day old, listed videos still unchecked once their check
// can run, and storyboard checks that failed a day ago.
func (sp *speed) videoPass(ctx context.Context) {
	t := time.NewTimer(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		sp.queueVideoWork(ctx)
		t.Reset(videoPassEvery)
	}
}

func (sp *speed) queueVideoWork(ctx context.Context) {
	artists, deep := sp.vids.Due(ctx, videoPassMax)
	for _, m := range artists {
		sp.wantVideos(ctx, m, fetchq.P3Refresh)
	}
	for _, id := range deep {
		sp.imgq.Enqueue(ctx, "hvsb:"+id, fetchq.P4Deep, false)
	}
}

// warm is the popular-artist warmer's fetch: with the queue on it only
// queues a P2 job (the queue paces MusicBrainz).
func (sp *speed) warm(ctx context.Context, mbid string) (bool, error) {
	sp.enqueueShared(ctx, sharedfill.Member{Kind: "artist", MBID: mbid})
	return false, nil
}

func (sp *speed) status(ctx context.Context, window time.Duration) map[string]any {
	artists, withVideos := sp.vids.Count(ctx)
	return map[string]any{
		"resources":       sp.vm.Report(ctx, window),
		"queues":          []fetchq.Stats{sp.mbq.Stats(ctx), sp.imgq.Stats(ctx)},
		"images":          sp.imgs.Stats(ctx),
		"shared_lists":    sp.fill.Last(),
		"videos":          map[string]int64{"artists": artists, "with_videos": withVideos},
		"video_check":     sp.vids.Stats(ctx),
		"control":         sp.ctl.Get(),
		"image_sources":   sp.res.Limits.Status(),
		"catalog_covers":  sp.catalogStatus(),
		"upstream":        upstreamStatus(),
		"bigs_left":       sp.imgs.BigsLeft(ctx),
		"catalog_publish": sp.catalogPublish(),
	}
}

// catalogTop is how many leading catalog albums have their covers stored
// at P2 (the rest wait for idle time, P4): every album of today's bundle,
// top to bottom, since an install can show any of them. catalogPass caps
// the jobs one pass queues (a pass every catalogEvery).
var (
	catalogTop   = 30000
	catalogPass  = 3000
	catalogEvery = 20 * time.Minute
)

// storeCatalogCovers stores every catalog album's cover ahead of any ask,
// so a page shows each one from the store: a pass a minute after start,
// then every catalogEvery (a new bundle's albums join the next pass).
func (sp *speed) storeCatalogCovers(ctx context.Context) {
	t := time.NewTimer(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		sp.catalogCoversPass(ctx)
		t.Reset(catalogEvery)
	}
}

// catalogCoversPass queues the covers not stored yet (a job queued,
// running or dead within a day is left alone) and keeps the catalog names
// the cover fallback reads.
func (sp *speed) catalogCoversPass(ctx context.Context) {
	if sp.catalog == nil {
		return
	}
	albums := sp.catalog.Albums()
	names := make(map[string]musictags.AlbumRef, len(albums))
	stored, queued := 0, 0
	for i, a := range albums {
		if ctx.Err() != nil {
			return
		}
		names[a.MBID] = a
		if sp.imgs.Version(ctx, imgstore.KindAlbum, a.MBID) != "" {
			stored++
			continue
		}
		if queued >= catalogPass || !imgstore.ValidMBID(a.MBID) {
			continue
		}
		key := "img:" + imgstore.KindAlbum + ":" + a.MBID
		if st, at := sp.imgq.State(ctx, key); st == "queued" || st == "running" || st == "dead" && time.Since(at) < 24*time.Hour {
			continue
		}
		p := fetchq.P4Deep
		if i < catalogTop {
			p = fetchq.P2Shared
		}
		sp.imgq.Enqueue(ctx, key, p, false)
		queued++
	}
	// Every catalog artist's wide background and logo are stored ahead too
	// (their own cap per pass, so the album fill never holds them back).
	artists := sp.catalog.Artists()
	artStored, artQueued := 0, 0
	for _, mbid := range artists {
		if ctx.Err() != nil {
			return
		}
		wide, logo := sp.imgs.Version(ctx, imgstore.KindWide, mbid) != "", sp.imgs.Version(ctx, imgstore.KindLogo, mbid) != ""
		if wide && logo {
			artStored++
			continue
		}
		if artQueued >= catalogPass || !imgstore.ValidMBID(mbid) {
			continue
		}
		var keys []string
		if !wide {
			keys = append(keys, "img:"+imgstore.KindWide+":"+mbid)
		}
		if !logo {
			keys = append(keys, "img:"+imgstore.KindLogo+":"+mbid)
		}
		for _, key := range keys {
			if st, at := sp.imgq.State(ctx, key); st == "queued" || st == "running" || st == "dead" && time.Since(at) < 24*time.Hour {
				continue
			}
			// Artist heroes come first: their pictures go at P2 (an artist
			// page opened asks at P1 through the picture route).
			sp.imgq.Enqueue(ctx, key, fetchq.P2Shared, false)
			artQueued++
		}
	}
	sp.catMu.Lock()
	sp.catNames = names
	sp.catLast = map[string]any{"albums": len(albums), "stored": stored, "artists": len(artists), "artists_done": artStored, "queued": queued + artQueued, "at": time.Now().UTC().Format(time.RFC3339), "built": sp.catalog.Built().UTC().Format(time.RFC3339)}
	sp.catMu.Unlock()
}

// catalogStatus is the last pass's numbers for /v1/status.
func (sp *speed) catalogStatus() map[string]any {
	sp.catMu.Lock()
	defer sp.catMu.Unlock()
	out := map[string]any{}
	for k, v := range sp.catLast {
		out[k] = v
	}
	return out
}

// catalogAlbum answers a catalog album's names and year.
func (sp *speed) catalogAlbum(mbid string) (musictags.AlbumRef, bool) {
	sp.catMu.Lock()
	a, ok := sp.catNames[mbid]
	sp.catMu.Unlock()
	if !ok && sp.catalog != nil {
		for _, x := range sp.catalog.Albums() {
			if x.MBID == mbid {
				return x, true
			}
		}
	}
	return a, ok
}

// start attaches the queue to the handler and runs every worker.
func (sp *speed) start(ctx context.Context, handler *api.Server) {
	sp.handler = handler
	handler.SetQueue(sp.mbq)
	go sp.ctl.Run(ctx)
	go sp.vm.Run(ctx)
	go sp.imgs.Run(ctx)
	go sp.refreshPictures(ctx)
	go sp.mbq.Run(ctx)
	// Big sizes queued before they moved to the background lane follow.
	if n := sp.imgq.Lower(ctx, "big:", bigPriority, fetchq.P3Refresh); n > 0 {
		log.Printf("images: %d queued big sizes moved to the background lane", n)
	}
	go sp.imgq.Run(ctx)
	go sp.fill.Run(ctx)
	go sp.videoPass(ctx)
	if sp.catalog != nil {
		go sp.storeCatalogCovers(ctx)
	}
}

// Close closes every store that was opened.
func (sp *speed) Close() {
	if sp.mbq != nil {
		sp.mbq.Close()
	}
	if sp.imgq != nil {
		sp.imgq.Close()
	}
	if sp.vids != nil {
		sp.vids.Close()
	}
	if sp.imgs != nil {
		sp.imgs.Close()
	}
	if sp.vm != nil {
		sp.vm.Close()
	}
	if sp.disc != nil {
		sp.disc.Close()
	}
}

// deepFillNewest is how many of a listed artist's newest studio albums are
// filled at P2; the older ones go to P4 (deep fill, idle time only).
const deepFillNewest = 10

// deepFill splits a stored artist answer's studio albums (primary type
// Album, no secondary type) into the newest deepFillNewest and the older rest.
func deepFill(artist []byte) (newest, older []string) {
	var d struct {
		Source struct {
			Groups []struct {
				ID        string   `json:"id"`
				Primary   string   `json:"primary-type"`
				Secondary []string `json:"secondary-types"`
				Date      string   `json:"first-release-date"`
			} `json:"release-groups"`
		} `json:"source_data"`
	}
	if json.Unmarshal(artist, &d) != nil {
		return nil, nil
	}
	albums := d.Source.Groups[:0]
	for _, g := range d.Source.Groups {
		if g.Primary == "Album" && len(g.Secondary) == 0 && imgstore.ValidMBID(g.ID) {
			albums = append(albums, g)
		}
	}
	sort.SliceStable(albums, func(a, b int) bool { return albums[a].Date > albums[b].Date })
	for i, g := range albums {
		if i < deepFillNewest {
			newest = append(newest, strings.ToLower(g.ID))
		} else {
			older = append(older, strings.ToLower(g.ID))
		}
	}
	return newest, older
}

// queueAlbums fills a listed artist's albums with tracklists: the newest at
// P2, older ones at P4. Stored, unchanged albums cost no upstream call (the
// worker skips them).
func (sp *speed) queueAlbums(ctx context.Context, mbid string) {
	if sp.handler == nil || sp.fill == nil || !sp.fill.Listed("artist", mbid) {
		return
	}
	b, ok := sp.handler.StoredData(ctx, "artist", mbid)
	if !ok {
		return
	}
	newest, older := deepFill(b)
	for _, id := range newest {
		sp.queueAlbum(ctx, id, fetchq.P2Shared)
	}
	for _, id := range older {
		sp.queueAlbum(ctx, id, fetchq.P4Deep)
	}
}

// queueAlbum queues one album, or, when the loaded slim copy holds it (the
// job would end at once without a call), only does what the job's end does.
func (sp *speed) queueAlbum(ctx context.Context, id string, p int) {
	if sp.slimHas != nil && sp.slimHas(ctx, "release-group", id) {
		sp.wantMissingImage(ctx, "release-group", id)
		return
	}
	sp.mbq.Enqueue(ctx, api.QueueKey("release-group", id), p, false)
}

// upstreamStatus is the shared MetaBrainz pace and the connection health
// watch (hosts failing to connect now, and any failure in the last day).
func upstreamStatus() map[string]any {
	return map[string]any{"metabrainz_pace": upnet.Shared.Stats(), "mb_egress": upnet.Egresses.Stats(), "health": upnet.Watch.Report()}
}

// mbIdle reports the MusicBrainz queue with nothing to run at P0 to P2:
// the catalog job (musictags) makes its MusicBrainz calls only then.
func (sp *speed) mbIdle(ctx context.Context) bool {
	return sp.mbq == nil || sp.mbq.Ready(ctx, fetchq.P2Shared) == 0
}

// idleTransport holds a MetaBrainz call until idle reports true, checking
// every idlePoll; after idleMax of waiting the call goes anyway (a slow
// trickle, so the catalog still builds under a queue that is never empty).
type idleTransport struct {
	next http.RoundTripper
	idle func(context.Context) bool
	held *atomic.Int64
}

var (
	idlePoll = 2 * time.Second
	idleMax  = 10 * time.Minute
)

func (t idleTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.idle != nil && upnet.MetaBrainz(r.URL.Hostname()) {
		start := time.Now()
		for !t.idle(r.Context()) && time.Since(start) < idleMax {
			if t.held != nil {
				t.held.Add(1)
			}
			tm := time.NewTimer(idlePoll)
			select {
			case <-r.Context().Done():
				tm.Stop()
				return nil, r.Context().Err()
			case <-tm.C:
			}
		}
	}
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(r)
}

// picturePending reports an artist's photo or an album's cover that is not
// stored yet and not known missing (the answer is then marked partial).
func (sp *speed) picturePending(ctx context.Context, kind, mbid string) bool {
	k := imageKind(kind)
	if k != imgstore.KindArtist && k != imgstore.KindAlbum || sp.imgs == nil {
		return false
	}
	return sp.imgs.Version(ctx, k, mbid) == "" && !sp.imgs.Missed(ctx, k, mbid)
}

// catalogPublish is the catalog bundle's last build outcome (published, or
// refused for losing too many items).
func (sp *speed) catalogPublish() map[string]any {
	if c, ok := sp.catalog.(interface{ PublishStatus() map[string]any }); ok {
		return c.PublishStatus()
	}
	return nil
}
