package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// KindGrab is the Job kind that sends a chosen Release to a Downloader.
const KindGrab = "grab"

// vanishAfterMisses is how many syncs in a row a download must be missing from
// its Downloader before it counts as vanished. An empty list while a share
// mounts or the client restarts is not a removal.
const vanishAfterMisses = 3

// vanishMissingFor is how long a download must also have been missing. Syncs
// come every few seconds during a download, and a client that restarts or waits
// for a NAS share after boot can list nothing for minutes.
const vanishMissingFor = 10 * time.Minute

// activeStates are the Download states the sync follows.
const activeStates = "'sent', 'queued', 'downloading', 'stalled', 'paused'"

// clientSaysPrefix starts the reason of a failure the Downloader reports itself.
// Such a failure never deletes the files.
const clientSaysPrefix = "the Downloader says "

// WaitingReason starts the reason of a download whose Downloader cannot use its
// files, most often a share mounted late.
const WaitingReason = "the Downloader can't use the files"

// WaitingHint ends that reason. Mounting the share is not enough: qBittorrent,
// Deluge and Transmission keep the error until the files are checked again.
const WaitingHint = "is the share mounted? Once it is, recheck the download in the Downloader (or use Check again on the Downloads page)"

// Publisher sends live events (live.Hub fits).
type Publisher interface {
	Publish(topic string, data any) uint64
}

// Manager sends Releases to Downloaders and keeps Download records in step.
type Manager struct {
	Store    *database.Store
	Live     Publisher                                 // may be nil
	Engine   func(d domain.Downloader) (Engine, error) // nil means the built-in adapters
	SavePath string                                    // where the Downloader saves, as it sees it
	// StallAfter and ActAfter override the settings stalled_after_hours and
	// stalled_act_hours (tests); zero reads the settings.
	StallAfter  time.Duration
	ActAfter    time.Duration
	VanishGrace time.Duration                                    // how long a sent Download may be unlisted; default 2m
	Active      time.Duration                                    // poll interval with active Downloads; default 2s
	Idle        time.Duration                                    // poll interval when idle; default 30s
	HealthEvery time.Duration                                    // status check of every Downloader; default 1m
	Now         func() time.Time                                 // test clock
	Sleep       func(ctx context.Context, d time.Duration) error // test sleep; nil waits for real
	OnFinished  func(ctx context.Context, downloadID int64)      // starts the Import
	// OnFailed is told about each Slot whose Download failed and should be
	// searched again. The search then picks the next Release, or the Slot needs a
	// decision.
	OnFailed func(ctx context.Context, slotID int64)
	// Link gives the link to send for a Release: its stored link with the Source's
	// key put back from the vault. Nil sends the stored link.
	Link func(ctx context.Context, rel domain.Release) (string, error)
	// SourceTransport fetches a Release file for a Source with a proxy, going
	// through the proxy with every address checked. Nil refuses such a send to
	// qBittorrent or SABnzbd.
	SourceTransport func(ctx context.Context, src domain.Source) (http.RoundTripper, error)
	// FromVault reads a Downloader's key or password from the vault.
	FromVault func(ctx context.Context, id int64) (string, error)
	// Sync, when set and on (downloads.sync = on), is the background reader.
	// SyncOnce reads its snapshots instead of listing clients, and falls back to its
	// own guarded List only for a client the reader has not read freshly for too
	// long.
	Sync SyncSource

	mu               sync.Mutex
	engines          map[int64]Engine
	engineGeneration map[int64]uint64
	wake             chan struct{}
	rr               map[string]int           // round robin turn per protocol and priority
	hosts            map[string]chan struct{} // one send at a time per host (a slot of one)
	lastSend         map[string]time.Time
	missed           map[int64]missCount // downloads missing from their Downloader, by id
}

// missCount is how many syncs in a row a download was missing, the snapshot
// time of the last one counted (a reused snapshot counts once), and when the
// first was counted.
type missCount struct {
	n     int
	at    time.Time
	first time.Time
}

// releaseRefused is a send this Release can never make on any Downloader, so
// trying again changes nothing.
type releaseRefused struct{ msg string }

// grabFailed holds each Downloader's refusal of one grab, in the order tried.
// It is final only when every refusal was the Release's own.
type grabFailed []error

// TagsError says no Downloader of the protocol takes the title's tags.
type TagsError struct {
	Protocol string
	Title    []string
}

// Forget drops the cached engine of a Downloader after an edit.
func (m *Manager) Forget(id int64) {
	m.mu.Lock()
	delete(m.engines, id)
	if m.engineGeneration == nil {
		m.engineGeneration = map[int64]uint64{}
	}
	m.engineGeneration[id]++
	m.mu.Unlock()
	m.Kick(id) // read with the new settings now, not after a widened wait
}

// Generation is the Downloader's engine generation. It moves on every Forget
// (an edit), so state kept per engine can tell it is stale.
func (m *Manager) Generation(id int64) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.engineGeneration[id]
}

