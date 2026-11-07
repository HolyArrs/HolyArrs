package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Stage 4.1: the Downloads screen and Needs a decision.
func (a *API) downloadsRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/downloads/view", a.downloadsView)
	m.HandleFunc("GET /api/v1/downloads/speeds", a.speedHistory)
	m.HandleFunc("POST /api/v1/downloads/{id}/remove", a.removeDownload)
	m.HandleFunc("GET /api/v1/needs-decision", a.needsDecision)
	a.downloadControlRoutes(m)
	a.clientMatchRoutes(m)
	m.HandleFunc("POST /api/v1/needs-decision/{id}/decide", a.decide)
}

// DownloadRow is one row of the Downloads screen, in plain words.
type DownloadRow struct {
	DownloadView
	Phase      string `json:"phase"` // waiting, downloading, stalled, paused, failed, vanished, removed, waiting_to_import, importing, needs_a_decision, done
	Name       string `json:"name"`
	TitleID    int64  `json:"title_id,omitempty"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	Version    string `json:"version"`
	Downloader string `json:"downloader"`
	// Path is the folder or file the Downloader holds it in, as it sees it
	// (the Remove confirm names it when files go too). Empty when unknown.
	Path string `json:"path,omitempty"`
	// DownloaderKind is the Downloader's adapter (qbittorrent, sabnzbd...) and
	// Protocol its torrent or usenet.
	DownloaderKind string     `json:"downloader_kind,omitempty"`
	Protocol       string     `json:"protocol,omitempty"`
	Size           int64      `json:"size"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
	SlotState      string     `json:"slot_state,omitempty"`
	CanAct         bool       `json:"can_act"` // Mark failed and Remove
	// ClientLocked says HolyArrs may not change this row's Downloader (its switch
	// is off, or it is listed read only): pause, queue and torrent controls are off,
	// Mark failed and Remove (HolyArrs' own records) still work.
	ClientLocked bool `json:"client_locked,omitempty"`
	// Replaced says a failed copy was turned down and HolyArrs moved on (it
	// searches again or already has another copy), so it is no problem.
	Replaced bool `json:"replaced,omitempty"`
	// Shows: the kind of Title, the episodes the download covers and a
	// short line naming them ("S01E01-E03, 3 episodes").
	// Artists: Episodes lists the tracks and Album names the album.
	Kind     string        `json:"kind,omitempty"` // movie, show, artist
	Episodes []EpisodeLine `json:"episodes,omitempty"`
	Covers   string        `json:"covers,omitempty"`
	Album    string        `json:"album,omitempty"`
	// Client items: a torrent read straight from the client's own list.
	// ClientItem rows have no HolyArrs download id (ID 0) and are addressed
	// by DownloaderID and Hash; Outside says HolyArrs never sent it.
	ClientItem bool `json:"client_item,omitempty"`
	Outside    bool `json:"outside,omitempty"`
	// Linked says an outside torrent is matched to a Library title (a
	// client_links row): Title, Kind and Covers come from that link.
	Linked       bool   `json:"linked,omitempty"`
	DownloaderID int64  `json:"downloader_id,omitempty"`
	Hash         string `json:"hash,omitempty"`
	Category     string `json:"category,omitempty"`
	// RequestedBy names who added the row's Library title ("Added in
	// <client>" for an outside item whose title nobody added by hand).
	// SourceName and SourceID are the Source the release came from, for
	// downloads HolyArrs sent; PostedAt when the Usenet post was made.
	RequestedBy string     `json:"requested_by,omitempty"`
	SourceName  string     `json:"source_name,omitempty"`
	SourceID    int64      `json:"source_id,omitempty"`
	PostedAt    *time.Time `json:"posted_at,omitempty"`
	// Ratio is a torrent's share ratio as the client reports it; nil unknown.
	Ratio *float64 `json:"ratio,omitempty"`
	// Limits are a client torrent's own seeding rules (-2 the client's
	// global rule, -1 none), when the client reports them.
	Limits *downloader.ShareLimits `json:"limits,omitempty"`
	// ClientHint is a client torrent's finer state where the phase is too
	// coarse: checking, moving, errored, missing (files gone) or idle_seed
	// (finished, nobody asking for it now).
	ClientHint string `json:"client_hint,omitempty"`
	// QueuePos is a client torrent's place in the client's queue (0 not
	// queued); Tracker the host of the tracker it works with now.
	QueuePos int    `json:"queue_pos,omitempty"`
	Tracker  string `json:"tracker,omitempty"`
	// TrackerProblem: error, warning or announce (no tracker answered).
	TrackerProblem string `json:"tracker_problem,omitempty"`
	// Availability is a downloading torrent's full copies in the swarm
	// (under 1: nobody holds every piece); nil when unknown.
	Availability *float64 `json:"availability,omitempty"`
	// LiveKey is the row's key in push updates ("<downloader id>:<external
	// id>"); empty for rows no client item backs.
	LiveKey string `json:"live_key,omitempty"`
	// LimitDown and LimitUp are a client torrent's own speed caps (bytes per
	// second, absent when none).
	LimitDown int64 `json:"limit_down,omitempty"`
	LimitUp   int64 `json:"limit_up,omitempty"`
}

