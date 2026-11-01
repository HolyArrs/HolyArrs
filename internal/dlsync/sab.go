package dlsync

import (
	"context"
	"sync"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// SABnzbd's reads: the whole queue every poll (it changes all the time),
// the finished list only when its change counter moved. The counter starts
// again at 1 when SABnzbd restarts, so a counter lower than the last one
// means a fresh full read.

type sabCursor struct {
	last int64
	hist []downloader.Job
}

// SabCursors keeps the finished list and its counter per (Downloader,
// engine generation). A new generation reads everything again.
type SabCursors struct {
	mu sync.Mutex
	by map[cursorKey]*sabCursor
}

// SabPoll is one read of a SABnzbd: queue and finished list, merged.
type SabPoll struct {
	Jobs        []downloader.Job
	Head        downloader.QueueHead
	HistoryRead bool // the finished list was read (it had changed)
	Reset       bool // the counter went down: SABnzbd restarted
}

// SabSource is what a SABnzbd poll needs: the guarded engine's queue and
// finished-list reads.
type SabSource interface {
	downloader.QueueReader
	downloader.HistoryReader
}

func (c *SabCursors) cursor(id int64, gen uint64) *sabCursor {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[cursorKey]*sabCursor{}
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
	cur := &sabCursor{}
	c.by[k] = cur
	return cur
}

// NeedsFull says whether the next poll reads the whole finished list (no
// counter kept yet for this Downloader).
func (c *SabCursors) NeedsFull(id int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, cur := range c.by {
		if k.id == id && cur.last > 0 {
			return false
		}
	}
	return true
}

// Drop forgets a Downloader's counter: the next poll reads everything.
func (c *SabCursors) Drop(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.by {
		if k.id == id {
			delete(c.by, k)
		}
	}
}

// Poll reads the queue and, when it changed, the finished list. Any error
// drops the counter so the next poll is a full read.
func (c *SabCursors) Poll(ctx context.Context, id int64, gen uint64, s SabSource) (SabPoll, error) {
	cur := c.cursor(id, gen)
	q, err := s.Queue(ctx)
	if err != nil {
		c.Drop(id)
		return SabPoll{}, err
	}
	h, err := s.History(ctx, cur.last)
	if err != nil {
		c.Drop(id)
		return SabPoll{}, err
	}
	p := SabPoll{Head: q.Head}
	if h.Changed {
		if cur.last > 0 && h.Last < cur.last {
			p.Reset = true
			if h.Last > 0 {
				// Read once more from scratch: the answer to an old counter
				// after a restart is not trusted as the whole list.
				if h, err = s.History(ctx, 0); err != nil {
					c.Drop(id)
					return SabPoll{}, err
				}
			}
		}
		cur.hist, cur.last, p.HistoryRead = h.Jobs, h.Last, true
	}
	seen := make(map[string]bool, len(q.Jobs))
	p.Jobs = make([]downloader.Job, 0, len(q.Jobs)+len(cur.hist))
	for _, j := range q.Jobs {
		seen[j.ID] = true
		p.Jobs = append(p.Jobs, j)
	}
	for _, j := range cur.hist {
		// A job moving from the queue to the finished list can show in both
		// for a moment: the queue's row wins.
		if !seen[j.ID] {
			p.Jobs = append(p.Jobs, j)
		}
	}
	return p, nil
}
