package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ErrFilePriority means a file priority is not 0, 1, 6 or 7.
var ErrFilePriority = errors.New("file priority is 0 (skip), 1 (normal), 6 (high) or 7 (maximum)")

// ErrNotSupported means the Downloader has no such control.
var ErrNotSupported = errors.New("this Downloader does not offer that control")

// ErrNoDownloader means the Downloader id is unknown or removed.
var ErrNoDownloader = errors.New("no such Downloader")

var qbitTrackerStatus = map[int]string{0: "disabled", 1: "not_contacted", 2: "working", 3: "working", 4: "not_working", 5: "error", 6: "unreachable"}

// urlish matches address-shaped words in free text. Tracker messages can echo
// the announce address with its passkey.
var urlish = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// secretish matches passkey-shaped leftovers once addresses are gone: key=value
// or key: value pairs that trackers use for a member key, and long hex path
// parts.
var (
	secretPair = regexp.MustCompile(`(?i)\b(passkey|authkey|torrent_pass|api_?key|apikey|token|pid|uid|key)(\s*[=:]\s*)[^\s&;,"'<>]+`)
	secretPath = regexp.MustCompile(`/[0-9a-fA-F]{24,}`)
)

// ClientStatus is what a Downloader reports about itself as a whole. Sizes are
// bytes, speeds and limits are bytes per second; 0 means none or unknown.
type ClientStatus struct {
	Paused      bool  `json:"paused"`
	Speed       int64 `json:"speed"`
	UpSpeed     int64 `json:"up_speed"`
	SpeedLimit  int64 `json:"speed_limit"`
	UpLimit     int64 `json:"up_limit"`
	SessionDown int64 `json:"session_down"`
	SessionUp   int64 `json:"session_up"`
	AllTimeDown int64 `json:"all_time_down"`
	AllTimeUp   int64 `json:"all_time_up"`
	DayDown     int64 `json:"day_down"`
	MonthDown   int64 `json:"month_down"`
	FreeSpace   int64 `json:"free_space"`
	// Torrent clients only: connected, firewalled or disconnected ("" when not
	// reported), and whether the slower alternative speed limits are on.
	Connection string `json:"connection,omitempty"`
	SlowMode   *bool  `json:"slow_mode,omitempty"`
	// TimeLeft is a Usenet queue's own estimate in seconds (0 for none).
	TimeLeft int64 `json:"time_left,omitempty"`
}

// Tracker is one tracker of a torrent.
type Tracker struct {
	URL     string `json:"url"`
	Tier    int    `json:"tier"`
	Status  string `json:"status"` // working, not_contacted, not_working, error, unreachable, disabled
	Message string `json:"message"`
	Seeds   int    `json:"seeds"`
	Peers   int    `json:"peers"`
	// NextCheck is seconds until the next announce, 0 when unknown.
	NextCheck int64 `json:"next_check"`
	// Fixed marks the client's own peer sources (DHT, PeX, LSD). They are not
	// trackers and cannot be edited. Ref names a real tracker for edit and remove
	// without exposing its full address, which can hold a passkey.
	Fixed bool   `json:"fixed,omitempty"`
	Ref   string `json:"ref,omitempty"`
}

// Peer is one connected peer of a torrent.
type Peer struct {
	Address  string  `json:"address"`
	Client   string  `json:"client"`
	Progress float64 `json:"progress"`
	Speed    int64   `json:"speed"`
	UpSpeed  int64   `json:"up_speed"`
	Country  string  `json:"country"`
}

// Controller is the optional part of an engine that steers downloads. An engine
// without it still works; the controls are just not offered.
type Controller interface {
	PauseItem(ctx context.Context, id string) error
	ResumeItem(ctx context.Context, id string) error
	PauseAll(ctx context.Context) error
	ResumeAll(ctx context.Context) error
	MoveTop(ctx context.Context, id string) error
	// SetSpeedLimit caps the total speed; 0 removes the cap.
	SetSpeedLimit(ctx context.Context, down, up int64) error
	Status(ctx context.Context) (ClientStatus, error)
}

