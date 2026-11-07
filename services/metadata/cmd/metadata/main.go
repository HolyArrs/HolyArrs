package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/anime"
	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/budget"
	"github.com/CoderLuii/holyarrs-metadata/internal/control"
	"github.com/CoderLuii/holyarrs-metadata/internal/extras"
	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
	"github.com/CoderLuii/holyarrs-metadata/internal/nightly"
	"github.com/CoderLuii/holyarrs-metadata/internal/providers"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
	"github.com/CoderLuii/holyarrs-metadata/internal/servicetop"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"github.com/CoderLuii/holyarrs-metadata/internal/stats"
	"github.com/CoderLuii/holyarrs-metadata/internal/store"
	"github.com/CoderLuii/holyarrs-metadata/internal/trailers"
	upnet "github.com/CoderLuii/holyarrs-metadata/internal/upstream"
	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

type providerConfig struct {
	BaseURL    string `json:"base_url"`
	KeyFile    string `json:"key_file"`
	KeyEnv     string `json:"key_env"`
	IntervalMS int    `json:"interval_ms"`
	// Concurrency is calls in flight at once (0: the provider's default).
	Concurrency int `json:"concurrency"`
}
type config struct {
	Listen              string                    `json:"listen"`
	DataDir             string                    `json:"data_dir"`
	PrivateFixtures     bool                      `json:"private_fixtures"`
	Providers           map[string]providerConfig `json:"providers"`
	CacheBytes          int64                     `json:"cache_bytes"`
	CacheEntries        int                       `json:"cache_entries"`
	MinFreeBytes        uint64                    `json:"min_free_bytes"`
	TTLSeconds          int                       `json:"ttl_seconds"`
	StaleSeconds        int                       `json:"stale_seconds"`
	PerMinute           float64                   `json:"per_minute"`
	Burst               int                       `json:"burst"`
	PerTokenConcurrent  int                       `json:"per_token_concurrent"`
	MaxConcurrent       int                       `json:"max_concurrent"`
	MaxPending          int                       `json:"max_pending"`
	ProviderTTLSeconds  map[string]int            `json:"provider_ttl_seconds"`
	OperationTTLSeconds map[string]int            `json:"operation_ttl_seconds"`
	ClientPerMinute     float64                   `json:"client_per_minute"`
	ClientBurst         int                       `json:"client_burst"`
	// ClientAuthPerMinute and ClientAuthBurst limit an address's reads with
	// a working credential (default twice and four times the above).
	ClientAuthPerMinute float64  `json:"client_auth_per_minute"`
	ClientAuthBurst     int      `json:"client_auth_burst"`
	StatsTokenEnv       string   `json:"stats_token_env"`
	UserAgent           string   `json:"user_agent"`
	TrustedProxies      []string `json:"trusted_proxies"`
	// OpenRegistration lets each new install get its own credential with
	// POST /v1/installations, limited per address and in total.
	OpenRegistration      bool `json:"open_registration"`
	RegisterPerHour       int  `json:"register_per_hour"`
	RegisterGlobalPerHour int  `json:"register_global_per_hour"`
	RegisteredIdleDays    int  `json:"registered_idle_days"`
	RegisteredMax         int  `json:"registered_max"`
	// UpstreamPerHour caps calls to each provider (tmdb, musicbrainz,
	// listenbrainz, deezer) for the whole server; unset uses the defaults.
	UpstreamPerHour map[string]int `json:"upstream_per_hour"`
	// Refresh turns on change-driven refresh: answers are kept (no size
	// cap; a disk alarm at 75%), served from the store, and refreshed in
	// the background when the TMDB change lists, MusicBrainz replication
	// (with HOLYARRS_METABRAINZ_TOKEN) or Wikidata revisions show a change,
	// with a 14-day backstop. EvictAfterDays drops answers not requested
	// for that long (default 180; -1 keeps everything). MediaRecheckDays
	// re-marks popular TMDB titles for images and videos (0 = off).
	Refresh          bool `json:"refresh"`
	EvictAfterDays   int  `json:"evict_after_days"`
	MediaRecheckDays int  `json:"media_recheck_days"`
	// ServiceTop runs the per-service top lists (Streaming Availability,
	// with a TMDB fallback) and serves
	// GET /v1/lists/service-top.
	ServiceTop *serviceTopConfig `json:"service_top"`
	// MusicTrending runs the music chart job (the paid lists' charts first,
	// then the Apple Music API's; ListenBrainz and Deezer for the extras
	// and new releases) and serves GET /v1/lists/music-trending.
	MusicTrending bool `json:"music_trending"`
	// MBSlimRoot is the slim MusicBrainz dump copy's root (CURRENT and
	// the dated copies, read-only); HOLYARRS_MB_SLIM_ROOT overrides it.
	// Unset: MusicBrainz answers come from the live queue only.
	MBSlimRoot string `json:"mb_slim_root"`
	// Trailers serves GET /v1/trailers/{movie|tv}/{tmdb_id} and runs the
	// daily trailer health job (oEmbed re-checks).
	Trailers bool `json:"trailers"`
	// Anime downloads the Anime-Lists and Fribb ID lists daily and serves
	// GET /v1/anime/map.
	Anime bool `json:"anime"`
	// YTDLP is ignored: nothing runs an outside program for trailers any
	// more. It stays so a config that still names it loads (unknown keys
	// are refused).
	YTDLP string `json:"ytdlp"`
	// Wikidata serves /v1/wikidata/... (awards, people, cross-walk IDs,
	// Wikipedia summaries) and rebuilds the award-winner lists weekly.
	Wikidata bool `json:"wikidata"`
	// Speed turns on the music speed system (speed.go): fetch-ahead queue,
	// shared-list fill, picture store, hero videos, VM tracking, kill switch.
	Speed *speedConfig `json:"speed"`
}

