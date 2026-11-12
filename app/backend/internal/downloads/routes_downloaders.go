package downloads

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func (a *Web) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/downloaders", a.listDownloaders)
	m.HandleFunc("POST /api/v1/downloaders", a.addDownloader)
	m.HandleFunc("PUT /api/v1/downloaders/{id}", a.editDownloader)
	m.HandleFunc("DELETE /api/v1/downloaders/{id}", a.deleteDownloader)
	m.HandleFunc("POST /api/v1/downloaders/{id}/test", a.TestDownloader)
	m.HandleFunc("POST /api/v1/downloaders/{id}/pause", a.pauseDownloader)
	m.HandleFunc("POST /api/v1/downloaders/{id}/resume", a.resumeDownloader)
	m.HandleFunc("GET /api/v1/downloaders/{id}/attempts", a.listDownloaderAttempts)
	m.HandleFunc("POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon", a.abandonDownloaderAttempt)
	m.HandleFunc("POST /api/v1/downloads/{id}/mark-failed", a.markFailed)
	m.HandleFunc("GET /api/v1/settings/downloaders", a.getDownloaderSettings)
	m.HandleFunc("PUT /api/v1/settings/downloaders", a.putDownloaderSettings)
	a.downloadsRoutes(m)
}

// DownloaderView is a Downloader as the API shows it: the key or password
// is masked, never sent.
type DownloaderView struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Secret   string `json:"secret"`
	Category string `json:"category"`
	// Category for one kind of title; empty uses Category.
	CategoryMovies string           `json:"category_movies"`
	CategoryShows  string           `json:"category_shows"`
	CategoryMusic  string           `json:"category_music"`
	Priority       int              `json:"priority"`
	PathMap        []domain.PathMap `json:"path_map"`
	AllowLocal     bool             `json:"allow_local_network"`
	// Writable is "HolyArrs can change this Downloader"; WritableLocked
	// says HOLYARRS_READONLY_DOWNLOADERS keeps it read only whatever the switch.
	Writable       bool `json:"writable"`
	WritableLocked bool `json:"writable_locked"`
	// ClearFinished: a Usenet job and its files leave the client once imported.
	ClearFinished bool `json:"clear_finished"`
	// AfterSeeding: a torrent leaves the client once imported and seeded.
	AfterSeeding SeedingChoice `json:"after_seeding"`
	Tags         []string      `json:"tags"`
	Health       HealthView    `json:"health"`
	CheckedAt    *time.Time    `json:"checked_at,omitempty"`
	Active       int           `json:"active_downloads"`
}

func downloaderView(d domain.Downloader, active int) DownloaderView {
	v := DownloaderView{ID: d.ID, Name: d.Name, Kind: d.Adapter, Protocol: d.Protocol, URL: d.URL, Username: d.Username, Category: d.Category,
		CategoryMovies: d.CategoryMovies, CategoryShows: d.CategoryShows, CategoryMusic: d.CategoryMusic,
		Priority: d.Priority, PathMap: d.PathMap, AllowLocal: d.AllowLocal, Health: HealthOf(d, time.Now()), CheckedAt: d.CheckedAt, Active: active,
		Writable: d.Writable, WritableLocked: ReadOnlyByEnv(d), ClearFinished: d.ClearFinished, Tags: d.Tags,
		AfterSeeding: seedingView(d)}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if v.PathMap == nil {
		v.PathMap = []domain.PathMap{}
	}
	if d.SecretRef != nil {
		v.Secret = secrets.Masked
	}
	return v
}

func (a *Web) Manager() *Manager {
	if a.Downloads == nil {
		m := &Manager{Store: a.Store}
		if a.Live != nil {
			m.Live = a.Live
		}
		if a.Secrets != nil {
			m.FromVault = a.Secrets.Get
		}
		a.Downloads = m
	}
	return a.Downloads
}

func (a *Web) listDownloaders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	db := a.Store.Read()
	dls, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 ORDER BY priority, name")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	counts := map[int64]int{}
	rows, err := db.QueryContext(ctx, "SELECT downloader_id, count(*) FROM downloads WHERE state IN ('sent', 'queued', 'downloading', 'stalled', 'paused') GROUP BY downloader_id")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for rows.Next() {
		var id int64
		var n int
		if rows.Scan(&id, &n) == nil {
			counts[id] = n
		}
	}
	rows.Close()
	out := []DownloaderView{}
	for _, d := range dls {
		out = append(out, downloaderView(d, counts[d.ID]))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"downloaders": out})
}