// TorrentController adds the controls only torrents have.
type TorrentController interface {
	Recheck(ctx context.Context, id string) error
	Reannounce(ctx context.Context, id string) error
	Trackers(ctx context.Context, id string) ([]Tracker, error)
	Peers(ctx context.Context, id string) ([]Peer, error)
}

// TorrentDetail is what a torrent client reports about one torrent. Sizes are
// bytes, speeds bytes per second, ETA seconds (0 unknown).
type TorrentDetail struct {
	Size       int64      `json:"size"`
	Downloaded int64      `json:"downloaded"`
	Uploaded   int64      `json:"uploaded"`
	Wasted     int64      `json:"wasted"`
	Speed      int64      `json:"speed"`
	UpSpeed    int64      `json:"up_speed"`
	ETA        int64      `json:"eta"`
	Ratio      float64    `json:"ratio"`
	Seeds      int        `json:"seeds"`
	SeedsTotal int        `json:"seeds_total"`
	Peers      int        `json:"peers"`
	PeersTotal int        `json:"peers_total"`
	AddedAt    *time.Time `json:"added_at,omitempty"`
	DoneAt     *time.Time `json:"done_at,omitempty"`
	SavePath   string     `json:"save_path"`
	Private    *bool      `json:"private,omitempty"`
	// SeedingTime and TimeActive are seconds (0 unknown). LastSeen is when a full
	// copy was last seen; LastActivity is when a piece last moved.
	SeedingTime  int64      `json:"seeding_time,omitempty"`
	TimeActive   int64      `json:"time_active,omitempty"`
	LastSeen     *time.Time `json:"last_seen,omitempty"`
	LastActivity *time.Time `json:"last_activity,omitempty"`
}

// TorrentFile is one file inside a torrent. Priority is 0 skip, 1 normal,
// 6 high, 7 maximum.
type TorrentFile struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

// TorrentInspector reads one torrent's detail and files, and sets which files
// to fetch.
type TorrentInspector interface {
	Properties(ctx context.Context, id string) (TorrentDetail, error)
	TorrentFiles(ctx context.Context, id string) ([]TorrentFile, error)
	SetFilePriority(ctx context.Context, id string, indexes []int, priority int) error
}

// Retrier retries a failed Usenet job; the job id may change.
type Retrier interface {
	Retry(ctx context.Context, id string) (string, error)
}

// ControlEngine returns a Downloader's engine without the guard, keeping the
// guard's bookkeeping around the call.
func (m *Manager) ControlEngine(ctx context.Context, d domain.Downloader, call func(Engine) error) error {
	e, err := m.engine(ctx, d)
	if err != nil {
		return err
	}
	if g, ok := e.(*guardedEngine); ok {
		return g.run(ctx, func() error { return call(g.Engine) })
	}
	return call(e)
}

// Control runs an action on the Downloader holding a Download, using the id the
// Downloader knows it by. A sync follows so the new state shows.
func (m *Manager) Control(ctx context.Context, downloadID int64, call func(e Engine, id string) error) error {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !isActive(d.State) {
		return ErrNotActive
	}
	if d.ExternalID == "" {
		return ErrNotFound
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	err := m.ControlEngine(ctx, dr, func(e Engine) error { return call(e, d.ExternalID) })
	if err == nil {
		m.Poke()
	}
	return err
}

// Inspect is Control for reads only. It also reaches a finished or seeding
// torrent the client still holds (any state but cleaned up or gone).
func (m *Manager) Inspect(ctx context.Context, downloadID int64, call func(e Engine, id string) error) error {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !Inspectable(d.State) {
		return ErrNotActive
	}
	if d.ExternalID == "" {
		return ErrNotFound
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	return m.ControlEngine(ctx, dr, func(e Engine) error { return call(e, d.ExternalID) })
}

// Inspectable reports whether the client may still hold a Download in this state.
func Inspectable(state string) bool {
	return state != "cleaned" && state != "vanished" && state != ""
}

// ControlItem runs a call on one item of a Downloader by the client's own id
// (a torrent's info hash), whether or not HolyArrs sent it. It never touches
// download records. When write is true a sync follows so the new state shows.
func (m *Manager) ControlItem(ctx context.Context, downloaderID int64, hash string, write bool, call func(e Engine, id string) error) error {
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", downloaderID, &dr); err != nil || dr.Removed {
		return ErrNoDownloader
	}
	var h string
	var err error
	switch dr.Protocol {
	case "torrent":
		if h, err = qbitHash(hash); err != nil {
			return ErrNotFound
		}
	case "usenet":
		// A Usenet job id goes out exactly as the client wrote it.
		if h, err = UsenetID(hash); err != nil {
			return ErrNotFound
		}
	default:
		return ErrNotSupported
	}
	err = m.ControlEngine(ctx, dr, func(e Engine) error { return call(e, h) })
	if err == nil && write {
		m.Poke()
	}
	return err
}

// UsenetID checks a Usenet job id from a request: letters, digits and _ . -
// only, at most 100 characters. SABnzbd ids look like SABnzbd_nzo_... or a UUID.
func UsenetID(id string) (string, error) {
	if id == "" || len(id) > 100 {
		return "", ErrNotFound
	}
	for _, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.'
		if !ok {
			return "", ErrNotFound
		}
	}
	return id, nil
}

