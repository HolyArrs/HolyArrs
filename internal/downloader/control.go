package downloader

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// ClientStatus is what a Downloader says about itself as a whole. Sizes
// are bytes, speeds and limits bytes per second; 0 means none or unknown.
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
}

// Tracker is one tracker of a torrent.
type Tracker struct {
	URL     string `json:"url"`
	Tier    int    `json:"tier"`
	Status  string `json:"status"` // working, not_contacted, not_working, error, unreachable, disabled
	Message string `json:"message"`
	Seeds   int    `json:"seeds"`
	Peers   int    `json:"peers"`
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

// Controller is the optional part of an engine that steers downloads. An
// engine without it keeps working; the controls just are not offered.
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

// TorrentController adds the torrent-only controls.
type TorrentController interface {
	Recheck(ctx context.Context, id string) error
	Reannounce(ctx context.Context, id string) error
	Trackers(ctx context.Context, id string) ([]Tracker, error)
	Peers(ctx context.Context, id string) ([]Peer, error)
}

// Retrier tries a failed Usenet job again; the job id may change.
type Retrier interface {
	Retry(ctx context.Context, id string) (string, error)
}

// ErrNotSupported means the Downloader has no such control.
var ErrNotSupported = errors.New("this Downloader does not offer that control")

// ControlEngine returns a Downloader's engine without the guard, with the
// guard's bookkeeping kept around the call.
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

// Control runs an action on the Downloader holding a Download, with the
// Download's id in that Downloader. A sync follows so the new state shows.
func (m *Manager) Control(ctx context.Context, downloadID int64, call func(e Engine, id string) error) error {
	var d domain.Download
	if err := store.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil {
		return err
	}
	if !isActive(d.State) {
		return ErrNotActive
	}
	if d.ExternalID == "" {
		return ErrNotFound
	}
	var dr domain.Downloader
	if err := store.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		return err
	}
	err := m.ControlEngine(ctx, dr, func(e Engine) error { return call(e, d.ExternalID) })
	if err == nil {
		m.Poke()
	}
	return err
}

// Downloaders lists the Downloaders in use, in priority order.
func (m *Manager) Downloaders(ctx context.Context) ([]domain.Downloader, error) {
	ds, err := store.Select[domain.Downloader](ctx, m.Store.Read(), "downloaders", "removed = 0 ORDER BY priority, name")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return ds, err
}

// --- qBittorrent ---

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
	if !source.ValidInfoHash(id) {
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

// MoveTop puts a torrent first in the queue; qBittorrent refuses it (409)
// while its queue is off.
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

// Status reads the server state from a full delta answer: free space and
// all-time totals are only there.
func (q *Qbittorrent) Status(ctx context.Context) (ClientStatus, error) {
	b, err := q.call(ctx, http.MethodGet, "sync/maindata", url.Values{"rid": {"0"}})
	if err != nil {
		return ClientStatus{}, err
	}
	var md struct {
		Torrents    map[string]struct{ State string } `json:"torrents"`
		ServerState struct {
			AllTimeDL int64 `json:"alltime_dl"`
			AllTimeUL int64 `json:"alltime_ul"`
			DLData    int64 `json:"dl_info_data"`
			ULData    int64 `json:"up_info_data"`
			DLSpeed   int64 `json:"dl_info_speed"`
			ULSpeed   int64 `json:"up_info_speed"`
			DLLimit   int64 `json:"dl_rate_limit"`
			ULLimit   int64 `json:"up_rate_limit"`
			Free      int64 `json:"free_space_on_disk"`
		} `json:"server_state"`
	}
	if err := json.Unmarshal(b, &md); err != nil {
		return ClientStatus{}, fmt.Errorf("qbittorrent sync/maindata: %w", err)
	}
	s := md.ServerState
	st := ClientStatus{Speed: s.DLSpeed, UpSpeed: s.ULSpeed, SpeedLimit: max(s.DLLimit, 0), UpLimit: max(s.ULLimit, 0),
		SessionDown: s.DLData, SessionUp: s.ULData, AllTimeDown: s.AllTimeDL, AllTimeUp: s.AllTimeUL, FreeSpace: s.Free}
	// Paused as a whole when there are torrents and none of them runs.
	st.Paused = len(md.Torrents) > 0
	for _, t := range md.Torrents {
		if !strings.HasPrefix(t.State, "stopped") && !strings.HasPrefix(t.State, "paused") {
			st.Paused = false
			break
		}
	}
	return st, nil
}

var qbitTrackerStatus = map[int]string{0: "disabled", 1: "not_contacted", 2: "working", 3: "working", 4: "not_working", 5: "error", 6: "unreachable"}

// Trackers lists a torrent's trackers, without the three fixed rows for
// DHT, peer exchange and local discovery.
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
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("qbittorrent torrents/trackers: %w", err)
	}
	out := []Tracker{}
	for _, r := range rows {
		if r.Tier < 0 {
			continue
		}
		out = append(out, Tracker{URL: trackerHost(r.URL), Tier: r.Tier, Status: qbitTrackerStatus[r.Status], Message: redactURLs(r.Msg), Seeds: max(r.Seeds, 0), Peers: max(r.Peers, 0)})
	}
	return out, nil
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
	return out, nil
}

// --- SABnzbd ---

func (s *Sabnzbd) queueAct(ctx context.Context, name, id string) error {
	if id == "" || strings.Contains(id, ",") {
		return ErrNotFound
	}
	_, err := s.call(ctx, "queue", url.Values{"name": {name}, "value": {id}}, true)
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
		return ErrNotFound
	}
	return nil
}

// SetSpeedLimit caps the download speed. The value goes out with a K so
// SABnzbd reads it as a rate, never as a percent. Usenet has no upload.
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
	st := ClientStatus{Paused: qu.Queue.Paused, Speed: int64(num(qu.Queue.KBPerSec) * 1024), SpeedLimit: int64(num(qu.Queue.Limit)), FreeSpace: free}
	b, err = s.call(ctx, "server_stats", nil, true)
	if err != nil {
		return st, nil // the counters are extra; the header is enough
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

// trackerHost keeps only scheme and host of a tracker address, so passkeys
// in the path, query or user part never leave HolyArrs. Entries like
// "** [DHT] **" are not addresses and pass through.
func trackerHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		if strings.Contains(raw, "://") {
			return "tracker"
		}
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// urlish finds address-shaped words in free text (tracker messages can echo
// the announce address, passkey included).
var urlish = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// redactURLs swaps every address in a message for its trackerHost.
func redactURLs(msg string) string {
	return urlish.ReplaceAllStringFunc(msg, trackerHost)
}