// DownloadTotals are the bytes per second moving across every active download.
type DownloadTotals struct {
	Speed   int64 `json:"speed"`
	UpSpeed int64 `json:"up_speed"`
}

// EpisodeLine is one episode (or track) a download covers, with where its
// Slot is. A track carries its disc in Season and its number in Episode.
type EpisodeLine struct {
	SlotID  int64  `json:"slot_id"`
	Season  int    `json:"season"`
	Episode int    `json:"episode"`
	Label   string `json:"label"` // S01E02; a track: Track 02, Disc 2 track 03
	Name    string `json:"name,omitempty"`
	State   string `json:"state"`
	Track   bool   `json:"track,omitempty"`
}

// episodeLines reads the episodes of a show download's Slots, in order.
func episodeLines(ctx context.Context, db *sql.DB, slotIDs []int64) []EpisodeLine {
	out := []EpisodeLine{}
	for _, id := range slotIDs {
		var e EpisodeLine
		var season, episode sql.NullInt64
		var name sql.NullString
		var kind string
		if db.QueryRowContext(ctx, `SELECT s.id, s.state, i.kind, CASE i.kind WHEN 'track' THEN COALESCE(json_extract(i.numbers, '$.disc'), 1) ELSE json_extract(i.numbers, '$.season') END,
			CASE i.kind WHEN 'track' THEN json_extract(i.numbers, '$.track') ELSE json_extract(i.numbers, '$.episode') END, i.name
			FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?`, id).Scan(&e.SlotID, &e.State, &kind, &season, &episode, &name) != nil {
			continue
		}
		e.Season, e.Episode, e.Name = int(season.Int64), int(episode.Int64), name.String
		labelLine(&e, kind)
		out = append(out, e)
	}
	sortLines(out)
	return out
}

// labelLine names an episode line: S01E02, or for a track Track 02 or
// Disc 2 track 03.
func labelLine(e *EpisodeLine, kind string) {
	e.Label = fmt.Sprintf("S%02dE%02d", e.Season, e.Episode)
	if kind == "track" {
		e.Track = true
		e.Label = fmt.Sprintf("Track %02d", e.Episode)
		if e.Season > 1 {
			e.Label = fmt.Sprintf("Disc %d track %02d", e.Season, e.Episode)
		}
	}
}

// sortLines puts episode lines in season and episode order.
func sortLines(out []EpisodeLine) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Season != out[j].Season {
			return out[i].Season < out[j].Season
		}
		return out[i].Episode < out[j].Episode
	})
}

// covers names the episodes in a short line: "S01E02 The Mill" for one,
// "S01E01-E03, 3 episodes" for a run in one season, else the first few.
func covers(eps []EpisodeLine) string {
	switch len(eps) {
	case 0:
		return ""
	case 1:
		return strings.TrimSpace(eps[0].Label + " " + eps[0].Name)
	}
	if eps[0].Track {
		// An album: "12 tracks", or "2 discs, 20 tracks".
		discs := map[int]bool{}
		for _, e := range eps {
			discs[e.Season] = true
		}
		if len(discs) > 1 {
			return fmt.Sprintf("%d discs, %d tracks", len(discs), len(eps))
		}
		return fmt.Sprintf("%d tracks", len(eps))
	}
	run := true
	for i := 1; i < len(eps); i++ {
		run = run && eps[i].Season == eps[0].Season && eps[i].Episode == eps[i-1].Episode+1
	}
	if run {
		return fmt.Sprintf("%s-E%02d, %d episodes", eps[0].Label, eps[len(eps)-1].Episode, len(eps))
	}
	var labels []string
	for i, e := range eps {
		if i == 4 {
			labels = append(labels, fmt.Sprintf("and %d more", len(eps)-4))
			break
		}
		labels = append(labels, e.Label)
	}
	return strings.Join(labels, ", ") + fmt.Sprintf(" (%d episodes)", len(eps))
}

// phaseOfMany is a show download's phase once the Downloader is done:
// the most pressing place among its Slots (importing, then needs a
// decision, then waiting), done when every Slot has settled.
func phaseOfMany(d domain.Download, eps []EpisodeLine) string {
	p := phase(d, "")
	if p != "done" || len(eps) == 0 {
		return p
	}
	has := map[string]bool{}
	for _, e := range eps {
		has[phase(d, e.State)] = true
	}
	for _, p := range []string{"importing", "needs_a_decision", "waiting_to_import"} {
		if has[p] {
			return p
		}
	}
	if len(has) == 1 && has["import_refused"] {
		return "import_refused"
	}
	return "done"
}