const downloaderHelp = `send name, kind (qbittorrent, transmission, deluge, rtorrent, aria2, sabnzbd or nzbget), url, and if needed secret (an API key or password; qBittorrent takes its API key with no username, or a username and password; aria2 takes its RPC secret and no username), category, priority (1 to 50), path_map [{theirs, ours}], allow_local_network, writable, clear_finished`

func downloaderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUncertain):
		webserver.Fail(w, http.StatusConflict, ErrUncertain.Error())
	case errors.Is(err, ErrAttemptStopped):
		webserver.Fail(w, http.StatusConflict, ErrAttemptStopped.Error())
	case errors.Is(err, ErrPendingAttempt):
		webserver.Fail(w, http.StatusConflict, ErrPendingAttempt.Error())
	case errors.Is(err, ErrNameUsed), errors.Is(err, ErrKeyAgain), errors.Is(err, ErrBusy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoVault):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, database.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		webserver.Fail(w, http.StatusNotFound, "no such Downloader")
	case errors.Is(err, ErrName), errors.Is(err, ErrAdapter), errors.Is(err, ErrURL), errors.Is(err, ErrCategory), errors.Is(err, ErrDelugeLabel), errors.Is(err, ErrAria2Folder), errors.Is(err, ErrSeedGoal), errors.Is(err, ErrSeedTorrent),
		errors.Is(err, ErrPriority), errors.Is(err, ErrPathMap), errors.Is(err, ErrUsername), errors.Is(err, ErrKeyNeed),
		errors.Is(err, ErrQbitKey):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		webserver.Internal(w, "api", err)
	}
}

func readDownloader(w http.ResponseWriter, r *http.Request) (Input, bool) {
	var in Input
	if !webserver.Decode(w, r, &in, downloaderHelp) {
		return in, false
	}
	if in.Secret == secrets.Masked {
		in.Secret = ""
	}
	return in, true
}

func idIn(w http.ResponseWriter, r *http.Request, what string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such "+what)
		return 0, false
	}
	return id, true
}

func (a *Web) publishDownloader(d domain.Downloader) {
	if a.Live != nil {
		a.Live.Publish("downloader", map[string]any{"id": d.ID, "health": HealthOf(d, time.Now())})
	}
}

func (a *Web) addDownloader(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	in, ok := readDownloader(w, r)
	if !ok {
		return
	}
	d, err := Add(r.Context(), a.Store, a.Secrets, in, actor(r))
	if err != nil {
		downloaderError(w, err)
		return
	}
	d, _ = a.Manager().LearnSaveRoot(r.Context(), d, actor(r))
	a.publishDownloader(d)
	webserver.WriteJSON(w, http.StatusCreated, downloaderView(d, 0))
}

func (a *Web) editDownloader(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	in, ok := readDownloader(w, r)
	if !ok {
		return
	}
	// A Downloader that had no path pair gets its save root; pairs the owner
	// removes in this edit stay removed.
	var before domain.Downloader
	_ = database.Get(r.Context(), a.Store.Read(), "downloaders", id, &before)
	d, err := Change(r.Context(), a.Store, a.Secrets, id, in, actor(r))
	if err != nil {
		downloaderError(w, err)
		return
	}
	a.Manager().Forget(id)
	if len(before.PathMap) == 0 {
		d, _ = a.Manager().LearnSaveRoot(r.Context(), d, actor(r))
	}
	a.publishDownloader(d)
	webserver.WriteJSON(w, http.StatusOK, downloaderView(d, 0))
}

func (a *Web) deleteDownloader(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	if err := Remove(r.Context(), a.Store, id, actor(r)); err != nil {
		downloaderError(w, err)
		return
	}
	a.Manager().Forget(id)
	a.forgetTorrents(r.Context(), id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Web) setDownloaderPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	d, err := SetPaused(r.Context(), a.Store, id, paused, actor(r))
	if err != nil {
		downloaderError(w, err)
		return
	}
	a.publishDownloader(d)
	webserver.WriteJSON(w, http.StatusOK, downloaderView(d, 0))
}

func (a *Web) pauseDownloader(w http.ResponseWriter, r *http.Request) {
	a.setDownloaderPaused(w, r, true)
}
func (a *Web) resumeDownloader(w http.ResponseWriter, r *http.Request) {
	a.setDownloaderPaused(w, r, false)
}

