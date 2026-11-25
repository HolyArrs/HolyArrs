package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// AddJob is what a download-client plugin receives for one grab: the
// .torrent or .nzb file, or a magnet link. Never the indexer's own link,
// which can hold the keys of other sites.
type AddJob struct {
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
	File     []byte `json:"file,omitempty"`
	Magnet   string `json:"magnet,omitempty"`
	InfoHash string `json:"info_hash,omitempty"`
	Stopped  bool   `json:"stopped,omitempty"`
}

// PluginEngine is a download client served by a plugin through the Broker.
// It implements downloads.Engine plus pause and resume of one item.
type PluginEngine struct {
	Broker *Broker
	Plugin string
	// Wait is how long one call waits for the plugin; 0 means 30 seconds.
	Wait time.Duration
}

var _ downloads.Engine = (*PluginEngine)(nil)

func (e *PluginEngine) call(ctx context.Context, op string, in, out any) error {
	w := e.Wait
	if w <= 0 {
		w = 30 * time.Second
	}
	raw, err := e.Broker.Call(ctx, e.Plugin, Downloader, op, in, w)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("the plugin's answer could not be read")
	}
	return nil
}

// Health asks the plugin for its version.
func (e *PluginEngine) Health(ctx context.Context) (downloads.Health, error) {
	var v struct {
		Version string `json:"version"`
	}
	if err := e.call(ctx, "health", nil, &v); err != nil {
		return downloads.Health{}, err
	}
	return downloads.Health{Version: clip(v.Version, 60), At: time.Now()}, nil
}

// Add hands the plugin the release file or magnet.
func (e *PluginEngine) Add(ctx context.Context, r downloads.AddRequest) (string, error) {
	a := AddJob{Name: r.Name, Category: r.Category, InfoHash: r.InfoHash, Stopped: r.Stopped}
	switch {
	case len(r.File) > 0:
		a.File = r.File
	case strings.HasPrefix(r.URL, "magnet:"):
		a.Magnet = r.URL
	default:
		return "", errors.New("a plugin download client takes the release file, and it could not be read")
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := e.call(ctx, "add", a, &v); err != nil {
		return "", err
	}
	if v.ID == "" {
		return "", errors.New("the plugin took the release but gave no id")
	}
	return clip(v.ID, 200), nil
}

// List is the plugin's queue for category.
func (e *PluginEngine) List(ctx context.Context, category string) ([]downloads.Job, error) {
	var out []downloads.Job
	err := e.call(ctx, "list", map[string]string{"category": category}, &out)
	return out, err
}

// Get is one download.
func (e *PluginEngine) Get(ctx context.Context, id string) (downloads.Job, error) {
	var out downloads.Job
	if err := e.call(ctx, "get", map[string]string{"id": id}, &out); err != nil {
		return downloads.Job{}, downloads.ErrNotFound
	}
	return out, nil
}

// Remove removes one download, with its files when asked.
func (e *PluginEngine) Remove(ctx context.Context, id string, deleteFiles bool) error {
	return e.call(ctx, "remove", map[string]any{"id": id, "delete_files": deleteFiles}, nil)
}

// PauseItem pauses one download.
func (e *PluginEngine) PauseItem(ctx context.Context, id string) error {
	return e.call(ctx, "pause", map[string]string{"id": id}, nil)
}

// ResumeItem resumes one download.
func (e *PluginEngine) ResumeItem(ctx context.Context, id string) error {
	return e.call(ctx, "resume", map[string]string{"id": id}, nil)
}
