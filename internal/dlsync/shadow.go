package dlsync

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// The setting downloads.sync: legacy (the default), shadow, or on (Phase 2).
// It is stored in the Downloads settings group ("links", where the stall
// and grab settings live; the settings table only allows known groups) as
// key "downloads_sync".
const (
	SettingGroup = "links"
	SettingSync  = "downloads_sync"
	ModeLegacy   = "legacy"
	ModeShadow   = "shadow"
	ModeOn       = "on"
)

// modeTTL is how long a read of the setting is kept, so the flag changes
// without a restart but SyncOnce does not query it every tick.
const modeTTL = 5 * time.Second

// Shadow feeds a Store from the lists downloader.Manager already reads
// (Manager.OnList). It never contacts a client.
type Shadow struct {
	Store *Store
	DB    *store.Store
	Now   func() time.Time

	mu     sync.Mutex
	mode   string
	readAt time.Time
}

func (sh *Shadow) now() time.Time {
	if sh.Now != nil {
		return sh.Now()
	}
	return time.Now()
}

// Mode reads downloads.sync, kept for a few seconds.
func (sh *Shadow) Mode(ctx context.Context) string {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.mode != "" && sh.now().Sub(sh.readAt) < modeTTL {
		return sh.mode
	}
	mode := ModeLegacy
	if sh.DB != nil {
		var v string
		err := sh.DB.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = ? AND key = ?", SettingGroup, SettingSync).Scan(&v)
		if err == nil && (v == ModeShadow || v == ModeOn) {
			mode = v
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			mode = ModeLegacy
		}
	}
	sh.mode, sh.readAt = mode, sh.now()
	return mode
}

// OnList is downloader.Manager.OnList: in shadow mode it records the list
// the Manager just read. Phase 1 only has shadow, so "on" acts as shadow.
func (sh *Shadow) OnList(id int64, gen uint64, _ string, jobs []downloader.Job, err error) {
	if sh == nil || sh.Store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if sh.Mode(ctx) == ModeLegacy {
		return
	}
	if err != nil {
		sh.Store.Fail(id, gen)
		return
	}
	sh.Store.Update(id, gen, jobs, sh.now())
}