type serviceTopConfig struct {
	// KeyEnv and BackupKeyEnv name the environment variables holding the
	// primary and backup Streaming Availability keys; either may be unset.
	KeyEnv       string   `json:"key_env"`
	BackupKeyEnv string   `json:"backup_key_env"`
	Regions      []string `json:"regions"`
	// ActiveEnv names the variable choosing the keys: "backup" (default)
	// uses only the backup key; "primary" uses the primary, then the
	// backup on rejection or quota.
	ActiveEnv string `json:"active_env"`
}

// serviceTopKeys picks the Streaming Availability keys in the order tried.
func serviceTopKeys(st *serviceTopConfig, env func(string) string) ([]string, error) {
	read := func(name string) string {
		if name == "" {
			return ""
		}
		return strings.TrimSpace(env(name))
	}
	active := "backup"
	if st.ActiveEnv != "" && read(st.ActiveEnv) != "" {
		active = read(st.ActiveEnv)
	}
	switch active {
	case "backup":
		return []string{read(st.BackupKeyEnv)}, nil
	case "primary":
		return []string{read(st.KeyEnv), read(st.BackupKeyEnv)}, nil
	}
	return nil, errors.New("service_top active key must be primary or backup")
}

func readConfig(path string) (config, error) {
	var c config
	f, e := os.Open(path)
	if e != nil {
		return c, errors.New("cannot open configuration")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, errors.New("invalid configuration")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("trailing configuration")
	}
	if c.DataDir == "" || len(c.Providers) == 0 || c.CacheBytes < 0 || c.CacheBytes > 1<<40 || c.CacheEntries < 0 || c.CacheEntries > 10000000 || c.TTLSeconds < 0 || c.TTLSeconds > 2592000 || c.StaleSeconds < 0 || c.StaleSeconds > 604800 || c.PerMinute < 0 || c.PerMinute > 1000000 || c.Burst < 0 || c.Burst > 100000 || c.MaxConcurrent < 0 || c.MaxConcurrent > 1000 || c.PerTokenConcurrent < 0 || c.PerTokenConcurrent > 1000 || c.MaxPending < 0 || c.MaxPending > 10000 || c.ClientPerMinute < 0 || c.ClientPerMinute > 1000000 || c.ClientBurst < 0 || c.ClientBurst > 100000 || c.ClientAuthPerMinute < 0 || c.ClientAuthPerMinute > 1000000 || c.ClientAuthBurst < 0 || c.ClientAuthBurst > 100000 || c.RegisterPerHour < 0 || c.RegisterPerHour > 1000 || c.RegisterGlobalPerHour < 0 || c.RegisterGlobalPerHour > 100000 || c.RegisteredIdleDays < 0 || c.RegisteredIdleDays > 3650 || c.RegisteredMax < 0 || c.RegisteredMax > 10000000 || len(c.UserAgent) > 256 || strings.ContainsAny(c.UserAgent, "\r\n") {
		return c, errors.New("invalid configuration bounds")
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if len(c.TrustedProxies) > 64 {
		return c, errors.New("too many trusted proxies")
	}
	for _, cidr := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return c, errors.New("invalid trusted proxy network")
		}
	}
	for name, p := range c.Providers {
		if p.KeyEnv != "" && (name != "tmdb" || p.KeyFile != "") {
			return c, errors.New("provider credential unsupported")
		}
		if p.IntervalMS < 0 || p.IntervalMS > 60000 {
			return c, errors.New("invalid provider interval")
		}
		if c.PrivateFixtures {
			expected := map[string]string{"tmdb": "http://fixture:8081/tmdb/3", "tvmaze": "http://fixture:8081/tvmaze", "musicbrainz": "http://fixture:8081/musicbrainz/ws/2"}
			if expected[name] == "" || p.BaseURL != expected[name] || p.KeyFile != "" || p.KeyEnv != "" {
				return c, errors.New("fixture mode requires dedicated synthetic endpoints without provider credentials")
			}
		} else {
			minimum := map[string]int{"tmdb": 50, "tvmaze": 1000, "musicbrainz": 1100}[name]
			if minimum == 0 || (p.IntervalMS != 0 && p.IntervalMS < minimum) {
				return c, errors.New("provider pacing below policy")
			}
			if p.IntervalMS == 0 {
				p.IntervalMS = minimum
				c.Providers[name] = p
			}
		}
	}
	for _, policies := range []map[string]int{c.ProviderTTLSeconds, c.OperationTTLSeconds} {
		for _, seconds := range policies {
			if seconds <= 0 || seconds > 2592000 {
				return c, errors.New("invalid expiry policy")
			}
		}
	}
	for name := range c.ProviderTTLSeconds {
		if _, ok := c.Providers[name]; !ok {
			return c, errors.New("expiry provider is not configured")
		}
	}
	for key := range c.OperationTTLSeconds {
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			return c, errors.New("invalid expiry operation")
		}
		if _, ok := c.Providers[parts[0]]; !ok {
			return c, errors.New("expiry provider is not configured")
		}
		if parts[1] != "search" && parts[1] != "detail" && !(parts[0] == "tmdb" && (parts[1] == "season" || parts[1] == "tmdb")) {
			return c, errors.New("unsupported expiry operation")
		}
	}
	return c, nil
}

