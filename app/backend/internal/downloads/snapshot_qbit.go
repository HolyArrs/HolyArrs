package downloads

import (
	"context"
	"sync"
)

// Cursors keeps qBittorrent's change-feed cursor per Downloader and engine
// generation. A new generation starts again from rid 0.
type Cursors struct {
	mu sync.Mutex
	by map[cursorKey]*mainCursor
}

// MainPoll is one change-feed read, merged.
type MainPoll struct {
	Jobs []Job
	Full bool
	// SharedSession is set when more than 3 of the last 10 polls with a valid
	// rid got a full answer, which means another program shares the session.
	SharedSession bool
	Bytes         int
}

// cursorKey identifies one cursor. It is shared with SabCursors.
type cursorKey struct {
	id  int64
	gen uint64
}

// mainCursor is the merged state of one qBittorrent change feed.
type mainCursor struct {
	state    MainState
	recent   []bool // full answers among the last polls sent with a valid rid
	baseline bool   // the next full answer follows a reset and is not counted
	status   *ClientStatus
}

// Poll reads the change feed once through s (the guarded engine) and merges
// it. A read or merge error drops the cursor so the next poll is a full
// answer.
func (c *Cursors) Poll(ctx context.Context, id int64, gen uint64, s MainSyncer) (MainPoll, error) {
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
	// Only polls that sent a valid rid count: a full answer to rid 0 is
	// expected and says nothing about sharing.
	if rid > 0 && !cur.baseline {
		cur.recent = append(cur.recent, d.FullUpdate)
		if len(cur.recent) > 10 {
			cur.recent = cur.recent[len(cur.recent)-10:]
		}
	}
	cur.baseline = false
	st := StatusFromMain(&cur.state)
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

// Rid returns the cursor the next poll would send, or 0 when there is none.
func (c *Cursors) Rid(id int64, gen uint64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cur := c.by[cursorKey{id, gen}]; cur != nil {
		return cur.state.Rid
	}
	return 0
}

// Status returns the client state from the last merged read, if any.
func (c *Cursors) Status(id int64) (ClientStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, cur := range c.by {
		if k.id == id && cur.status != nil {
			return *cur.status, true
		}
	}
	return ClientStatus{}, false
}

// Drop forgets a Downloader's cursor, so the next poll sends rid 0. Used
// when a Downloader is removed or retried.
func (c *Cursors) Drop(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.by {
		if k.id == id {
			delete(c.by, k)
		}
	}
}

// cursor returns the cursor for one Downloader and generation, making it
// and discarding older generations of the same Downloader.
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