// ControlTorrentClient runs a call on a torrent Downloader as a whole, such as
// an add or one action on many items. Like ControlItem it never touches download
// records. When write is true a sync follows.
func (m *Manager) ControlTorrentClient(ctx context.Context, downloaderID int64, write bool, call func(e Engine) error) error {
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", downloaderID, &dr); err != nil || dr.Removed {
		return ErrNoDownloader
	}
	if dr.Protocol != "torrent" {
		return ErrNotSupported
	}
	err := m.ControlEngine(ctx, dr, call)
	if err == nil && write {
		m.Poke()
	}
	return err
}

// Downloaders lists the Downloaders in use, in priority order.
func (m *Manager) Downloaders(ctx context.Context) ([]domain.Downloader, error) {
	ds, err := database.Select[domain.Downloader](ctx, m.Store.Read(), "downloaders", "removed = 0 ORDER BY priority, name")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ds, err
}

// PauseItem stops one torrent.
func (q *Qbittorrent) PauseItem(ctx context.Context, id string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	return q.qbitStartStop(ctx, h, false)
}

// ResumeItem starts one torrent again.
func (q *Qbittorrent) ResumeItem(ctx context.Context, id string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	return q.qbitStartStop(ctx, h, true)
}

// PauseAll stops every torrent.
func (q *Qbittorrent) PauseAll(ctx context.Context) error { return q.qbitStartStop(ctx, "all", false) }

// ResumeAll starts every torrent.
func (q *Qbittorrent) ResumeAll(ctx context.Context) error { return q.qbitStartStop(ctx, "all", true) }

// MoveTop puts a torrent first in the queue. qBittorrent answers 409 while its
// queue is off.
func (q *Qbittorrent) MoveTop(ctx context.Context, id string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/topPrio", url.Values{"hashes": {h}})
	if err != nil && strings.Contains(err.Error(), "409") {
		return errors.New("qbittorrent has its queue turned off, so there is no order to change")
	}
	return err
}

// Recheck checks the torrent's data on disk again.
func (q *Qbittorrent) Recheck(ctx context.Context, id string) error {
	return q.onOne(ctx, "torrents/recheck", id)
}

// Reannounce asks the trackers for peers now.
func (q *Qbittorrent) Reannounce(ctx context.Context, id string) error {
	return q.onOne(ctx, "torrents/reannounce", id)
}

// SetSpeedLimit sets the global download and upload caps.
func (q *Qbittorrent) SetSpeedLimit(ctx context.Context, down, up int64) error {
	if _, err := q.call(ctx, http.MethodPost, "transfer/setDownloadLimit", url.Values{"limit": {strconv.FormatInt(max(down, 0), 10)}}); err != nil {
		return err
	}
	_, err := q.call(ctx, http.MethodPost, "transfer/setUploadLimit", url.Values{"limit": {strconv.FormatInt(max(up, 0), 10)}})
	return err
}

