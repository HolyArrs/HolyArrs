// Package downloader sends Releases to Downloaders and follows their
// progress. Each download engine sits behind the Engine interface.
package downloader

import (
	"context"
	"errors"
	"time"
)

// Our Download states (model.md), as engines report them.
const (
	StateQueued      = "queued"
	StateDownloading = "downloading"
	StatePaused      = "paused"
	StateFinished    = "finished"
	StateFailed      = "failed"
)

// Errors every engine uses.
var (
	ErrNotFound  = errors.New("download not found in the Downloader")
	ErrAuth      = errors.New("the Downloader wants a sign-in")
	ErrUncertain = errors.New("the Downloader send outcome is unknown; inspect it before choosing another Release")
)

// AddRequest is one Release to send.
type AddRequest struct {
	URL        string // link to the torrent or nzb
	Name       string // the Release name; Usenet jobs are found again by it
	Category   string
	SavePath   string
	InfoHash   string // when known, it becomes the external id
	AllowLocal bool   // the Source may fetch Release files on the local network
	// OperationKey identifies the original grab across an uncertain response.
	OperationKey string
	// AttemptMarker is a durable, opaque identity claimed before sending.
	// NZBGet stores it remotely; torrent clients reconcile by InfoHash.
	// ReconcileOnly never sends another add or changes a category.
	AttemptMarker string
	ReconcileOnly bool
	// Skip are ids an engine must not take as this Release when it finds a
	// job again by name: they belong to other Download rows.
	Skip map[string]bool
}

// File is one file inside a download.
type File struct {
	Name     string  `json:"name"` // relative to the download's save path
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
}

// Job is one download as the engine sees it.
type Job struct {
	ID          string // external id (info hash for torrents)
	Name        string
	Category    string
	SavePath    string
	ContentPath string
	Size        int64
	Progress    float64 // 0 to 1
	Speed       int64   // bytes per second
	UpSpeed     int64   // upload bytes per second (torrents)
	Seeds       int     // connected seeds (torrents)
	Peers       int     // connected peers (torrents)
	ETA         int64   // seconds, 0 when unknown
	State       string  // one of the State constants
	Detail      string  // the engine's own state word, for reasons
	Files       []File
}

// Health is what an engine says about itself.
type Health struct {
	Version string
	At      time.Time
}

// Engine is one download engine. A new engine (another torrent client, a
// Usenet client) implements these five methods and nothing else.
type Engine interface {
	Health(ctx context.Context) (Health, error)
	Add(ctx context.Context, r AddRequest) (id string, err error)
	List(ctx context.Context, category string) ([]Job, error)
	Get(ctx context.Context, id string) (Job, error) // with Files
	Remove(ctx context.Context, id string, deleteFiles bool) error
}
