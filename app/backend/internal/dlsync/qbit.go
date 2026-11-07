package dlsync

import (
	"context"
	"sync"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

type cursorKey struct {
	id  int64
	gen uint64
}

type mainCursor struct {
	state    downloader.MainState
	recent   []bool // full answers among the last polls sent with a valid rid
	baseline bool   // the next full answer follows a reset and is not counted
	status   *downloader.ClientStatus
}

// Cursors keeps qBittorrent's change-feed cursor per (Downloader,
// engine generation). A new generation starts again from rid 0.
type Cursors struct {
	mu sync.Mutex
	by map[cursorKey]*mainCursor
}

// MainPoll is one change-feed read, merged.
type MainPoll struct {
	Jobs          []downloader.Job
	Full          bool
	SharedSession bool // more than 3 full answers in 10 polls with a valid rid
	Bytes         int
}

func (c *Cursors) cursor(id int64, gen uint64) *mainCursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[cursorKey]*mainCursor{}
	}
	k := cursorKey{id, gen}
	if cur := c.by[k]; cur != nil {
		return cur
	}
	for old := range c.by {
		if old.id == id {
			delete(c.by, old)
		}
	}
	cur := &mainCursor{baseline: true}
	c.by[k] = cur
	return cur
}

// Drop forgets a Downloader's cursor (Forget, "Try again"): the next poll
// sends rid 0.
func (c *Cursors) Drop(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.by {
		if k.id == id {
			delete(c.by, k)
		}
	}
}

// Status is the client state from the last merged read, if any.
func (c *Cursors) Status(id int64) (downloader.ClientStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, cur := range c.by {
		if k.id == id && cur.status != nil {
			return *cur.status, true
		}
	}
	return downloader.ClientStatus{}, false
}

// Rid is the cursor the next poll would send (0 when none).
func (c *Cursors) Rid(id int64, gen uint64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur := c.by[cursorKey{id, gen}]; cur != nil {
		return cur.state.Rid
	}
	return 0
}

// Poll reads the change feed once through s (the guarded engine) and
// merges it. A read or merge error drops the cursor so the next poll is a
// full answer.
func (c *Cursors) Poll(ctx context.Context, id int64, gen uint64, s downloader.MainSyncer) (MainPoll, error) {
	cur := c.cursor(id, gen)
	rid := cur.state.Rid
	d, err := s.SyncMain(ctx, rid)
	if err != nil {
		c.Drop(id)
		return MainPoll{}, err
	}
	if _, err := cur.state.Apply(d); err != nil {
		c.Drop(id)
		return MainPoll{}, err
	}
	p := MainPoll{Jobs: cur.state.Jobs(""), Full: d.FullUpdate, Bytes: d.Bytes}
	if rid > 0 && !cur.baseline {
		cur.recent = append(cur.recent, d.FullUpdate)
		if len(cur.recent) > 10 {
			cur.recent = cur.recent[len(cur.recent)-10:]
		}
	}
	cur.baseline = false
	st := downloader.StatusFromMain(&cur.state)
	c.mu.Lock()
	cur.status = &st
	c.mu.Unlock()
	full := 0
	for _, f := range cur.recent {
		if f {
			full++
		}
	}
	p.SharedSession = full > 3
	return p, nil
}
