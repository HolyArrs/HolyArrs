package backups

import (
	"context"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Preset is a ready-made schedule and keep rule.
type Preset string

// Rule says how often a preset backs up and what it keeps.
type Rule struct {
	Every   time.Duration
	Nightly int           // newest scheduled backups always kept
	Weekly  int           // plus the newest backup of this many earlier weeks
	MaxAge  time.Duration // or: keep everything younger than this
}

// Meta is the content of backup.json.
type Meta struct {
	App          string    `json:"app"`
	Version      string    `json:"version"`
	SchemaMain   int       `json:"schema_main"`
	SchemaEvents int       `json:"schema_events"`
	Kind         string    `json:"kind"`
	CreatedAt    time.Time `json:"created_at"`
	Note         string    `json:"note"`
	// Detail says what the backup is, such as "before updating from 0.3.0
	// to 0.4.0" (pre-update backups only).
	Detail string `json:"detail,omitempty"`
	// FromVersion and ToVersion name the update a pre-update backup was
	// taken for, when the version that ran before is known.
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
}

// Manager makes, lists, prunes and restores backups.
type Manager struct {
	Store   *database.Store
	Dir     string // the config folder
	Version string
	Now     func() time.Time
	// CheckKey, when set, is asked whether a database file can be read with
	// the current master key, before a restore is staged.
	CheckKey func(dbPath string) error
	// Key is the master key, for the key kit download and the Backups card.
	Key []byte
	// KeyFromEnv says the key is a named file (HOLYARRS_MASTER_KEY_FILE)
	// the person looks after, so the card does not ask for a key kit.
	KeyFromEnv bool
	// FreeSpace reads free bytes on a disk (nil means importer.FreeSpace;
	// -1 unknown).
	FreeSpace func(dir string) int64
	// Stop, when set, ends with HolyArrs (shutdown). A manual backup stops
	// only then, never because the browser or a proxy dropped the request.
	Stop context.Context
	// OnPreset, when set, is told about each saved preset (the health
	// checks run again so backup warnings go away when backups are off).
	OnPreset func(p Preset)

	mu      sync.Mutex
	lastErr string // the last scheduled backup's error, "" after a good one
	passed  bool   // a scheduled pass has finished since start
}

// Info is one backup file.
type Info struct {
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size"`
	Detail    string    `json:"detail,omitempty"`
	// FromVersion and ToVersion: the update a pre-update backup was taken
	// for (when known). Going back to FromVersion takes this backup.
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
	Path        string `json:"-"`
}
