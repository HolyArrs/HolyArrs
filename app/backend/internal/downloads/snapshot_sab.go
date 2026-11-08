package downloads

import (
	"context"
	"sync"
)

// SabSource is what a SABnzbd poll needs: the guarded engine's queue and
// finished-list reads.
type SabSource interface {
	QueueReader
	HistoryReader
}

// SabCursors keeps the finished list and its change counter per Downloader
// and engine generation. A new generation reads everything again. The queue
// is read in full every poll because it changes all the time; the finished
// list is read only when its counter moved.
type SabCursors struct {
	mu sync.Mutex
	by map[cursorKey]*sabCursor
}

// SabPoll is one read of a SABnzbd: queue and finished list, merged.
type SabPoll struct {
	Jobs        []Job
	Head        QueueHead
	HistoryRead bool // the finished list was read because it had changed
	Reset       bool // the counter went down, so SABnzbd restarted
}

// sabCursor is the last finished list and counter of one SABnzbd.
type sabCursor struct {
	last int64
	hist []Job
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
		// SABnzbd starts its counter at 1 again after a restart, so a lower
		// counter than the last one means a restart.
		if cur.last > 0 && h.Last < cur.last {
			p.Reset = true
			if h.Last > 0 {
				// The answer to an old counter after a restart is not trusted
				// as the whole list, so read it again from scratch.
				if h, err = s.History(ctx, 0); err != nil {
					c.Drop(id)
					return SabPoll{}, err
				}
			}
		}
		cur.hist, cur.last, p.HistoryRead = h.Jobs, h.Last, true
	}
	seen := make(map[string]bool, len(q.Jobs))
	p.Jobs = make([]Job, 0, len(q.Jobs)+len(cur.hist))
	for _, j := range q.Jobs {
		seen[j.ID] = true
		p.Jobs = append(p.Jobs, j)
	}
	for _, j := range cur.hist {
		// A job moving from the queue to the finished list can show in both
		// for a moment, and the queue's row is the newer one.
		if !seen[j.ID] {
			p.Jobs = append(p.Jobs, j)
		}
	}
	return p, nil
}

// NeedsFull reports whether the next poll reads the whole finished list,
// which is so while no counter is kept for this Downloader.
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

// Drop forgets a Downloader's counter, so the next poll reads everything.
func (c *SabCursors) Drop(id int64) {
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