func main() {
	if e := run(os.Args[1:]); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("expected serve, healthcheck, token-issue, token-revoke, backup or check")
	}
	if args[0] == "edge-rule" {
		return edgeRule(args[1:])
	}
	if args[0] == "purge-edge" {
		return purgeEdge(args[1:])
	}
	if args[0] == "purge-provider" {
		return purgeProvider(args[1:])
	}
	if args[0] == "refill-from-links" {
		return refillFromLinks(args[1:])
	}
	if args[0] == "backup-permanent" {
		return backupPermanent(args[1:])
	}
	if args[0] == "healthcheck" {
		hc := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
		hc.SetOutput(io.Discard)
		target := hc.String("url", "http://127.0.0.1:8080/readyz", "readiness address")
		if hc.Parse(args[1:]) != nil || hc.NArg() != 0 {
			return errors.New("invalid command arguments")
		}
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(*target)
		if err != nil {
			return errors.New("service unreachable")
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return errors.New("service not ready")
		}
		return nil
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configFile := fs.String("config", "", "configuration file")
	dataDir := fs.String("data-dir", "", "private data directory")
	tokenFile := fs.String("token-file", "", "credential file")
	destination := fs.String("destination", "", "new backup directory")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return errors.New("invalid command arguments")
	}
	if args[0] == "serve" {
		log.SetOutput(addressFree{os.Stderr})
		c, e := readConfig(*configFile)
		if e != nil {
			return e
		}
		return serve(c)
	}
	if args[0] != "token-issue" && args[0] != "token-register" && args[0] != "token-revoke" && args[0] != "backup" && args[0] != "check" {
		return errors.New("unknown command")
	}
	if *dataDir == "" {
		return errors.New("data directory required")
	}
	storageOptions := store.Options{}
	if *configFile != "" {
		c, err := readConfig(*configFile)
		if err != nil {
			return err
		}
		storageOptions = store.Options{MaxBytes: c.CacheBytes, MaxEntries: c.CacheEntries, MinFreeBytes: c.MinFreeBytes}
	}
	s, e := store.Open(*dataDir, storageOptions)
	if e != nil {
		return e
	}
	defer s.Close()
	budget := time.Minute
	if args[0] == "backup" {
		budget = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	switch args[0] {
	case "token-issue":
		if *tokenFile == "" {
			return errors.New("credential file required")
		}
		b := make([]byte, 32)
		if _, e = rand.Read(b); e != nil {
			return errors.New("credential generation failed")
		}
		token := base64.RawURLEncoding.EncodeToString(b)
		f, e := os.OpenFile(*tokenFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return errors.New("cannot create credential file")
		}
		if _, e = f.WriteString(token + "\n"); e != nil {
			f.Close()
			os.Remove(*tokenFile)
			return errors.New("cannot write credential file")
		}
		if e = f.Close(); e != nil {
			os.Remove(*tokenFile)
			return errors.New("cannot close credential file")
		}
		if e = s.IssueToken(ctx, token); e != nil {
			os.Remove(*tokenFile)
			return e
		}
		fmt.Println("credential issued to private file")
	case "token-revoke", "token-register":
		b, e := os.ReadFile(*tokenFile)
		if e != nil || len(b) > 1024 {
			return errors.New("cannot read credential file")
		}
		if args[0] == "token-register" {
			if e = s.IssueToken(ctx, strings.TrimSpace(string(b))); e != nil {
				return e
			}
			fmt.Println("credential registered from private file")
			return nil
		}
		if e = s.RevokeToken(ctx, strings.TrimSpace(string(b))); e != nil {
			return e
		}
		fmt.Println("credential revoked")
	case "backup":
		if *destination == "" {
			return errors.New("backup destination required")
		}
		if e = s.Backup(ctx, *destination); e != nil {
			return e
		}
		fmt.Println("consistent database snapshot complete")
	case "check":
		if e = s.Check(ctx); e != nil {
			return e
		}
		fmt.Println("database integrity verified")
	}
	return nil
}

