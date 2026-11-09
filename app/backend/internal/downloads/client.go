package downloads

import (
	"context"
	"errors"
	"time"
)

// State values every engine maps its own states onto.
const (
	StateQueued      = "queued"
	StateDownloading = "downloading"
	StatePaused      = "paused"
	StateFinished    = "finished"
	StateFailed      = "failed"
)

// Errors shared by all engines, so callers can test them with errors.Is.
var (
	ErrNotFound  = errors.New("download not found in the Downloader")
	ErrAuth      = errors.New("the Downloader wants a sign-in")
	ErrTooLarge  = errors.New("the Downloader's answer is too large to read")
	ErrUncertain = errors.New("the Downloader send outcome is unknown; inspect it before choosing another Release")
)

// AddRequest is one Release to send to an engine.
type AddRequest struct {
	URL string
	// File is the torrent or nzb already fetched from URL. qBittorrent and SABnzbd take it instead of the address.
	File []byte
	// Name is the Release name. Usenet jobs are found again by it.
	Name     string
	Category string
	SavePath string
	// InfoHash, when known, becomes the external id.
	InfoHash string
	// AllowLocal lets the Source fetch Release files on the local network.
	AllowLocal bool
	// OperationKey ties a retry to the original grab when the first answer was lost.
	OperationKey string
	// AttemptMarker is a durable, opaque id claimed before sending. NZBGet stores it remotely; torrent clients match by InfoHash.
	// ReconcileOnly means look for the earlier add and never send another or change a category.
	AttemptMarker string
	ReconcileOnly bool
	// Skip holds ids that belong to other Download rows, so a by-name lookup never takes them for this Release.
	Skip map[string]bool
}

// File is one file inside a download.
type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Progress runs from 0 to 1.
	// Speed is bytes per second and ETA is seconds, 0 when unknown.
	Progress float64 `json:"progress"`
}

// Job is one download as the engine sees it.
type Job struct {
	// ID is the external id (the info hash for torrents).
	ID          string
	Name        string
	Category    string
	SavePath    string
	ContentPath string
	Size        int64
	Progress    float64
	Speed       int64
	UpSpeed     int64
	// Seeds and Peers count connections; the Total fields count the whole swarm. Torrents only.
	Seeds      int
	Peers      int
	SeedsTotal int
	PeersTotal int
	ETA        int64
	// State is one of the State constants. Detail keeps the engine's own state word for showing reasons.
	State  string
	Detail string
	Files  []File
	// AddedAt is unix seconds when the client took it, 0 when unknown.
	AddedAt int64
	Ratio   float64
	// Limits are the torrent's own seeding rules, nil when the client does not report them.
	Limits *ShareLimits
	// DoneAt is unix seconds when the torrent finished, 0 if not yet. QueuePos is its place in the client queue, 0 if not queued.
	// Tracker is the host it works with now, empty if none.
	DoneAt   int64
	QueuePos int
	Tracker  string
	// TrackerProblem is error, warning or announce when trackers are unwell, empty when fine or not reported.
	TrackerProblem string
	// LimitDown and LimitUp are the torrent's own speed caps in bytes per second, 0 for none.
	LimitDown, LimitUp int64
	// LastActivity and SeenComplete are unix seconds, 0 when unknown. SeedingTime and TimeActive are seconds; Uploaded is bytes.
	LastActivity, SeenComplete, SeedingTime, TimeActive, Uploaded int64
	// PostedAt is unix seconds when the Usenet post was made. 0 when unknown, and always 0 for torrents.
	PostedAt int64
	// Availability is how many full copies the swarm holds, nil when the client does not say.
	Availability *float64
}

// Health is what an engine says about itself.
type Health struct {
	Version string
	At      time.Time
	// Note is a caveat the connection test shows beside a pass.
	Note string
}

// Engine is one download engine. A new client implements these five methods and nothing else.
type Engine interface {
	Health(ctx context.Context) (Health, error)
	Add(ctx context.Context, r AddRequest) (id string, err error)
	List(ctx context.Context, category string) ([]Job, error)
	Get(ctx context.Context, id string) (Job, error)
	Remove(ctx context.Context, id string, deleteFiles bool) error
}

// AllLister is the optional read-only part of a torrent engine that lists every item in the client, whatever its category, for the Downloads screen.
// Nothing it returns becomes a HolyArrs download.
type AllLister interface {
	ListAll(ctx context.Context) ([]Job, error)
}
