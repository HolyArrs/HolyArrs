package downloads

import (
	"context"
	"errors"
	"sync"
	"time"
)

// rtSweep is how often the reader reads an rTorrent's whole list while nobody
// views the Downloads page. Between sweeps it reads only what is still
// downloading, so a torrent added outside HolyArrs already complete shows at
// the next sweep.
var rtSweep = 5 * time.Minute

// rtCursors keeps each rTorrent's last whole list, so a background read fetches
// only what is still downloading and keeps the finished torrents as last read.
type rtCursors struct {
	mu sync.Mutex
	m  map[int64]*rtCursor
}

type rtCursor struct {
	gen    uint64
	items  map[string]Job
	fullAt time.Time
}

// poll returns the client's list in cats. It reads the whole list on the first
// read, after a settings change, on the sweep and while full is set (the
// Downloads page is open). Otherwise it reads what is still downloading and,
// by hash, follow (the Downloads HolyArrs follows) and every torrent it last
// saw unfinished, so a torrent that just finished is seen as finished and a
// removed one goes missing.
func (c *rtCursors) poll(ctx context.Context, id int64, gen uint64, e Engine, cats, follow []string, full bool, now time.Time) ([]Job, error) {
	c.mu.Lock()
	cur := c.m[id]
	c.mu.Unlock()
	al, ok := e.(ActiveLister)
	var next map[string]Job
	whole := !ok || full || cur == nil || cur.gen != gen || now.Sub(cur.fullAt) >= rtSweep
	var got []Job
	var ids []string
	if !whole {
		ids = append([]string(nil), follow...)
		for k, j := range cur.items {
			if j.State != StateFinished {
				ids = append(ids, k)
			}
		}
		var err error
		got, err = al.ListActive(ctx, ids)
		// An engine that cannot read only the active torrents gets a whole read.
		if errors.Is(err, ErrNotSupported) {
			whole = true
		} else if err != nil {
			return nil, err
		}
	}
	if whole {
		jobs, err := e.List(ctx, "")
		if err != nil {
			return nil, err
		}
		jobs = inCategory(jobs, cats)
		next = make(map[string]Job, len(jobs))
		for _, j := range jobs {
			next[j.ID] = j
		}
		cur = &rtCursor{gen: gen, fullAt: now}
	} else {
		next = make(map[string]Job, len(cur.items)+len(got))
		for k, j := range cur.items {
			next[k] = j
		}
		for _, k := range ids {
			delete(next, k)
		}
		for _, j := range inCategory(got, cats) {
			next[j.ID] = j
		}
		cur = &rtCursor{gen: gen, fullAt: cur.fullAt}
	}
	cur.items = next
	c.mu.Lock()
	if c.m == nil {
		c.m = map[int64]*rtCursor{}
	}
	c.m[id] = cur
	c.mu.Unlock()
	out := make([]Job, 0, len(next))
	for _, j := range next {
		out = append(out, j)
	}
	return out, nil
}