// Syncer gives the Downloader's guarded engine for background reads, with the
// generation it belongs to. Every call on it goes through the guard.
func (m *Manager) Syncer(ctx context.Context, d domain.Downloader) (Syncer, uint64, error) {
	gen := m.Generation(d.ID)
	e, err := m.engine(ctx, d)
	if err != nil {
		return nil, gen, err
	}
	s, ok := e.(Syncer)
	if !ok {
		return nil, gen, ErrNotSupported
	}
	return s, gen, nil
}

// EngineFor builds the built-in adapter for a Downloader, with its secret from
// the vault and a network policy that keeps it off the local network unless the
// Downloader may reach it.
func (m *Manager) EngineFor(ctx context.Context, d domain.Downloader) (Engine, error) {
	secret := ""
	if d.SecretRef != nil {
		if m.FromVault == nil {
			return nil, errors.New("the Downloader's key cannot be read: the vault is not open")
		}
		var err error
		if secret, err = m.FromVault(ctx, *d.SecretRef); err != nil {
			return nil, errors.New("the Downloader's key cannot be read from the vault")
		}
	}
	h := Client(d.URL, d.AllowLocal)
	switch d.Adapter {
	case "qbittorrent":
		if d.Username != "" {
			return NewQbittorrentLogin(d.URL, d.Username, secret, h), nil
		}
		return NewQbittorrentKey(d.URL, secret, h), nil
	case "sabnzbd":
		return NewSabnzbd(d.URL, secret, h), nil
	case "transmission":
		e := NewTransmission(d.URL, d.Username, secret, h)
		e.category, e.others = d.Category, otherCategories(d)
		return e, nil
	case "nzbget":
		e := NewNZBGet(d.URL, d.Username, secret, h)
		e.Category, e.Others = d.Category, otherCategories(d)
		return e, nil
	case "deluge":
		e := NewDeluge(d.URL, secret, h)
		e.Category, e.Others = d.Category, otherCategories(d)
		return e, nil
	}
	return nil, fmt.Errorf("no engine for adapter %q", d.Adapter)
}

func (e *TagsError) Error() string {
	if len(e.Title) == 0 {
		return fmt.Sprintf("not sent: every %s Downloader has tags and this title has none of them; tag the title, or leave one Downloader without tags", e.Protocol)
	}
	return fmt.Sprintf("not sent: no %s Downloader has this title's tags (%s); add one of them to a Downloader, or take them off the title", e.Protocol, strings.Join(e.Title, ", "))
}

// missedLongEnough counts one more missed sync for a download and reports
// whether it has been missing for vanishAfterMisses syncs in a row and for
// vanishMissingFor. The count then starts over.
func (m *Manager) missedLongEnough(id int64, readAt time.Time) bool {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.missed == nil {
		m.missed = map[int64]missCount{}
	}
	c := m.missed[id]
	if readAt.IsZero() || !readAt.Equal(c.at) {
		c.n++
		c.at = readAt
		if c.n == 1 {
			c.first = now
		}
	}
	m.missed[id] = c
	return c.n >= vanishAfterMisses && now.Sub(c.first) >= vanishMissingFor
}

func (m *Manager) seen(id int64) {
	m.mu.Lock()
	delete(m.missed, id)
	m.mu.Unlock()
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) publish(topic string, data any) {
	if m.Live != nil {
		m.Live.Publish(topic, data)
	}
}

func (m *Manager) engine(ctx context.Context, d domain.Downloader) (Engine, error) {
	m.mu.Lock()
	if e := m.engines[d.ID]; e != nil {
		m.mu.Unlock()
		return e, nil
	}
	generation := m.engineGeneration[d.ID]
	m.mu.Unlock()
	var e Engine
	var err error
	if m.Engine != nil {
		e, err = m.Engine(d)
	} else {
		e, err = m.EngineFor(ctx, d)
		if err == nil {
			// Inside the guard, so refused writes never reach outbound or the client.
			if ReadOnlyByEnv(d) {
				e = readOnly(e)
			} else if !d.Writable {
				e = readOnlyAdds(e) // takes grabs, changes nothing already there
			}
			instance := fmt.Sprintf("%p", m)
			if m.Store != nil {
				instance = m.Store.Dir()
			}
			e = protectEngine(e, d, instance)
		}
	}
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if generation != m.engineGeneration[d.ID] {
		m.mu.Unlock()
		return nil, errors.New("the Downloader changed while connecting; try again")
	}
	if m.engines == nil {
		m.engines = map[int64]Engine{}
	}
	if existing := m.engines[d.ID]; existing != nil {
		m.mu.Unlock()
		return existing, nil
	}
	m.engines[d.ID] = e
	m.mu.Unlock()
	return e, nil
}

// otherCategories lists d's categories other than d.Category, the one in use.
func otherCategories(d domain.Downloader) []string {
	var out []string
	for _, c := range d.Categories() {
		if c != d.Category {
			out = append(out, c)
		}
	}
	return out
}

// listCategories lists the jobs in each of cats, once each.
func listCategories(ctx context.Context, e Engine, cats []string) ([]Job, error) {
	if len(cats) == 1 {
		return e.List(ctx, cats[0])
	}
	seen := map[string]bool{}
	var out []Job
	for _, c := range cats {
		jobs, err := e.List(ctx, c)
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			if !seen[j.ID] {
				seen[j.ID] = true
				out = append(out, j)
			}
		}
	}
	return out, nil
}