// DownloaderAttemptView is the deliberately small public form of an attempt.
// The opaque marker and diagnostic detail stay inside the Downloader manager.
type DownloaderAttemptView struct {
	ID           int64  `json:"id"`
	DownloaderID int64  `json:"downloader_id"`
	ReleaseID    int64  `json:"release_id"`
	SlotID       int64  `json:"slot_id"`
	State        string `json:"state"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func downloaderAttemptView(attempt Attempt) DownloaderAttemptView {
	return DownloaderAttemptView{
		ID: attempt.ID, DownloaderID: attempt.DownloaderID, ReleaseID: attempt.ReleaseID, SlotID: attempt.SlotID,
		State: string(attempt.State), CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt,
	}
}

// listDownloaderAttempts shows the durable send journal without its opaque
// reconciliation marker, Downloader address or internal error details.
func (a *Web) listDownloaderAttempts(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	attempts, err := a.Manager().Attempts(r.Context(), id)
	if err != nil {
		downloaderError(w, err)
		return
	}
	out := make([]DownloaderAttemptView, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, downloaderAttemptView(attempt))
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"attempts": out})
}

// abandonDownloaderAttempt makes one uncertain send terminal. It never asks
// the Downloader to remove or cancel anything.
func (a *Web) abandonDownloaderAttempt(w http.ResponseWriter, r *http.Request) {
	downloaderID, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	attemptID, err := strconv.ParseInt(r.PathValue("attemptID"), 10, 64)
	if err != nil || attemptID < 1 {
		webserver.Fail(w, http.StatusNotFound, "no such send attempt")
		return
	}
	if err := a.Manager().AbandonAttempt(r.Context(), downloaderID, attemptID, actor(r)); err != nil {
		if errors.Is(err, database.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			webserver.Fail(w, http.StatusNotFound, "no such send attempt")
			return
		}
		downloaderError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// explainDownloader says a failed test in plain words. The error never
// holds a key: adapters strip addresses from network errors.
func explainDownloader(err error) string {
	switch {
	case errors.Is(err, ErrQbitBanned):
		return "qBittorrent has banned this address after too many wrong passwords. Wait for the ban to end (an hour by default), check the password, then test again."
	case errors.Is(err, ErrQbitKeyRefused):
		return "qBittorrent turned down the API key. Check the key; the API key needs qBittorrent 5.2 or newer, so on an older qBittorrent sign in with user name and password instead."
	case errors.Is(err, ErrAuth):
		return "It turned down the key or sign-in. Check them and test again."
	case errors.Is(err, ErrNoCategory):
		return "It answered, but " + err.Error() + "."
	case errors.Is(err, outbound.ErrAddressRefused):
		return "The address is on this machine or the local network. Turn on May reach the local network if that is where it runs."
	case errors.Is(err, context.DeadlineExceeded):
		return "It did not answer within 20 seconds."
	}
	msg := firstLine(err.Error())
	if strings.Contains(msg, "connection refused") {
		return "Nothing answers at that address and port. Is it running?"
	}
	return "It did not answer: " + msg
}

// mayKeepSaveRoot says whether a test may store the save root. Like an edit,
// that takes a browser session or an API key (no sign-in is for tests).
func mayKeepSaveRoot(r *http.Request) bool {
	_, signedIn := webserver.From(r.Context())
	return !signedIn || webserver.Strong(r.Context())
}

// TestDownloader asks a saved Downloader for its version and says the
// result in plain words. A working answer ends a rest.
func (a *Web) TestDownloader(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	var d domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &d); err != nil || d.Removed {
		downloaderError(w, database.ErrNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	m := a.Manager()
	m.Forget(id)
	start := time.Now()
	h, err := m.CheckNow(ctx, d)
	out := map[string]any{"ok": err == nil, "ms": time.Since(start).Milliseconds()}
	if err != nil {
		out["text"] = explainDownloader(err)
	} else {
		out["text"] = "It answered: " + d.Adapter + " " + h.Version + "."
		out["version"] = h.Version
		if h.Note != "" {
			out["text"] = out["text"].(string) + " " + h.Note
		}
		// A Downloader saved before it had a save root gets it on this test.
		if len(d.PathMap) == 0 && mayKeepSaveRoot(r) {
			if _, kept := m.LearnSaveRoot(ctx, d, actor(r)); len(kept) > 0 {
				out["text"] = out["text"].(string) + " HolyArrs reads its finished downloads from " + strings.Join(kept, ", ") + "."
			}
		}
	}
	_ = database.Get(r.Context(), a.Store.Read(), "downloaders", id, &d)
	out["health"] = HealthOf(d, time.Now())
	a.publishDownloader(d)
	webserver.WriteJSON(w, http.StatusOK, out)
}

// markFailed is "Mark failed" on an active Download.
func (a *Web) markFailed(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	who := ""
	if p, ok := webserver.From(r.Context()); ok {
		who = p.Name
	}
	err := a.Manager().MarkFailed(r.Context(), id, actor(r), who)
	switch {
	case errors.Is(err, database.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		webserver.Fail(w, http.StatusNotFound, "no such download")
	case errors.Is(err, ErrNotActive):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		var d domain.Download
		_ = database.Get(r.Context(), a.Store.Read(), "downloads", id, &d)
		webserver.WriteJSON(w, http.StatusOK, DownloadViewOf(d))
	}
}

// DownloadView is a Download in plain words for the movie page.
type DownloadView struct {
	ID       int64   `json:"id"`
	State    string  `json:"state"`
	Label    string  `json:"label"`
	Text     string  `json:"text"`
	Progress float64 `json:"progress"`
	Speed    int64   `json:"speed"`
	UpSpeed  int64   `json:"up_speed"`
	Seeds    int     `json:"seeds"`
	Peers    int     `json:"peers"`
	// Whole-swarm counts, when the Downloader reports them.
	SeedsTotal int    `json:"seeds_total,omitempty"`
	PeersTotal int    `json:"peers_total,omitempty"`
	ETA        *int64 `json:"eta,omitempty"`
	Active     bool   `json:"active"`
	// Pack is set for a season pack fetched a few episodes at a time.
	Pack *PackRange `json:"pack,omitempty"`
}

// PackRange is the switched-on episodes of a season pack fetched a few at a
// time: the first and last episode numbers, how many are on and how many the
// pack holds.
type PackRange struct {
	From  int `json:"from"`
	To    int `json:"to"`
	Taken int `json:"taken"`
	Total int `json:"total"`
}

func packRangeOf(files []domain.PackFile) *PackRange {
	if len(files) == 0 {
		return nil
	}
	r := &PackRange{}
	for _, f := range files {
		r.Total += len(f.Eps)
		for _, e := range f.Eps {
			if !f.On {
				continue
			}
			r.Taken++
			if r.From == 0 || e < r.From {
				r.From = e
			}
			r.To = max(r.To, e)
		}
	}
	return r
}

// DownloadLabels are the words people see for Download states.
var DownloadLabels = map[string]string{
	"sent": "Sent", "queued": "Waiting its turn", "downloading": "Downloading", "stalled": "Stalled", "paused": "Paused in the Downloader",
	"finished": "Finished", "failed": "Failed", "vanished": "Removed outside HolyArrs", "seeding": "Finished, sharing", "cleaned": "Cleaned up",
}

func DownloadViewOf(d domain.Download) DownloadView {
	v := DownloadView{ID: d.ID, State: d.State, Label: DownloadLabels[d.State], Progress: d.Progress, Speed: d.Speed, UpSpeed: d.UpSpeed, Seeds: d.Seeds, Peers: d.Peers, ETA: d.ETA, Pack: packRangeOf(d.PackFiles)}
	switch d.State {
	case "sent":
		v.Text = "Sent to the Downloader; waiting for it to start."
	case "queued":
		v.Text = "The Downloader has it and will start it when there is room."
	case "downloading":
		v.Text = strconv.Itoa(int(d.Progress*100)) + "% done."
	case "paused":
		v.Text = "Someone paused it in the Downloader. HolyArrs waits."
	case "finished":
		v.Text = "Done; HolyArrs puts it in the library."
	}
	if d.StateReason != nil && *d.StateReason != "" && (d.State == "stalled" || d.State == "failed" || d.State == "vanished" ||
		d.State == "paused" && strings.HasPrefix(*d.StateReason, WaitingReason)) {
		r, n := utf8.DecodeRuneInString(*d.StateReason)
		v.Text = string(unicode.ToUpper(r)) + (*d.StateReason)[n:] + "."
	}
	switch d.State {
	case "sent", "queued", "downloading", "stalled", "paused":
		v.Active = true
	}
	return v
}

// DownloaderSettings are the choices on Settings > Downloaders.
type DownloaderSettings struct {
	StalledAfterHours  float64 `json:"stalled_after_hours"`
	StalledActHours    float64 `json:"stalled_act_hours"`
	RemoveFailed       bool    `json:"remove_failed"`
	FailDangerous      bool    `json:"fail_dangerous"`
	ReplaceVanished    bool    `json:"replace_vanished"`
	GrabSpacingSeconds float64 `json:"grab_spacing_seconds"`
	TryNextDownloader  bool    `json:"try_next_downloader"`
	// The Remove dialog starts from these (stage 4.1.1).
	RemoveDeleteFiles bool `json:"remove_delete_files"`
	RemoveTurnDown    bool `json:"remove_turn_down"`
	RemoveSearchAgain bool `json:"remove_search_again"`
	// Sync is downloads.sync: always "on" now (the background
	// reader always runs); a saved value is ignored. SABOutside is
	// downloads.sab_outside; left out of a save, it stays as it is.
	Sync       *string `json:"downloads_sync,omitempty"`
	SABOutside *bool   `json:"downloads_sab_outside,omitempty"`
	// AutoAdd is downloads_auto_add: a sure match of an outside item whose
	// title is not in the Library adds it (no search). Off by default.
	AutoAdd *bool `json:"downloads_auto_add,omitempty"`
	// The dead or crawling torrent rule, 0 for off (the default). Left out of
	// a save, each stays as it is.
	DeadMetadataMinutes *float64 `json:"dead_metadata_minutes,omitempty"`
	CrawlKBps           *float64 `json:"crawl_kbps,omitempty"`
	CrawlHours          *float64 `json:"crawl_hours,omitempty"`
}

func (a *Web) readDownloaderSettings(ctx context.Context) (DownloaderSettings, error) {
	st := DownloaderSettings{StalledAfterHours: 6, StalledActHours: 24, RemoveFailed: true, FailDangerous: true, ReplaceVanished: true, GrabSpacingSeconds: 2, TryNextDownloader: true,
		RemoveDeleteFiles: true, RemoveSearchAgain: true}
	rows, err := a.Store.Read().QueryContext(ctx, "SELECT grp, key, value FROM settings WHERE grp IN ('links', 'what_to_pick')")
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var g, k, v string
		if err := rows.Scan(&g, &k, &v); err != nil {
			return st, err
		}
		n, _ := strconv.ParseFloat(v, 64)
		switch g + "/" + k {
		case "links/" + SettingStallAfter:
			st.StalledAfterHours = n
		case "links/" + SettingActAfter:
			st.StalledActHours = n
		case "links/" + SettingSpacing:
			st.GrabSpacingSeconds = n
		case "links/" + SettingRemoveFailed:
			st.RemoveFailed = v != "false"
		case "links/" + SettingFailDangerous:
			st.FailDangerous = v != "false"
		case "links/" + SettingReplaceVanished:
			st.ReplaceVanished = v != "false"
		case "links/remove_delete_files":
			st.RemoveDeleteFiles = v != "false"
		case "links/remove_turn_down":
			st.RemoveTurnDown = v == "true"
		case "links/remove_search_again":
			st.RemoveSearchAgain = v != "false"
		case "what_to_pick/try_next_downloader":
			st.TryNextDownloader = v != "false"
		case "links/" + SettingSABOutside:
			on := v != "false"
			st.SABOutside = &on
		case "links/" + settingAutoAdd:
			on := v == "true"
			st.AutoAdd = &on
		case "links/" + SettingDeadMetadata:
			st.DeadMetadataMinutes = &n
		case "links/" + SettingCrawlKBps:
			st.CrawlKBps = &n
		case "links/" + SettingCrawlHours:
			st.CrawlHours = &n
		}
	}
	mode := ModeOn // any stored value reads as on
	st.Sync = &mode
	if st.AutoAdd == nil {
		off := false
		st.AutoAdd = &off
	}
	for _, p := range []**float64{&st.DeadMetadataMinutes, &st.CrawlKBps, &st.CrawlHours} {
		if *p == nil {
			*p = new(float64)
		}
	}
	if st.SABOutside == nil {
		on := true // shown by default
		st.SABOutside = &on
	}
	return st, rows.Err()
}

func (a *Web) getDownloaderSettings(w http.ResponseWriter, r *http.Request) {
	st, err := a.readDownloaderSettings(r.Context())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, st)
}

func (a *Web) putDownloaderSettings(w http.ResponseWriter, r *http.Request) {
	var in DownloaderSettings
	if !webserver.Decode(w, r, &in, "body is the Downloader settings") {
		return
	}
	if in.StalledAfterHours < 1 || in.StalledAfterHours > 168 {
		webserver.Fail(w, http.StatusBadRequest, "stalled_after_hours is 1 to 168")
		return
	}
	if in.StalledActHours < in.StalledAfterHours || in.StalledActHours > 720 {
		webserver.Fail(w, http.StatusBadRequest, "stalled_act_hours is at least stalled_after_hours and at most 720")
		return
	}
	if in.GrabSpacingSeconds < 0 || in.GrabSpacingSeconds > 60 {
		webserver.Fail(w, http.StatusBadRequest, "grab_spacing_seconds is 0 to 60")
		return
	}
	if msg := deadRuleProblem(in); msg != "" {
		webserver.Fail(w, http.StatusBadRequest, msg)
		return
	}
	by := actor(r)
	now := database.FormatTime(time.Now())
	byText := actorJSONText(by)
	vals := []struct{ grp, key, value string }{
		{"links", SettingStallAfter, strconv.FormatFloat(in.StalledAfterHours, 'f', -1, 64)},
		{"links", SettingActAfter, strconv.FormatFloat(in.StalledActHours, 'f', -1, 64)},
		{"links", SettingSpacing, strconv.FormatFloat(in.GrabSpacingSeconds, 'f', -1, 64)},
		{"links", SettingRemoveFailed, strconv.FormatBool(in.RemoveFailed)},
		{"links", SettingFailDangerous, strconv.FormatBool(in.FailDangerous)},
		{"links", SettingReplaceVanished, strconv.FormatBool(in.ReplaceVanished)},
		{"what_to_pick", "try_next_downloader", strconv.FormatBool(in.TryNextDownloader)},
		{"links", "remove_delete_files", strconv.FormatBool(in.RemoveDeleteFiles)},
		{"links", "remove_turn_down", strconv.FormatBool(in.RemoveTurnDown)},
		{"links", "remove_search_again", strconv.FormatBool(in.RemoveSearchAgain)},
	}
	if in.SABOutside != nil {
		vals = append(vals, struct{ grp, key, value string }{"links", SettingSABOutside, strconv.FormatBool(*in.SABOutside)})
	}
	if in.AutoAdd != nil {
		vals = append(vals, struct{ grp, key, value string }{"links", settingAutoAdd, strconv.FormatBool(*in.AutoAdd)})
	}
	for _, f := range []struct {
		key string
		v   *float64
	}{{SettingDeadMetadata, in.DeadMetadataMinutes}, {SettingCrawlKBps, in.CrawlKBps}, {SettingCrawlHours, in.CrawlHours}} {
		if f.v != nil {
			vals = append(vals, struct{ grp, key, value string }{"links", f.key, strconv.FormatFloat(*f.v, 'f', -1, 64)})
		}
	}
	err := a.Store.Write(r.Context(), func(tx *sql.Tx) error {
		for _, v := range vals {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				v.grp, v.key, v.value, now, byText, now, byText); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// The change is logged once by logSettingsChanges as an audit Event (area
	// and who, never the values), so it is not written or sent live here.
	a.getDownloaderSettings(w, r)
}

// deadRuleProblem checks the dead or crawling torrent rule numbers; 0 is off.
func deadRuleProblem(in DownloaderSettings) string {
	in.DeadMetadataMinutes, in.CrawlKBps, in.CrawlHours = orZero(in.DeadMetadataMinutes), orZero(in.CrawlKBps), orZero(in.CrawlHours)
	switch {
	case *in.DeadMetadataMinutes != 0 && (*in.DeadMetadataMinutes < 5 || *in.DeadMetadataMinutes > 1440):
		return "dead_metadata_minutes is 0 (off) or 5 to 1440"
	case *in.CrawlKBps < 0 || *in.CrawlKBps > 100000:
		return "crawl_kbps is 0 (off) to 100000"
	case *in.CrawlHours != 0 && (*in.CrawlHours < 1 || *in.CrawlHours > 168):
		return "crawl_hours is 0 (off) or 1 to 168"
	}
	return ""
}

func orZero(p *float64) *float64 {
	if p == nil {
		return new(float64)
	}
	return p
}
