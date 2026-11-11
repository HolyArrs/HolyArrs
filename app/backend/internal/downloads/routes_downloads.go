package downloads

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Stage 4.1: the Downloads screen and Needs a decision.
func (a *Web) downloadsRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/downloads/view", a.DownloadsView)
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
	Limits *ShareLimits `json:"limits,omitempty"`
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
	SortLines(out)
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

// SortLines puts episode lines in season and episode order.
func SortLines(out []EpisodeLine) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Season != out[j].Season {
			return out[i].Season < out[j].Season
		}
		return out[i].Episode < out[j].Episode
	})
}

// Covers names the episodes in a short line: "S01E02 The Mill" for one,
// "S01E01-E03, 3 episodes" for a run in one season, else the first few.
func Covers(eps []EpisodeLine) string {
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
		switch mediafiles.SlotState(e.State) {
		case mediafiles.SlotInLibrary, mediafiles.SlotBetterWanted:
			in++
		case mediafiles.SlotNeedsADecision:
			decide = append(decide, e.Label)
		case mediafiles.SlotWanted, mediafiles.SlotRejected, mediafiles.SlotSearching, mediafiles.SlotChosen, mediafiles.SlotHeld, mediafiles.SlotFailed:
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
	switch mediafiles.SlotState(slot) {
	case mediafiles.SlotImporting:
		return "importing"
	case mediafiles.SlotNeedsADecision:
		return "needs_a_decision"
	case mediafiles.SlotRejected:
		return "import_refused"
	case mediafiles.SlotDownloading, mediafiles.SlotStalled, mediafiles.SlotChosen:
		return "waiting_to_import"
	}
	return "done"
}

func (a *Web) DownloadsView(w http.ResponseWriter, r *http.Request) {
	// People who may not manage the Library watch only the downloads for
	// titles they asked for, and act on none of them.
	// (No person on the request: the sign-in layer is off, as in tests.)
	who, ok := webserver.From(r.Context())
	manage := !ok || who.Has(webserver.PermManageLibrary)
	where, args := "1=1", []any{}
	// People who approve read the downloads of every approved ask on the
	// Requests page (?asked=1), as they read everyone's asks there;
	// the rows stay the plain ones members get.
	asked := r.URL.Query().Get("asked") == "1"
	if !manage && !(asked && ok && who.Has(webserver.PermApprove)) {
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
		revs = a.DLSync.Store.Revs()
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
	ds, err := database.Select[domain.Download](r.Context(), a.read(), "downloads", where+" ORDER BY id DESC LIMIT ?", append(args, n)...)
	if err != nil {
		webserver.Internal(w, "api", err)
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
		row := DownloadRow{DownloadView: DownloadViewOf(d), StartedAt: d.StartedAt, FinishedAt: d.FinishedAt, UpdatedAt: d.UpdatedAt, Name: d.ExternalID, Path: d.SavePath}
		if d.ExternalID != "" {
			shown[clientKey{d.DownloaderID, strings.ToLower(d.ExternalID)}] = true
			row.LiveKey = LiveKey(d.DownloaderID, d.ExternalID)
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
			row.Covers = Covers(row.Episodes)
			row.Phase = phaseOfMany(d, row.Episodes)
		}
		row.Label = PhaseLabels[row.Phase]
		row.CanAct = row.Active && manage
		row.ClientLocked = ReadOnlyFor(n)
		if (d.State == "failed" || d.State == "vanished") && row.SlotState != "" && row.SlotState != string(mediafiles.SlotFailed) && row.SlotState != string(mediafiles.SlotNeedsADecision) {
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
		if a.DLSync != nil {
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
	cursor := r.URL.Query().Get("cursor")
	page, next := pageOutside(outside, cursor, outsideLimit(r))
	rows = append(rows, page...)
	out := map[string]any{"downloads": rows, "totals": total, "clients": health, "outside_total": len(outside), "next_cursor": next,
		"outside_rest": outsideRest(outside, cursor, len(page))}
	if a.syncOn() {
		// Someone is watching: the reader reads faster (no write, no call).
		a.DLSync.Watched()
		out["revs"] = revs
		if a.Live != nil {
			out["boot"] = a.Live.Boot()
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// RestOf is the outside rows of one Downloader a view answer left out:
// its protocol (so a Torrent or Usenet filter counts only its own) and how
// many there are per stage.
type RestOf struct {
	Protocol string         `json:"protocol"`
	Stages   map[string]int `json:"stages"`
}

// outsideRest counts, per Downloader id and stage, the outside rows this
// answer leaves out (those before the cursor and after its page), so the
// tabs can count every torrent while the page counts its own rows live.
func outsideRest(outside []DownloadRow, cursor string, sent int) map[string]RestOf {
	from, err := strconv.Atoi(cursor)
	if err != nil || from < 0 {
		from = 0
	}
	rest := map[string]RestOf{}
	for i, row := range outside {
		if i >= from && i < from+sent {
			continue
		}
		k := strconv.FormatInt(row.DownloaderID, 10)
		e, ok := rest[k]
		if !ok {
			e = RestOf{Protocol: row.Protocol, Stages: map[string]int{}}
		}
		e.Stages[stageOf(row)]++
		rest[k] = e
	}
	return rest
}

// stageOf places a row on the stage the Downloads tabs count it under. It
// must match stageOf in frontend/src/lib/downloads/status-tabs.ts.
func stageOf(r DownloadRow) string {
	if r.ClientHint == "checking" || r.ClientHint == "moving" {
		return "checking"
	}
	switch r.Phase {
	case "waiting":
		return "queued"
	case "paused":
		return "paused"
	case "downloading":
		if r.Progress >= 1 {
			return "importing"
		}
		return "downloading"
	case "stalled":
		return "stalled"
	case "waiting_to_import", "importing":
		return "importing"
	case "needs_a_decision":
		return "decide"
	case "failed", "vanished":
		if r.Replaced {
			return "done"
		}
		return "failed"
	case "import_refused":
		return "failed"
	case "done":
		if r.State == "seeding" {
			return "seeding"
		}
		return "done"
	}
	return "done"
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

// OutsideCap is how many outside rows one view answer carries by default.
const OutsideCap = 200

// outsideMax is the most outside rows one view answer carries.
const outsideMax = 2000

// outsideLimit is the page size for outside rows (?outside_limit=, 1-2000;
// more is held at 2000, so Show more past it never drops back to the first 200).
func outsideLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("outside_limit"))
	if err != nil || n <= 0 {
		return OutsideCap
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
	SortLines(out)
	return out
}

func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}