// showText says in plain words where the episodes of a finished show
// download are: "2 of 3 episodes in the library; S01E03 looks again."
func showText(ph string, eps []EpisodeLine) string {
	in, decide, again := 0, []string{}, []string{}
	for _, e := range eps {
		switch domain.SlotState(e.State) {
		case domain.SlotInLibrary, domain.SlotBetterWanted:
			in++
		case domain.SlotNeedsADecision:
			decide = append(decide, e.Label)
		case domain.SlotWanted, domain.SlotRejected, domain.SlotSearching, domain.SlotChosen, domain.SlotHeld, domain.SlotFailed:
			again = append(again, e.Label)
		}
	}
	if len(eps) < 2 {
		return ""
	}
	noun := "episodes"
	if eps[0].Track {
		noun = "tracks"
	}
	parts := []string{fmt.Sprintf("%d of %d %s in the library", in, len(eps), noun)}
	if len(decide) > 0 {
		parts = append(parts, strings.Join(decide, ", ")+" "+map[bool]string{true: "needs", false: "need"}[len(decide) == 1]+" a decision")
	}
	if len(again) > 0 {
		parts = append(parts, strings.Join(again, ", ")+" "+map[bool]string{true: "looks", false: "look"}[len(again) == 1]+" for another copy")
	}
	if ph == "importing" {
		return "HolyArrs is putting the files in the library."
	}
	return strings.Join(parts, "; ") + "."
}

// PhaseLabels are the words people see on the Downloads screen.
var PhaseLabels = map[string]string{
	"waiting": "Waiting its turn", "downloading": "Downloading", "stalled": "Stalled", "paused": "Paused in the Downloader",
	"failed": "Failed", "vanished": "Failed", "removed": "Removed", "waiting_to_import": "Waiting to import",
	"importing": "Importing", "needs_a_decision": "Review", "import_refused": "Import refused", "done": "Done",
}

// phase is a Download's place in plain words: its own state while it
// downloads, then where its Slot is once the Downloader is done.
func phase(d domain.Download, slot string) string {
	switch d.State {
	case "sent", "queued":
		return "waiting"
	case "downloading", "stalled", "paused", "failed", "vanished":
		return d.State
	case "cleaned":
		return "removed"
	}
	switch domain.SlotState(slot) {
	case domain.SlotImporting:
		return "importing"
	case domain.SlotNeedsADecision:
		return "needs_a_decision"
	case domain.SlotRejected:
		return "import_refused"
	case domain.SlotDownloading, domain.SlotStalled, domain.SlotChosen:
		return "waiting_to_import"
	}
	return "done"
}