func directoryBytes(dir string) int64 {
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
	}
	return total
}

func serve(c config) error {
	// The log's ALARM lines are kept for HolyDash's label (alarms.go).
	alarms := openAlarmLog(c.DataDir)
	log.SetOutput(alarms.tap(addressFree{os.Stderr}))
	// Listen at once and hold requests until everything is open, so a
	// deploy's new process takes over without refusing any (handoff.go).
	ln, e := net.Listen("tcp", c.Listen)
	if e != nil {
		return errors.New("metadata listener failed")
	}
	gate := newStartGate()
	server := &http.Server{Handler: gate, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
		ErrorLog: log.New(addressFree{os.Stderr}, "", log.LstdFlags)}
	served := make(chan error, 1)
	go func() { served <- server.Serve(ln) }()
	storeOptions := store.Options{MaxBytes: c.CacheBytes, MaxEntries: c.CacheEntries, MinFreeBytes: c.MinFreeBytes}
	if c.Refresh {
		// No hard cap: keep everything; the disk alarm warns instead.
		storeOptions.MaxBytes, storeOptions.MaxEntries = 1<<40, 10000000
	}
	s, e := openStoreWaiting(c.DataDir, storeOptions, ownerWait)
	if e != nil {
		return e
	}
	defer s.Close()
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCheck()
	if e = s.Check(checkCtx); e != nil {
		return e
	}
	configs := make(map[string]providers.Config, len(c.Providers))
	for name, p := range c.Providers {
		cfg := providers.Config{BaseURL: p.BaseURL, KeyFile: p.KeyFile, Interval: time.Duration(p.IntervalMS) * time.Millisecond, Concurrency: p.Concurrency}
		if p.KeyEnv != "" {
			cfg.Key = os.Getenv(p.KeyEnv)
			if cfg.Key == "" {
				return errors.New("provider credential environment is empty")
			}
		}
		configs[name] = cfg
	}
	// MusicBrainz calls spread over several outgoing addresses
	// (HOLYARRS_MB_EGRESS): every MusicBrainz pace and budget scales by
	// their count, each address still at one call a second.
	if upnet.Egresses, e = upnet.ParseEgress(os.Getenv(upnet.EgressEnv)); e != nil {
		return e
	}
	if n := upnet.MBScale(); n > 1 {
		mb := configs["musicbrainz"]
		if mb.Interval == 0 {
			mb.Interval = 1100 * time.Millisecond
		}
		mb.Interval /= time.Duration(n)
		mb.Concurrency = min(n, 8)
		configs["musicbrainz"] = mb
		budget.DefaultPerHour["musicbrainz"] *= n
		control.MBScale = float64(n)
		log.Printf("mb egress: %d outgoing paths, MusicBrainz at %d calls a second", n, n)
	}
	userAgent := c.UserAgent
	if userAgent == "" {
		userAgent = "HolyArrsMetadata/0.2 (+https://github.com/CoderLuii/HolyArrs)"
	}
	direct, e := providers.New(providers.Options{Providers: configs, AllowPrivate: c.PrivateFixtures, MaxPending: c.MaxPending, Timeout: 10 * time.Second, Attempts: 2, UserAgent: userAgent})
	if e != nil {
		return e
	}
	upstream := budget.New(c.UpstreamPerHour)
	client := budget.Fetcher{Fetcher: direct, Budget: upstream}
	providerTTLs := map[string]time.Duration{}
	operationTTLs := map[string]time.Duration{}
	enabled := map[string]bool{}
	for key, value := range c.ProviderTTLSeconds {
		providerTTLs[key] = time.Duration(value) * time.Second
	}
	for key, value := range c.OperationTTLSeconds {
		operationTTLs[key] = time.Duration(value) * time.Second
	}
	for name := range c.Providers {
		enabled[name] = true
	}
	recorder, e := stats.Open(filepath.Join(c.DataDir, "stats.sqlite"), stats.Options{ItemTTL: time.Duration(c.TTLSeconds+c.StaleSeconds) * time.Second, MaxItems: c.CacheEntries, Size: func() int64 { return directoryBytes(c.DataDir) }})
	if e != nil {
		return e
	}
	defer recorder.Close()
	recorder.Run(30 * time.Second)
	statsToken := ""
	if c.StatsTokenEnv != "" {
		statsToken = strings.TrimSpace(os.Getenv(c.StatsTokenEnv))
		if len(statsToken) < 24 {
			return errors.New("statistics credential missing or too short")
		}
	}
	var proxies []*net.IPNet
	for _, cidr := range c.TrustedProxies {
		_, network, _ := net.ParseCIDR(cidr)
		proxies = append(proxies, network)
	}
	options := api.Options{TrustedProxies: proxies, Stats: recorder, StatsToken: statsToken, ClientPerMinute: c.ClientPerMinute, ClientBurst: c.ClientBurst, ClientAuthPerMinute: c.ClientAuthPerMinute, ClientAuthBurst: c.ClientAuthBurst, ProviderTTLs: providerTTLs, OperationTTLs: operationTTLs, EnabledProviders: enabled, TTL: time.Duration(c.TTLSeconds) * time.Second, StaleTTL: time.Duration(c.StaleSeconds) * time.Second, PerMinute: c.PerMinute, Burst: c.Burst, PerTokenConcurrent: c.PerTokenConcurrent, MaxConcurrent: c.MaxConcurrent, Registration: c.OpenRegistration, RegisterPerHour: c.RegisterPerHour, RegisterGlobalPerHour: c.RegisterGlobalPerHour, RegisteredIdle: time.Duration(c.RegisteredIdleDays) * 24 * time.Hour, RegisteredMax: c.RegisteredMax}
	// Outside sources are dialed on public addresses only.
	upnet.BlockPrivate.Store(true)
	var similarStore *musictrend.Similar
	slim, e := openSlim(c, os.Getenv, &options)
	if e != nil {
		return e
	}
	if slim != nil {
		defer slim.Close()
	}
	var tracker *refresh.Tracker
	var mbFeed *refresh.MBFeed
	if c.Refresh {
		tracker, e = refresh.Open(filepath.Join(c.DataDir, "refresh.sqlite"))
		if e != nil {
			return e
		}
		defer tracker.Close()
		mbFeed = &refresh.MBFeed{Tracker: tracker, Token: os.Getenv("HOLYARRS_METABRAINZ_TOKEN"), HTTP: &http.Client{Timeout: 5 * time.Minute, Transport: upnet.Transport{}}, UserAgent: userAgent, Now: time.Now}
		options.Refresh, options.MusicFeed = tracker, mbFeed.Enabled()
	}
	var serviceTop *servicetop.Service
	if st := c.ServiceTop; st != nil {
		keys, e := serviceTopKeys(st, os.Getenv)
		if e != nil {
			return e
		}
		serviceTop, e = servicetop.Open(filepath.Join(c.DataDir, "servicetop.sqlite"), servicetop.Options{Keys: keys, Regions: st.Regions, New: true, TMDB: servicetop.TMDB{Fetcher: client},
			Report: func(code string, n int) {
				for range n {
					recorder.ProviderError("streaming-availability", code)
				}
			}})
		if e != nil {
			return e
		}
		defer serviceTop.Close()
		options.ServiceTop = serviceTop
	}
	var music *musictrend.Service
	var newMusic *musictrend.NewMusic
	if c.MusicTrending {
		music, e = musictrend.Open(filepath.Join(c.DataDir, "music.sqlite"), musictrend.Options{Extras: true, HTTP: budget.Client(upstream, time.Minute), ListenBrainzURL: musictrend.ListenBrainzURL, UserAgent: userAgent, Search: musictrend.MBSearch{Fetcher: client},
			Report: func(source, code string, n int) {
				for range n {
					recorder.ProviderError(source, code)
				}
			}})
		if e != nil {
			return e
		}
		defer music.Close()
		options.Music = music
		similar := &musictrend.Similar{UserAgent: userAgent, HTTP: budget.Client(upstream, 15*time.Second), Report: func(source, code string) { recorder.ProviderError(source, code) }}
		// Similar artists and artist data are kept across restarts.
		if e = similar.OpenStore(filepath.Join(c.DataDir, "similar.sqlite")); e != nil {
			return e
		}
		defer similar.CloseStore()
		options.Similar = similar
		similarStore = similar
		newMusic, e = musictrend.OpenNew(filepath.Join(c.DataDir, "newmusic.sqlite"), musictrend.NewOptions{FreshURL: musictrend.FreshReleasesURL, UserAgent: userAgent, HTTP: budget.Client(upstream, time.Minute),
			Report: func(source, code string, n int) {
				for range n {
					recorder.ProviderError(source, code)
				}
			}})
		if e != nil {
			return e
		}
		defer newMusic.Close()
		options.NewMusic = newMusic
	}
	var tr *trailers.Service
	if c.Trailers {
		tr, e = trailers.Open(filepath.Join(c.DataDir, "trailers.sqlite"), trailers.Options{Fetcher: client})
		if e != nil {
			return e
		}
		defer tr.Close()
		options.Trailers = tr
	}
	// fanart.tv and MDBList stay off unless their key is set. Key values
	// are never logged.
	if key := strings.TrimSpace(os.Getenv("HOLYARRS_FANART_KEY")); key != "" {
		options.Fanart = &extras.Fanart{Key: key, ClientKey: strings.TrimSpace(os.Getenv("FANARTTV_PERSONAL_KEY")), UserAgent: userAgent, HTTP: budget.Client(upstream, 15*time.Second), TVDB: extras.TMDBToTVDB(client)}
	}
	var ratings *extras.MDBList
	if key := strings.TrimSpace(os.Getenv("HOLYARRS_MDBLIST_KEY")); key != "" {
		ratings = &extras.MDBList{Key: key, UserAgent: userAgent, HTTP: budget.Client(upstream, 15*time.Second)}
		if e := ratings.Open(filepath.Join(c.DataDir, "ratings.sqlite")); e != nil {
			return e
		}
		defer ratings.Close()
		options.Ratings = ratings
	}
	// Genre tile art: fanart.tv when its key is set, else Deezer (no key).
	if music != nil {
		options.GenreTiles = &api.GenreTiles{DeezerURL: musictrend.DeezerURL, UserAgent: userAgent, HTTP: publicClient(upstream, 15*time.Second)}
		if options.Fanart != nil {
			options.GenreTiles.Fanart = options.Fanart
		}
		tileStore, e := api.OpenTileStore(filepath.Join(c.DataDir, "genretiles.sqlite"))
		if e != nil {
			return e
		}
		defer tileStore.Close()
		options.GenreTiles.Store = tileStore
		options.GenreTiles.Restore()
	}
	var am *anime.Service
	if c.Anime {
		am, e = anime.Open(filepath.Join(c.DataDir, "anime.sqlite"), anime.Options{HTTP: budget.Client(upstream, time.Minute), UserAgent: userAgent})
		if e != nil {
			return e
		}
		defer am.Close()
		options.Anime = am
	}
	var wd *wikidata.Service
	if c.Wikidata {
		wd, e = wikidata.Open(filepath.Join(c.DataDir, "wikidata.sqlite"), wikidata.Options{HTTP: budget.Client(upstream, 20*time.Second), UserAgent: userAgent})
		if e != nil {
			return e
		}
		defer wd.Close()
		options.Wikidata = wd
	}
	var sp *speed
	if c.Speed != nil {
		if sp, e = openSpeed(c, &options, upstream, userAgent, music, newMusic); e != nil {
			return e
		}
		if slim != nil {
			sp.brief = slim.ArtistBrief
			sp.slimHas = slim.Has
		}
		defer sp.Close()
	}
	var dumpRows func(context.Context) (string, map[string]int64)
	if slim != nil {
		dumpRows = slim.DumpRows
	}
	openMusicSources(sp, c.DataDir, dumpRows, &options)
	var slimDate func() string
	if slim != nil {
		slimDate = slim.Date
	}
	var musicUsed func() int64
	if sp != nil {
		musicUsed = sp.musicUsed.Load
	}
	openAlarms(alarms, alarmChecks{dataDir: c.DataDir, slimDate: slimDate, music: musicUsed, now: time.Now}, &options)
	openDiskShare(c.DataDir, []string{c.DataDir, slimRoot(c, os.Getenv)}, &options)
	if _, e = openDiscog(c, sp, direct, upstream, slim, &options); e != nil {
		return e
	}
	pk, e := openPackages(c, sp, &options)
	if e != nil {
		return e
	}
	if pk != nil {
		defer pk.Close()
	}
	if sp != nil && sp.disc != nil && mbFeed != nil {
		mbFeed.OnChanged = func(ctx context.Context, ids []string) { sp.disc.MarkChanged(ctx, ids) }
	}
	md, e := openMusicData(c, sp, music, &options)
	if e != nil {
		return e
	}
	if md != nil {
		defer md.Close()
	}
	kw, e := openKworb(c, sp, direct, upstream, userAgent, &options)
	if e != nil {
		return e
	}
	if kw != nil {
		defer kw.Close()
	}
	if pk != nil {
		pk.kw = kw
	}
	ls, e := openLists(c, os.Getenv, musictrend.MBSearch{Fetcher: client}, listsLink(direct, upstream, userAgent), kw, sp, &options)
	if e != nil {
		return e
	}
	if ls != nil {
		defer ls.Close()
		attachLists(ls, sp)
		if newMusic != nil {
			ls.SetFresh(freshFrom(newMusic))
		}
		if music != nil {
			music.SetLists(ls) // charts, trending lists and genre pages: the lists first
		}
	}
	if music != nil && sp != nil && sp.res.AppleMusic != nil {
		music.SetApple(sp.res.AppleJSON) // the Apple Music API charts
	}
	chartPrefetch := openChartPrefetch(music, newMusic, ls, slim, sp, &options, c.DataDir)
	var mbIdle func(context.Context) bool
	if sp != nil {
		mbIdle = sp.mbIdle
	}
	mt, e := openMusicTags(c, music, ls, direct, upstream, userAgent, &options, mbIdle)
	if e != nil {
		return e
	}
	if mt != nil {
		defer mt.Close()
	}
	if sp != nil && mt != nil {
		sp.catalog = mt // the catalog's album covers are stored ahead
	}
	handler := api.New(s, client, options)
	if sp != nil {
		sp.slimAbsent = handler.SlimAbsent // shares the API's short negative cache
	}
	if sim, ok := options.Similar.(*musictrend.Similar); ok {
		// A "none" is kept on disk only for artists MusicBrainz knows.
		sim.SetKnown(func(mbid string) bool { _, ok := handler.StoredData(context.Background(), "artist", mbid); return ok })
	}
	if wd != nil {
		// Artists go by their stored MusicBrainz answer first (its Wikidata
		// link, and the name, country and year a name search is checked on).
		wd.SetArtistMB(func(ctx context.Context, mbid string) (wikidata.MBArtist, bool) {
			b, ok := handler.StoredData(ctx, "artist", mbid)
			if !ok {
				return wikidata.MBArtist{}, false
			}
			return wikidata.ParseMBArtist(b)
		})
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if sigs := profileSignals(); len(sigs) > 0 && os.Getenv(ProfileEnv) == "1" {
		// SIGUSR1 writes a 30 s CPU profile to the temp folder (off
		// unless HOLYARRS_CPU_PROFILE=1).
		ask := make(chan os.Signal, 1)
		signal.Notify(ask, sigs...)
		go cpuProfileOn(ctx, ask, os.TempDir(), 30*time.Second)
	}
	if sp != nil {
		if slim != nil && sp.imgs != nil {
			// An artist photo that is one of the artist's covers is refused.
			sp.imgs.SetAlbumsOf(slimGroupIDs(slim))
		}
		if slim != nil && sp.imgs != nil && sp.res != nil {
			// Exact ids come after MusicBrainz's link and before a name:
			// release barcodes, then the linked Wikidata item's ids (kept
			// answers; never asked on a page open).
			sp.res.Barcodes, sp.res.Via = slim.ArtistBarcodes, sp.imgs.SetVia
			if wd != nil {
				sp.res.WikidataIDs = func(ctx context.Context, qid string) (string, string, string, error) {
					m, err := wd.OutsideIDs(ctx, []string{qid})
					ids := m[qid]
					return ids.AppleMusic, ids.Deezer, ids.Spotify, err
				}
			}
		}
		sp.start(ctx, handler)
		if slim != nil && sp.imgs != nil {
			go purgeBadArtistPhotos(ctx, sp.imgs, sp.imgq, c.DataDir, time.Now)
		}
		if slim != nil && sp.imgs != nil && sp.res != nil {
			pass := &homonymRestore{imgs: sp.imgs, rivals: slim.HomonymIDs, titles: slim.GroupTitles, evidence: sp.res.AppleEvidenceOf,
				// live-20261029s (the old rule) started 20:49 UTC, live-20261029u
				// (the evidence rule) at 21:32.
				staleFrom: time.Date(2026, 10, 29, 20, 49, 0, 0, time.UTC), staleTo: time.Date(2026, 10, 29, 21, 32, 15, 0, time.UTC),
				name: func(ctx context.Context, id string) string { b, _ := slim.ArtistBrief(ctx, id); return b.Name },
				dir:  filepath.Join(c.DataDir, "backups"), now: time.Now, logf: log.Printf,
				refetch: func(ctx context.Context, mbid string) {
					for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
						sp.imgs.Expire(ctx, kind, mbid)
						sp.imgq.Enqueue(ctx, "img:"+kind+":"+mbid, fetchq.P3Refresh, false)
					}
					if similarStore != nil {
						similarStore.Expire(mbid)
					}
					if md != nil {
						md.Expire(ctx, "top:"+mbid)
					}
				},
				undo: func(ctx context.Context, mbid string) {
					sp.imgs.KeepID(imgstore.KindArtist, mbid, srctag.Tag{Source: "applemusic"}, "", "")
					for _, kind := range []string{imgstore.KindArtist, imgstore.KindWide} {
						sp.imgs.DropFrom(ctx, kind, mbid, "applemusic")
						sp.imgs.Expire(ctx, kind, mbid)
						sp.imgq.Enqueue(ctx, "img:"+kind+":"+mbid, fetchq.P3Refresh, false)
					}
					if similarStore != nil {
						similarStore.DropSource(mbid, "applemusic")
					}
					if md != nil {
						md.Expire(ctx, "top:"+mbid)
					}
				}}
			shared := &sharedPass{imgs: sp.imgs, brief: slim.ArtistBrief, rivals: slim.HomonymIDs, titles: slim.GroupTitles,
				evidence: sp.res.EvidenceOf, refetch: pass.refetch, dir: pass.dir, now: time.Now, logf: log.Printf}
			if similarStore != nil {
				shared.similar = similarStore.DropSource
			}
			go func() {
				if _, _, err := pass.run(ctx); err != nil {
					log.Printf("apple homonyms re-check: %v", err)
				}
				single := &singleTitlePass{imgs: sp.imgs, rivals: slim.HomonymIDs, evidence: sp.res.EvidenceOf,
					similar: shared.similar, refetch: pass.refetch, dir: pass.dir, now: time.Now, logf: log.Printf}
				if _, _, err := single.run(ctx); err != nil {
					log.Printf("single-title re-check: %v", err)
				}
				exact := &exactIDsPass{imgs: sp.imgs, brief: slim.ArtistBrief, rivals: slim.HomonymIDs, exact: sp.res.ExactID,
					evidence: sp.res.EvidenceOf, similar: shared.similar, refetch: pass.refetch, dir: pass.dir, now: time.Now, logf: log.Printf}
				if wd != nil {
					exact.prefetch = func(ctx context.Context, qids []string) {
						if _, err := wd.OutsideIDs(ctx, qids); err != nil {
							log.Printf("exact ids: wikidata: %v", err)
						}
					}
				}
				go func() {
					if _, err := exact.run(ctx); err != nil {
						log.Printf("exact ids: %v", err)
					}
				}()
				// Groups a paused source could not answer are asked again
				// every 30 minutes until all are done.
				for {
					if _, err := shared.run(ctx); err != nil {
						log.Printf("shared ids: %v", err)
					}
					if shared.done() {
						break
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(30 * time.Minute):
					}
				}
			}()
		}
		go upnet.Egresses.LogEvery(ctx, time.Minute)
		if sp.disc != nil {
			go sp.discogPrefetch(ctx, discogSources{kw: kw, tracker: tracker})
		}
		if pk != nil {
			go pk.run(ctx)
		}
	}
	if serviceTop != nil {
		go serviceTop.Run(ctx)
	}
	if mt != nil {
		go mt.Run(ctx)
	}
	if music != nil {
		go music.Run(ctx)
		// Keep the most popular artists' pages warm (charts plus ListenBrainz tops).
		warm := &musictrend.Warmer{ListenBrainzURL: musictrend.ListenBrainzURL, UserAgent: userAgent, HTTP: budget.Client(upstream, time.Minute), Charts: music.ChartArtists, Warm: handler.WarmMusicArtist}
		if sp != nil {
			warm.Warm = sp.warm // through the queue
		}
		go warm.Run(ctx)
	}
	if newMusic != nil {
		go newMusic.Run(ctx)
	}
	if chartPrefetch != nil {
		chartPrefetch.Start = 2 * time.Minute
		go chartPrefetch.Run(ctx)
	}
	if kw != nil {
		go kw.Run(ctx)
	}
	if ls != nil {
		go ls.Run(ctx)
	}
	if tr != nil {
		go tr.Run(ctx)
	}
	if wd != nil {
		go wd.Run(ctx)
	}
	if am != nil {
		go am.Run(ctx)
	}
	if ratings != nil || wd != nil {
		// Each night: MDBList ratings (within its budget) and Wikidata
		// facts for trending, service-top and newly released titles.
		o := nightly.Options{Left: func() float64 { return upstream.Left("mdblist") },
			Titles: func(ctx context.Context) []nightly.Title {
				list := nightly.TMDBLists(ctx, client)
				if serviceTop != nil && c.ServiceTop != nil {
					var bodies [][]byte
					for _, region := range c.ServiceTop.Regions {
						if b, _, ok := serviceTop.List(region); ok {
							bodies = append(bodies, b)
						}
					}
					list = append(list, nightly.ServiceTop(bodies...)...)
				}
				return list
			}}
		if ratings != nil {
			o.Ratings = ratings
		}
		if wd != nil {
			o.Facts = wd
		}
		go nightly.New(o).Run(ctx)
	}
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			server.Close()
		}
		shutdownDone <- err
	}()
	if tracker != nil {
		runner := &refresh.Runner{Tracker: tracker, TMDB: &refresh.TMDBPoller{Tracker: tracker, Fetcher: client, Now: time.Now}, MB: mbFeed, Refetch: handler.Revalidate, Evict: s.Delete,
			EvictAfter: time.Duration(c.EvictAfterDays) * 24 * time.Hour, MediaRecheck: time.Duration(c.MediaRecheckDays) * 24 * time.Hour, DiskPath: c.DataDir, StoreBytes: func() int64 { return directoryBytes(c.DataDir) }}
		if c.EvictAfterDays < 0 {
			runner.EvictAfter = -1
		}
		if wd != nil {
			runner.Wikidata = wd
		}
		handler.SetRefreshSnapshot(runner.Snapshot)
		go runner.Run(ctx)
		log.Print("change-driven refresh on; musicbrainz feed=", mbFeed.Enabled())
	}
	log.Print("metadata service starting; private fixtures=", c.PrivateFixtures)
	gate.open(handler)
	e = <-served
	if errors.Is(e, http.ErrServerClosed) {
		if <-shutdownDone != nil {
			return errors.New("metadata shutdown deadline exceeded")
		}
		return nil
	}
	return errors.New("metadata listener failed")
}