// Status reads the server state from a full delta answer, the only place free
// space and all-time totals appear.
func (q *Qbittorrent) Status(ctx context.Context) (ClientStatus, error) {
	b, err := q.call(ctx, http.MethodGet, "sync/maindata", url.Values{"rid": {"0"}})
	if err != nil {
		return ClientStatus{}, err
	}
	var md struct {
		Torrents map[string]struct{ State string } `json:"torrents"`
		Server   json.RawMessage                   `json:"server_state"`
	}
	if err := json.Unmarshal(b, &md); err != nil {
		return ClientStatus{}, fmt.Errorf("qbittorrent sync/maindata: %w", err)
	}
	states := make([]string, 0, len(md.Torrents))
	for _, t := range md.Torrents {
		states = append(states, t.State)
	}
	return qbitStatus(md.Server, states), nil
}

// StatusFromMain is Status built from the background reader's merged change
// feed, so the page needs no full read of its own.
func StatusFromMain(s *MainState) ClientStatus {
	server, _ := json.Marshal(s.Server)
	states := make([]string, 0, len(s.fields))
	for _, f := range s.fields {
		var st string
		_ = json.Unmarshal(f["state"], &st)
		states = append(states, st)
	}
	return qbitStatus(server, states)
}

// Trackers lists a torrent's trackers, then three fixed rows for DHT, peer
// exchange and local discovery.
func (q *Qbittorrent) Trackers(ctx context.Context, id string) ([]Tracker, error) {
	h, err := qbitHash(id)
	if err != nil {
		return nil, err
	}
	b, err := q.call(ctx, http.MethodGet, "torrents/trackers", url.Values{"hash": {h}})
	if err != nil {
		return nil, err
	}
	var rows []struct {
		URL    string `json:"url"`
		Tier   int    `json:"tier"`
		Status int    `json:"status"`
		Msg    string `json:"msg"`
		Seeds  int    `json:"num_seeds"`
		Peers  int    `json:"num_peers"`
		Next   int64  `json:"next_announce"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("qbittorrent torrents/trackers: %w", err)
	}
	out, fixed := []Tracker{}, []Tracker{}
	for _, r := range rows {
		if r.Tier < 0 {
			name := fixedSource(r.URL)
			if name == "" {
				continue
			}
			// Status 0 on a fixed row means the source is off; otherwise it is running.
			st := "working"
			if r.Status == 0 {
				st = "disabled"
			}
			fixed = append(fixed, Tracker{URL: name, Tier: -1, Status: st, Message: redactURLs(r.Msg), Seeds: max(r.Seeds, 0), Peers: max(r.Peers, 0), Fixed: true})
			continue
		}
		out = append(out, Tracker{URL: trackerHost(r.URL), Tier: r.Tier, Status: qbitTrackerStatus[r.Status], Message: redactURLs(r.Msg), Seeds: max(r.Seeds, 0), Peers: max(r.Peers, 0), NextCheck: max(r.Next, 0), Ref: trackerRef(r.URL)})
	}
	return append(out, fixed...), nil
}

// Peers lists a torrent's connected peers.
func (q *Qbittorrent) Peers(ctx context.Context, id string) ([]Peer, error) {
	h, err := qbitHash(id)
	if err != nil {
		return nil, err
	}
	b, err := q.call(ctx, http.MethodGet, "sync/torrentPeers", url.Values{"hash": {h}, "rid": {"0"}})
	if err != nil {
		return nil, err
	}
	var tp struct {
		Peers map[string]struct {
			IP       string  `json:"ip"`
			Port     int     `json:"port"`
			Client   string  `json:"client"`
			Progress float64 `json:"progress"`
			DLSpeed  int64   `json:"dl_speed"`
			UPSpeed  int64   `json:"up_speed"`
			Country  string  `json:"country"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(b, &tp); err != nil {
		return nil, fmt.Errorf("qbittorrent sync/torrentPeers: %w", err)
	}
	out := []Peer{}
	for key, p := range tp.Peers {
		addr := key
		if p.IP != "" {
			addr = p.IP + ":" + strconv.Itoa(p.Port)
		}
		out = append(out, Peer{Address: addr, Client: p.Client, Progress: p.Progress, Speed: p.DLSpeed, UpSpeed: p.UPSpeed, Country: p.Country})
	}
	// Sorted so the list does not reshuffle between reads.
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

// Properties reads one torrent's detail.
func (q *Qbittorrent) Properties(ctx context.Context, id string) (TorrentDetail, error) {
	h, err := qbitHash(id)
	if err != nil {
		return TorrentDetail{}, err
	}
	b, err := q.call(ctx, http.MethodGet, "torrents/properties", url.Values{"hash": {h}})
	if err != nil {
		return TorrentDetail{}, err
	}
	var p struct {
		Size       int64   `json:"total_size"`
		Downloaded int64   `json:"total_downloaded"`
		Uploaded   int64   `json:"total_uploaded"`
		Wasted     int64   `json:"total_wasted"`
		DLSpeed    int64   `json:"dl_speed"`
		UPSpeed    int64   `json:"up_speed"`
		ETA        int64   `json:"eta"`
		Ratio      float64 `json:"share_ratio"`
		Seeds      int     `json:"seeds"`
		SeedsTotal int     `json:"seeds_total"`
		Peers      int     `json:"peers"`
		PeersTotal int     `json:"peers_total"`
		Added      int64   `json:"addition_date"`
		Done       int64   `json:"completion_date"`
		SavePath   string  `json:"save_path"`
		Private    *bool   `json:"is_private"`
		SeedTime   int64   `json:"seeding_time"`
		Elapsed    int64   `json:"time_elapsed"`
		LastSeen   int64   `json:"last_seen"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return TorrentDetail{}, fmt.Errorf("qbittorrent torrents/properties: %w", err)
	}
	when := func(s int64) *time.Time {
		if s <= 0 {
			return nil
		}
		t := time.Unix(s, 0).UTC()
		return &t
	}
	eta := p.ETA
	if eta >= 8640000 || eta < 0 {
		eta = 0
	}
	return TorrentDetail{Size: max(p.Size, 0), Downloaded: max(p.Downloaded, 0), Uploaded: max(p.Uploaded, 0), Wasted: max(p.Wasted, 0),
		Speed: max(p.DLSpeed, 0), UpSpeed: max(p.UPSpeed, 0), ETA: eta, Ratio: max(p.Ratio, 0),
		Seeds: max(p.Seeds, 0), SeedsTotal: max(p.SeedsTotal, 0), Peers: max(p.Peers, 0), PeersTotal: max(p.PeersTotal, 0),
		AddedAt: when(p.Added), DoneAt: when(p.Done), SavePath: p.SavePath, Private: p.Private,
		SeedingTime: max(p.SeedTime, 0), TimeActive: max(p.Elapsed, 0), LastSeen: when(p.LastSeen)}, nil
}

// TorrentFiles lists a torrent's files with their priority; empty until the
// metadata arrives.
func (q *Qbittorrent) TorrentFiles(ctx context.Context, id string) ([]TorrentFile, error) {
	h, err := qbitHash(id)
	if err != nil {
		return nil, err
	}
	b, err := q.call(ctx, http.MethodGet, "torrents/files", url.Values{"hash": {h}})
	if err != nil {
		return nil, err
	}
	out := []TorrentFile{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("qbittorrent torrents/files: %w", err)
	}
	return out, nil
}

// SetFilePriority sets the priority of files by index.
func (q *Qbittorrent) SetFilePriority(ctx context.Context, id string, indexes []int, priority int) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	switch priority {
	case 0, 1, 6, 7:
	default:
		return ErrFilePriority
	}
	if len(indexes) == 0 {
		return nil
	}
	ids := make([]string, len(indexes))
	for i, n := range indexes {
		if n < 0 {
			return ErrNotFound
		}
		ids[i] = strconv.Itoa(n)
	}
	_, err = q.call(ctx, http.MethodPost, "torrents/filePrio", url.Values{"hash": {h}, "id": {strings.Join(ids, "|")}, "priority": {strconv.Itoa(priority)}})
	return err
}

// PauseItem pauses one job.
func (s *Sabnzbd) PauseItem(ctx context.Context, id string) error {
	return s.queueAct(ctx, "pause", id)
}

// ResumeItem resumes one job.
func (s *Sabnzbd) ResumeItem(ctx context.Context, id string) error {
	return s.queueAct(ctx, "resume", id)
}

// PauseAll pauses the whole queue.
func (s *Sabnzbd) PauseAll(ctx context.Context) error {
	_, err := s.call(ctx, "pause", nil, true)
	return err
}

// ResumeAll resumes the whole queue.
func (s *Sabnzbd) ResumeAll(ctx context.Context) error {
	_, err := s.call(ctx, "resume", nil, true)
	return err
}

// MoveTop moves a job to the first place.
func (s *Sabnzbd) MoveTop(ctx context.Context, id string) error {
	if id == "" || strings.Contains(id, ",") {
		return ErrNotFound
	}
	b, err := s.call(ctx, "switch", url.Values{"value": {id}, "value2": {"0"}}, true)
	if err != nil {
		return err
	}
	var r struct {
		Result struct {
			Position *int `json:"position"`
		} `json:"result"`
	}
	if json.Unmarshal(b, &r) == nil && r.Result.Position != nil && *r.Result.Position < 0 {
		// Do not move: SABnzbd gives the same answer for a job already first (or the
		// only one) as for a job it does not have.
		qb, err := s.call(ctx, "queue", url.Values{"nzo_ids": {id}, "start": {"0"}, "limit": {"1"}}, true)
		if err != nil {
			return err
		}
		var qu sabQueueFull
		if json.Unmarshal(qb, &qu) == nil {
			for _, x := range qu.Queue.Slots {
				if x.ID == id {
					return nil
				}
			}
		}
		return ErrNotFound
	}
	return nil
}

// SetSpeedLimit caps the download speed. The value carries a K so SABnzbd reads
// it as a rate and not a percent. Usenet has no upload.
func (s *Sabnzbd) SetSpeedLimit(ctx context.Context, down, _ int64) error {
	v := "0"
	if down > 0 {
		v = strconv.FormatInt(max(down/1024, 1), 10) + "K"
	}
	_, err := s.call(ctx, "config", url.Values{"name": {"speedlimit"}, "value": {v}}, true)
	return err
}

// Status reads the queue header and the byte counters.
func (s *Sabnzbd) Status(ctx context.Context) (ClientStatus, error) {
	b, err := s.call(ctx, "queue", url.Values{"start": {"0"}, "limit": {"1"}}, true)
	if err != nil {
		return ClientStatus{}, err
	}
	var qu struct {
		Queue struct {
			Paused     bool   `json:"paused"`
			KBPerSec   string `json:"kbpersec"`
			Limit      string `json:"speedlimit_abs"`
			TimeLeft   string `json:"timeleft"`
			DiskSpace1 string `json:"diskspace1"`
			DiskSpace2 string `json:"diskspace2"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(b, &qu); err != nil {
		return ClientStatus{}, fmt.Errorf("sabnzbd queue: %w", err)
	}
	gib := func(s string) int64 { return int64(num(s) * 1024 * 1024 * 1024) }
	free := gib(qu.Queue.DiskSpace1)
	if f2 := gib(qu.Queue.DiskSpace2); f2 > 0 && (free == 0 || f2 < free) {
		free = f2
	}
	st := ClientStatus{Paused: qu.Queue.Paused, Speed: int64(num(qu.Queue.KBPerSec) * 1024), SpeedLimit: int64(num(qu.Queue.Limit)), FreeSpace: free, TimeLeft: clock(qu.Queue.TimeLeft)}
	b, err = s.call(ctx, "server_stats", nil, true)
	if err != nil {
		return st, nil // The counters are extra; the header is enough.
	}
	var ss struct {
		Total int64 `json:"total"`
		Month int64 `json:"month"`
		Day   int64 `json:"day"`
	}
	if json.Unmarshal(b, &ss) == nil {
		st.AllTimeDown, st.MonthDown, st.DayDown = ss.Total, ss.Month, ss.Day
	}
	return st, nil
}

// Retry sends a failed job back to the queue.
func (s *Sabnzbd) Retry(ctx context.Context, id string) (string, error) {
	if id == "" || strings.Contains(id, ",") {
		return "", ErrNotFound
	}
	b, err := s.call(ctx, "retry", url.Values{"value": {id}}, true)
	if err != nil {
		return "", err
	}
	var r struct {
		ID string `json:"nzo_id"`
	}
	if json.Unmarshal(b, &r) == nil && r.ID != "" {
		return r.ID, nil
	}
	return id, nil
}

// RedactURLs swaps every address in free text for its scheme and host, so a
// passkey never leaves the client's answer.
func RedactURLs(msg string) string { return redactURLs(msg) }

// qbitStartStop sends the 5.x action and falls back to the 4.x name.
func (q *Qbittorrent) qbitStartStop(ctx context.Context, hashes string, start bool) error {
	now, old := "torrents/stop", "torrents/pause"
	if start {
		now, old = "torrents/start", "torrents/resume"
	}
	_, err := q.call(ctx, http.MethodPost, now, url.Values{"hashes": {hashes}})
	if errors.Is(err, ErrNotFound) {
		_, err = q.call(ctx, http.MethodPost, old, url.Values{"hashes": {hashes}})
	}
	return err
}

func qbitHash(id string) (string, error) {
	id = strings.ToLower(id)
	if !releasename.ValidInfoHash(id) {
		return "", errBadHash
	}
	return id, nil
}

func (q *Qbittorrent) onOne(ctx context.Context, path, id string) error {
	h, err := qbitHash(id)
	if err != nil {
		return err
	}
	_, err = q.call(ctx, http.MethodPost, path, url.Values{"hashes": {h}})
	return err
}

func qbitStatus(server []byte, states []string) ClientStatus {
	var s struct {
		AllTimeDL int64  `json:"alltime_dl"`
		AllTimeUL int64  `json:"alltime_ul"`
		DLData    int64  `json:"dl_info_data"`
		ULData    int64  `json:"up_info_data"`
		DLSpeed   int64  `json:"dl_info_speed"`
		ULSpeed   int64  `json:"up_info_speed"`
		DLLimit   int64  `json:"dl_rate_limit"`
		ULLimit   int64  `json:"up_rate_limit"`
		Free      int64  `json:"free_space_on_disk"`
		Conn      string `json:"connection_status"`
		Alt       *bool  `json:"use_alt_speed_limits"`
	}
	_ = json.Unmarshal(server, &s)
	st := ClientStatus{Speed: s.DLSpeed, UpSpeed: s.ULSpeed, SpeedLimit: max(s.DLLimit, 0), UpLimit: max(s.ULLimit, 0),
		SessionDown: s.DLData, SessionUp: s.ULData, AllTimeDown: s.AllTimeDL, AllTimeUp: s.AllTimeUL, FreeSpace: s.Free, SlowMode: s.Alt}
	switch s.Conn {
	case "connected", "firewalled", "disconnected":
		st.Connection = s.Conn
	}
	// Paused as a whole when there are torrents and none of them is running.
	st.Paused = len(states) > 0
	for _, t := range states {
		if !strings.HasPrefix(t, "stopped") && !strings.HasPrefix(t, "paused") {
			st.Paused = false
			break
		}
	}
	return st
}

func (s *Sabnzbd) queueAct(ctx context.Context, name, id string) error {
	if id == "" || strings.Contains(id, ",") {
		return ErrNotFound
	}
	_, err := s.call(ctx, "queue", url.Values{"name": {name}, "value": {id}}, true)
	return err
}

// trackerHost keeps only the scheme and host of a tracker address, so passkeys
// in the path, query or user part never leave HolyArrs. Entries like
// "** [DHT] **" are not addresses and pass through.
func trackerHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "tracker"
	}
	return u.Scheme + "://" + u.Host
}

// redactURLs swaps every address in a message for its trackerHost and blanks
// passkey-shaped fragments written without a scheme.
func redactURLs(msg string) string {
	msg = urlish.ReplaceAllStringFunc(msg, trackerHost)
	msg = secretPair.ReplaceAllString(msg, "$1$2…")
	return secretPath.ReplaceAllString(msg, "/…")
}