func (a *API) downloadsView(w http.ResponseWriter, r *http.Request) {
	// People who may not manage the Library watch only the downloads for
	// titles they asked for, and act on none of them.
	// (No person on the request: the sign-in layer is off, as in tests.)
	who, ok := auth.From(r.Context())
	manage := !ok || who.Has(auth.PermManageLibrary)
	where, args := "1=1", []any{}
	// People who approve read the downloads of every approved ask on the
	// Requests page (?asked=1), as they read everyone's asks there (#139);
	// the rows stay the plain ones members get.
	asked := r.URL.Query().Get("asked") == "1"
	if !manage && !(asked && ok && who.Has(auth.PermApprove)) {
		where = `EXISTS (SELECT 1 FROM json_each(downloads.slot_ids) j JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id
			JOIN asks a ON a.title_id = i.title_id JOIN ask_people p ON p.ask_id = a.id WHERE p.user_id = ?)`
		args = append(args, who.UserID)
	}
	// ?client=<id> reads one client's rows only (the page lost that client's
	// patches). Its rev is taken before the rows, so the rows are at least as
	// new as the rev and the patches after it apply on top.
	only, _ := strconv.ParseInt(r.URL.Query().Get("client"), 10, 64)
	var revs map[int64]uint64
	if a.syncOn() {
		revs = a.s.DLSync.Store.Revs()
		if only > 0 {
			revs = map[int64]uint64{only: revs[only]}
		}
	}
	if only > 0 {
		where += " AND downloader_id = ?"
		args = append(args, only)
	}
	// ?asked=1 reads every download for a title with an approved ask, with
	// no 500-row cap, so the Requests page counts failed asks exactly.
	n := limit(r, 100)
	if asked {
		where += ` AND EXISTS (SELECT 1 FROM json_each(downloads.slot_ids) j JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id
			JOIN asks a ON a.title_id = i.title_id WHERE a.state = 'approved')`
		n = -1
	}
	ds, err := store.Select[domain.Download](r.Context(), a.read(), "downloads", where+" ORDER BY id DESC LIMIT ?", append(args, n)...)
	if err != nil {
		internal(w, err)
		return
	}
	db := a.read()
	ctx := r.Context()
	// One read per related table for the whole page (no read per row).
	names := a.downloaderNames(ctx)
	relIDs, slotIDs := []int64{}, []int64{}
	for _, d := range ds {
		if d.ReleaseID != nil {
			relIDs = append(relIDs, *d.ReleaseID)
		}
		slotIDs = append(slotIDs, d.SlotIDs...)
	}
	rels := releaseNames(ctx, db, relIDs)
	slots := slotInfos(ctx, db, slotIDs)
	rows := make([]DownloadRow, 0, len(ds))
	shown := map[clientKey]bool{}
	for _, d := range ds {
		row := DownloadRow{DownloadView: downloadView(d), StartedAt: d.StartedAt, FinishedAt: d.FinishedAt, UpdatedAt: d.UpdatedAt, Name: d.ExternalID, Path: d.SavePath}
		if d.ExternalID != "" {
			shown[clientKey{d.DownloaderID, strings.ToLower(d.ExternalID)}] = true
			row.LiveKey = liveKey(d.DownloaderID, d.ExternalID)
		}
		n := names[d.DownloaderID]
		row.Downloader, row.DownloaderKind, row.Protocol = n.Name, n.Adapter, n.Protocol
		// A torrent of HolyArrs' own is also addressed by Downloader and hash,
		// so a selection can act on it together with outside torrents.
		if h := strings.ToLower(d.ExternalID); n.Protocol == "torrent" && validHash(h) {
			row.DownloaderID, row.Hash = d.DownloaderID, h
		}
		if d.ReleaseID != nil {
			if rl, ok := rels[*d.ReleaseID]; ok {
				row.Name, row.Size = rl.name, rl.size
				// Which Source and who asked are for people who manage the Library.
				if manage {
					row.SourceName, row.SourceID = rl.source, rl.sourceID
				}
				if n.Protocol == "usenet" {
					row.PostedAt = rl.posted
				}
			}
		}
		if len(d.SlotIDs) > 0 {
			if si, ok := slots[d.SlotIDs[0]]; ok && si.full {
				row.SlotState, row.TitleID, row.Title, row.Version, row.Kind = si.line.State, si.titleID, si.title, si.version, si.kind
				if si.year.Valid {
					y := int(si.year.Int64)
					row.Year = &y
				}
			}
		}
		row.Phase = phase(d, row.SlotState)
		// The save folder on the host is for people who manage the Library.
		if !manage {
			row.Path = ""
		}
		if row.Kind == "artist" && len(d.SlotIDs) > 0 {
			row.Album, row.Year = "", nil
			if si, ok := slots[d.SlotIDs[0]]; ok && si.album.Valid {
				row.Album = si.album.String
				if y, err := strconv.Atoi(si.albumYear.String); err == nil && y > 0 {
					row.Year = &y
				}
			}
		}
		if row.Kind == "show" || row.Kind == "artist" {
			row.Episodes = linesOf(d.SlotIDs, slots)
			row.Covers = covers(row.Episodes)
			row.Phase = phaseOfMany(d, row.Episodes)
		}
		row.Label = PhaseLabels[row.Phase]
		row.CanAct = row.Active && manage
		row.ClientLocked = downloader.ReadOnlyFor(n)
		if (d.State == "failed" || d.State == "vanished") && row.SlotState != "" && row.SlotState != string(domain.SlotFailed) && row.SlotState != string(domain.SlotNeedsADecision) {
			row.Replaced = true
		}
		if d.State == "cleaned" && d.StateReason != nil {
			row.Text = upperFirst(*d.StateReason) + "."
		}
		switch row.Phase {
		case "waiting_to_import":
			row.Text = "The Downloader is done; HolyArrs will import it next."
		case "importing":
			row.Text = "HolyArrs is putting the files in the library."
		case "needs_a_decision":
			row.Text = "The import could not finish on its own; see Needs a decision."
		case "import_refused":
			row.Text = "The files were not what the name said; HolyArrs looks for another copy."
		case "done":
			row.Text = "In the library."
		}
		if t := showText(row.Phase, row.Episodes); t != "" && d.State != "cleaned" && !row.Active {
			row.Text = t
		}
		rows = append(rows, row)
	}
	if manage {
		a.rowOwners(ctx, rows)
	}
	// Managers see everything in their torrent clients, as the client's own
	// screen does: the torrents HolyArrs did not send come after its own.
	// Rows come from the background reader's snapshots: a client that is
	// slow or down never holds the page up, and its health says why.
	var outside []DownloadRow
	health := []ClientHealth{}
	if manage {
		outside, health = a.clientRows(ctx, names, shown)
		// HolyArrs' own torrents take the ratio the reader's snapshot saw.
		if a.s.DLSync != nil {
			for i := range rows {
				if rows[i].Ratio == nil && rows[i].Protocol == "torrent" {
					rows[i].Ratio = a.storeRatio(rows[i].DownloaderID, rows[i].Hash)
				}
			}
		}
	}
	// Totals count every row, before the outside rows are paged: download
	// speed from what is moving, upload from everything that sends (seeding
	// torrents too).
	if only > 0 {
		keep := outside[:0:0]
		for _, row := range outside {
			if row.DownloaderID == only {
				keep = append(keep, row)
			}
		}
		outside = keep
	}
	total := viewTotals(rows, outside)
	page, next := pageOutside(outside, r.URL.Query().Get("cursor"), outsideLimit(r))
	rows = append(rows, page...)
	out := map[string]any{"downloads": rows, "totals": total, "clients": health, "outside_total": len(outside), "next_cursor": next}
	if a.syncOn() {
		// Someone is watching: the reader reads faster (no write, no call).
		a.s.DLSync.Watched()
		out["revs"] = revs
		if a.s.Live != nil {
			out["boot"] = a.s.Live.Boot()
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// viewTotals adds up the speeds of HolyArrs' rows and every client row.
func viewTotals(sets ...[]DownloadRow) DownloadTotals {
	var t DownloadTotals
	for _, rows := range sets {
		for _, row := range rows {
			if row.Active && row.State == "downloading" {
				t.Speed += row.Speed
			}
			t.UpSpeed += row.UpSpeed
		}
	}
	return t
}

// outsideCap is how many outside rows one view answer carries by default.
const outsideCap = 200

// outsideMax is the most outside rows one view answer carries.
const outsideMax = 2000

// outsideLimit is the page size for outside rows (?outside_limit=, 1-2000;
// more is held at 2000, so Show more past it never drops back to the first 200).
func outsideLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("outside_limit"))
	if err != nil || n <= 0 {
		return outsideCap
	}
	return min(n, outsideMax)
}

// pageOutside returns one page of outside rows from the cursor (an offset
// into the stable order clientRows gives) and the cursor of the next page,
// "" when this page is the last.
func pageOutside(rows []DownloadRow, cursor string, n int) ([]DownloadRow, string) {
	from, err := strconv.Atoi(cursor)
	if err != nil || from < 0 {
		from = 0
	}
	if from >= len(rows) {
		return nil, ""
	}
	to := min(from+n, len(rows))
	if to == len(rows) {
		return rows[from:to], ""
	}
	return rows[from:to], strconv.Itoa(to)
}

// releaseLine is a Release's name and size.
type releaseLine struct {
	name     string
	size     int64
	source   string
	sourceID int64
	posted   *time.Time
}

// inChunks calls f on ids in parts small enough for one IN (...) read.
func inChunks(ids []int64, f func(part []any, marks string)) {
	seen := map[int64]bool{}
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	for len(uniq) > 0 {
		n := min(len(uniq), 500)
		part := make([]any, n)
		for i, id := range uniq[:n] {
			part[i] = id
		}
		uniq = uniq[n:]
		f(part, "?"+strings.Repeat(", ?", n-1))
	}
}

// releaseNames reads the names and sizes of many Releases at once.
func releaseNames(ctx context.Context, db *sql.DB, ids []int64) map[int64]releaseLine {
	out := map[int64]releaseLine{}
	inChunks(ids, func(part []any, marks string) {
		rs, err := db.QueryContext(ctx, `SELECT r.id, r.name, r.size, r.source_id, COALESCE(s.name, ''), COALESCE(r.published_at, '')
			FROM releases r LEFT JOIN sources s ON s.id = r.source_id WHERE r.id IN (`+marks+")", part...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var id int64
			var v releaseLine
			var posted string
			if rs.Scan(&id, &v.name, &v.size, &v.sourceID, &v.source, &posted) == nil {
				if v.source == "" {
					v.sourceID = 0
				}
				if t, err := time.Parse(time.RFC3339Nano, posted); err == nil {
					t = t.UTC()
					v.posted = &t
				}
				out[id] = v
			}
		}
	})
	return out
}

// slotInfo is what the Downloads screen shows about one Slot: its episode
// line, and its Title, Version and (for tracks) album.
type slotInfo struct {
	line      EpisodeLine
	titleID   int64
	title     string
	year      sql.NullInt64
	kind      string
	version   string
	full      bool // the Slot has its Version (the Title line is shown)
	album     sql.NullString
	albumYear sql.NullString
}

// slotInfos reads many Slots at once.
func slotInfos(ctx context.Context, db *sql.DB, ids []int64) map[int64]slotInfo {
	out := map[int64]slotInfo{}
	inChunks(ids, func(part []any, marks string) {
		rs, err := db.QueryContext(ctx, `SELECT s.id, s.state, i.kind, CASE i.kind WHEN 'track' THEN COALESCE(json_extract(i.numbers, '$.disc'), 1) ELSE json_extract(i.numbers, '$.season') END,
			CASE i.kind WHEN 'track' THEN json_extract(i.numbers, '$.track') ELSE json_extract(i.numbers, '$.episode') END, i.name,
			t.id, t.name, t.year, t.kind, v.label, p.name, substr(p.release_date, 1, 4)
			FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
			LEFT JOIN versions v ON v.id = s.version_id LEFT JOIN parts p ON p.id = i.part_id WHERE s.id IN (`+marks+`)`, part...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var si slotInfo
			var season, episode sql.NullInt64
			var name, version sql.NullString
			var kind string
			if rs.Scan(&si.line.SlotID, &si.line.State, &kind, &season, &episode, &name, &si.titleID, &si.title, &si.year, &si.kind, &version, &si.album, &si.albumYear) != nil {
				continue
			}
			si.line.Season, si.line.Episode, si.line.Name = int(season.Int64), int(episode.Int64), name.String
			labelLine(&si.line, kind)
			si.version, si.full = version.String, version.Valid
			out[si.line.SlotID] = si
		}
	})
	return out
}

// linesOf is the episode lines of a download's Slots, in order.
func linesOf(ids []int64, slots map[int64]slotInfo) []EpisodeLine {
	out := []EpisodeLine{}
	for _, id := range ids {
		if si, ok := slots[id]; ok {
			out = append(out, si.line)
		}
	}
	sortLines(out)
	return out
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

func (a *API) removeDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	st, err := a.readDownloaderSettings(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	var in struct {
		DeleteFiles *bool `json:"delete_files"`
		TurnDown    *bool `json:"turn_down"`
		SearchAgain *bool `json:"search_again"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"delete_files": bool, "turn_down": bool, "search_again": bool}, each optional (defaults in Settings > Downloaders)`) {
		return
	}
	c := downloader.RemoveChoice{DeleteFiles: st.RemoveDeleteFiles, TurnDown: st.RemoveTurnDown, SearchAgain: st.RemoveSearchAgain}
	if in.DeleteFiles != nil {
		c.DeleteFiles = *in.DeleteFiles
	}
	if in.TurnDown != nil {
		c.TurnDown = *in.TurnDown
	}
	if in.SearchAgain != nil {
		c.SearchAgain = *in.SearchAgain
	}
	who := ""
	if p, ok := auth.From(r.Context()); ok {
		who = p.Name
	}
	err = a.manager().Remove(r.Context(), id, c, actor(r), who)
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no such download")
	case errors.Is(err, downloader.ErrNotActive):
		fail(w, http.StatusConflict, err.Error())
	case err != nil:
		internal(w, err)
	default:
		var d domain.Download
		_ = store.Get(r.Context(), a.s.Store.Read(), "downloads", id, &d)
		if d.ExternalID != "" {
			a.forgetTorrents(r.Context(), d.DownloaderID, []string{d.ExternalID})
		}
		v := downloadView(d)
		v.Label = PhaseLabels["removed"]
		if d.StateReason != nil {
			v.Text = upperFirst(*d.StateReason) + "."
		}
		writeJSON(w, http.StatusOK, v)
	}
}

// DecisionView is one movie waiting for a person on Needs a decision.
type DecisionView struct {
	SlotID      int64     `json:"slot_id"`
	TitleID     int64     `json:"title_id"`
	Title       string    `json:"title"`
	Year        *int      `json:"year,omitempty"`
	Version     string    `json:"version"`
	Versions    int       `json:"versions"` // how many Versions the Title has
	Why         string    `json:"why"`      // ran_out, conflict, not_replaced
	Reason      string    `json:"reason"`
	ReleaseID   *int64    `json:"release_id,omitempty"`
	ReleaseName string    `json:"release_name,omitempty"`
	HasCopy     bool      `json:"has_copy"`
	Since       time.Time `json:"since"`
	// Shows: the episode, and the pack it came in (the other episodes of
	// the same download that also wait for a decision).
	// Artists: Episode is the track, Pack every track of the album
	// download, and Album names the album.
	Kind        string        `json:"kind"` // movie, show, artist
	Episode     *EpisodeLine  `json:"episode,omitempty"`
	Album       string        `json:"album,omitempty"`
	DownloadID  *int64        `json:"download_id,omitempty"`
	PackSlotIDs []int64       `json:"pack_slot_ids,omitempty"`
	Pack        []EpisodeLine `json:"pack,omitempty"` // every episode of that download
}

// why sorts a Slot's last reason into the three ways it ends up waiting.
func why(reason string) string {
	switch {
	case strings.Contains(reason, "not replaced (setting)"):
		return "not_replaced"
	case strings.HasPrefix(reason, "not sent:"):
		// No Downloader takes the title's tags (#108).
		return "not_sent"
	case strings.HasPrefix(reason, "nothing could be placed"), strings.Contains(reason, "import"),
		strings.Contains(reason, "could not be placed"), strings.HasPrefix(reason, "nothing in the download could be matched"),
		// The unpack reasons (importer/unpack.go): the import could not finish, Releases did not run out.
		strings.Contains(reason, "RAR file"), strings.Contains(reason, "archive file"), strings.HasPrefix(reason, "Packed in "), strings.HasPrefix(reason, "Not enough free space to unpack"):
		return "conflict"
	}
	return "ran_out"
}

func (a *API) decisions(ctx context.Context, slotID int64) ([]DecisionView, error) {
	q := `SELECT s.id, t.id, t.name, t.year, v.label, (SELECT count(*) FROM versions v2 WHERE v2.title_id = t.id),
		COALESCE(s.last_reason, ''), s.file_id IS NOT NULL, s.updated_at,
		(SELECT d.release_id FROM downloads d WHERE EXISTS (SELECT 1 FROM json_each(d.slot_ids) WHERE value = s.id) ORDER BY d.id DESC LIMIT 1),
		t.kind, (SELECT d.id FROM downloads d WHERE EXISTS (SELECT 1 FROM json_each(d.slot_ids) WHERE value = s.id) ORDER BY d.id DESC LIMIT 1)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id JOIN versions v ON v.id = s.version_id
		WHERE s.state = 'needs_a_decision' AND t.state != 'removed'`
	args := []any{}
	if slotID > 0 {
		q += " AND s.id = ?"
		args = append(args, slotID)
	}
	rows, err := a.s.Store.Read().QueryContext(ctx, q+" ORDER BY s.updated_at DESC, s.id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DecisionView{}
	for rows.Next() {
		var dv DecisionView
		var year, rel, dl sql.NullInt64
		var since string
		if err := rows.Scan(&dv.SlotID, &dv.TitleID, &dv.Title, &year, &dv.Version, &dv.Versions, &dv.Reason, &dv.HasCopy, &since, &rel, &dv.Kind, &dl); err != nil {
			return nil, err
		}
		if dl.Valid {
			dv.DownloadID = &dl.Int64
		}
		if year.Valid {
			y := int(year.Int64)
			dv.Year = &y
		}
		if rel.Valid {
			dv.ReleaseID = &rel.Int64
		}
		dv.Since, _ = time.Parse(store.TimeLayout, since)
		dv.Why = why(dv.Reason)
		out = append(out, dv)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Kind == "artist" {
			_ = a.s.Store.Read().QueryRowContext(ctx, `SELECT p.name FROM slots s JOIN items i ON i.id = s.item_id JOIN parts p ON p.id = i.part_id
				WHERE s.id = ?`, out[i].SlotID).Scan(&out[i].Album)
		}
		if out[i].Kind == "show" || out[i].Kind == "artist" {
			if eps := episodeLines(ctx, a.s.Store.Read(), []int64{out[i].SlotID}); len(eps) == 1 {
				out[i].Episode = &eps[0]
			}
			if out[i].DownloadID != nil {
				var d domain.Download
				if store.Get(ctx, a.s.Store.Read(), "downloads", *out[i].DownloadID, &d) == nil && len(d.SlotIDs) > 1 {
					out[i].Pack = episodeLines(ctx, a.s.Store.Read(), d.SlotIDs)
					for _, e := range out[i].Pack {
						if e.State == string(domain.SlotNeedsADecision) {
							out[i].PackSlotIDs = append(out[i].PackSlotIDs, e.SlotID)
						}
					}
				}
			}
		}
		if out[i].ReleaseID != nil {
			_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *out[i].ReleaseID).Scan(&out[i].ReleaseName)
		}
		if out[i].Reason != "" {
			out[i].Reason = upperFirst(out[i].Reason) + "."
		}
	}
	return out, nil
}

func (a *API) needsDecision(w http.ResponseWriter, r *http.Request) {
	list, err := a.decisions(r.Context(), 0)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "count": len(list)})
}

// decide runs a person's choice for a Slot that needs a decision:
// search (search again by hand), turn_down (turn its last Release down,
// then search again) or stop (stop looking: Keep up to date off for that
// Version, the Slot rests). Choosing a Release by hand and trying another
// Version happen on the Movie page.
// errMovedOn is a Slot that left Needs a decision while a choice was made.
var errMovedOn = errors.New("the slot no longer needs a decision")

// beforeDecide lets tests change a Slot between the read and the write.
var beforeDecide func()

func (a *API) decide(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "slot")
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
		Pack   bool   `json:"pack"`
	}
	if !decode(w, r, &in, `body is {"action": "search" | "turn_down" | "stop", "pack": bool (optional, for every episode of the same download)}`) {
		return
	}
	list, err := a.decisions(r.Context(), id)
	if err != nil {
		internal(w, err)
		return
	}
	if len(list) == 0 {
		fail(w, http.StatusNotFound, "this movie or episode does not need a decision")
		return
	}
	dv := list[0]
	noun := "movie"
	many := dv.Kind == "show" || dv.Kind == "artist" // one Slot of many in a download
	switch dv.Kind {
	case "show":
		noun = "episode"
	case "artist":
		noun = "track"
	}
	targets := []DecisionView{dv}
	if in.Pack {
		if !many || len(dv.PackSlotIDs) == 0 {
			fail(w, http.StatusBadRequest, "this "+noun+" did not come in a pack")
			return
		}
		targets = nil
		for _, sid := range dv.PackSlotIDs {
			if l, err := a.decisions(r.Context(), sid); err == nil && len(l) == 1 {
				targets = append(targets, l[0])
			}
		}
	}
	if beforeDecide != nil {
		beforeDecide()
	}
	by := actor(r)
	who := "someone"
	if p, ok := auth.From(r.Context()); ok {
		who = p.Name
	}
	switch in.Action {
	case "turn_down":
		if dv.ReleaseID == nil {
			fail(w, http.StatusConflict, "there is no Release to turn down for this "+noun)
			return
		}
		reason := "turned down by " + who + " on Needs a decision"
		// An episode turns the Release down for that episode only; a pack
		// (or a movie) for everything it holds.
		if many && !in.Pack {
			err = a.searcher().TurnDownFor(r.Context(), *dv.ReleaseID, dv.SlotID, reason, by)
		} else {
			err = a.searcher().TurnDown(r.Context(), *dv.ReleaseID, reason, by)
		}
		if err != nil {
			searchError(w, err)
			return
		}
		fallthrough
	case "search":
		if a.s.Queue == nil {
			fail(w, http.StatusServiceUnavailable, "searches are not running")
			return
		}
		var jobs []int64
		for _, t := range targets {
			jid, _, err := a.s.Queue.Enqueue(r.Context(), source.KindSearchSlot, &domain.Subject{Type: "slot", ID: t.SlotID}, map[string]any{"by_hand": true},
				"hand_search_slot:"+itoa(t.SlotID), by)
			if err != nil {
				internal(w, err)
				return
			}
			jobs = append(jobs, jid)
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobs[0], "job_ids": jobs, "action": in.Action})
	case "stop":
		to := domain.SlotWanted
		if dv.HasCopy {
			to = domain.SlotBetterWanted
		}
		reason := "stopped looking (" + who + ")"
		var stopped []DecisionView
		err := a.s.Store.Write(r.Context(), func(tx *sql.Tx) error {
			stopped = nil
			for _, t := range targets {
				// The Slot may have moved on since it was read: only one that
				// still needs a decision stops, and nothing is written otherwise.
				var state string
				if err := tx.QueryRow("SELECT state FROM slots WHERE id = ?", t.SlotID).Scan(&state); err != nil {
					return err
				}
				if state != string(domain.SlotNeedsADecision) {
					if t.SlotID == id {
						return errMovedOn
					}
					continue
				}
				// A movie stops for its Version; an episode for itself, so
				// the rest of the show keeps going.
				q := "UPDATE versions SET keep_up_to_date = 0, updated_at = ? WHERE id = (SELECT version_id FROM slots WHERE id = ?)"
				if t.Kind == "show" || t.Kind == "artist" {
					q = "UPDATE items SET keep_up_to_date = 0, updated_at = ? WHERE id = (SELECT item_id FROM slots WHERE id = ?)"
				}
				if _, err := tx.Exec(q, store.FormatTime(time.Now()), t.SlotID); err != nil {
					return err
				}
				st := domain.SlotWanted
				if t.HasCopy {
					st = domain.SlotBetterWanted
				}
				if err := store.MoveSlotTx(tx, t.SlotID, st, reason, by); err != nil {
					return err
				}
				stopped = append(stopped, t)
			}
			return nil
		})
		if errors.Is(err, errMovedOn) {
			fail(w, http.StatusConflict, "this "+noun+" no longer needs a decision")
			return
		}
		if err != nil {
			internal(w, err)
			return
		}
		for _, t := range stopped {
			tid, sid := t.TitleID, t.SlotID
			name := t.Title + " (" + t.Version + ")"
			if t.Episode != nil {
				name = t.Title + " " + t.Episode.Label
			}
			e := &domain.Event{Actor: by, Kind: "stopped_looking", Summary: "Stopped looking for " + name, TitleID: &tid, SlotID: &sid}
			if a.s.Store.AddEvent(r.Context(), e) == nil && a.s.Live != nil {
				a.s.Live.Publish("event", e)
			}
			st := domain.SlotWanted
			if t.HasCopy {
				st = domain.SlotBetterWanted
			}
			if a.s.Live != nil {
				a.s.Live.Publish("slot", map[string]any{"id": sid, "state": st, "reason": reason})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"action": "stop", "state": to, "stopped": len(stopped)})
	default:
		fail(w, http.StatusBadRequest, `action is "search", "turn_down" or "stop"`)
	}
}
