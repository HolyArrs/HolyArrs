package downloader

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Queue order and priority for Usenet jobs, and NZBGet's pause and resume.

// Queue moves one place at a time or to an end.
const (
	MoveUp     = "up"
	MoveDown   = "down"
	MoveBottom = "bottom"
)

// One priority set for every Usenet client.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityForce  = "force"
)

// ErrJobPriority is a priority that is not low, normal, high or force.
var ErrJobPriority = errors.New("priority is low, normal, high or force")

// ErrMove is a queue move that is not up, down or bottom.
var ErrMove = errors.New("move is up, down or bottom")

// UsenetQueue is the optional part of a Usenet engine that reorders its
// queue and sets a job's priority. MoveTop stays on Controller.
type UsenetQueue interface {
	MoveItem(ctx context.Context, id, where string) error
	SetPriority(ctx context.Context, id, priority string) error
}

// --- SABnzbd ---

// sabPlace finds a job's place in the queue and the queue's length.
func (s *Sabnzbd) sabPlace(ctx context.Context, id string) (int, int, error) {
	// Read the whole queue: with nzo_ids set, SABnzbd numbers the slots
	// within the filtered list, so the place would always be 0.
	b, err := s.call(ctx, "queue", url.Values{"start": {"0"}}, true)
	if err != nil {
		return 0, 0, err
	}
	var qu sabQueueFull
	if err := json.Unmarshal(b, &qu); err != nil {
		return 0, 0, err
	}
	total := len(qu.Queue.Slots)
	if qu.Queue.Total > total {
		total = qu.Queue.Total
	}
	for i, x := range qu.Queue.Slots {
		if x.ID == id {
			return i, total, nil
		}
	}
	return 0, 0, ErrNotFound
}

// MoveItem moves a job up, down or to the bottom (mode=switch).
func (s *Sabnzbd) MoveItem(ctx context.Context, id, where string) error {
	if id == "" || strings.Contains(id, ",") {
		return ErrNotFound
	}
	if where != MoveUp && where != MoveDown && where != MoveBottom {
		return ErrMove
	}
	at, total, err := s.sabPlace(ctx, id)
	if err != nil {
		return err
	}
	to := at
	switch where {
	case MoveUp:
		to = at - 1
	case MoveDown:
		to = at + 1
	case MoveBottom:
		to = total - 1
	}
	if to < 0 || (total > 0 && to >= total) || to == at {
		return nil // already at that end
	}
	_, err = s.call(ctx, "switch", url.Values{"value": {id}, "value2": {strconv.Itoa(to)}}, true)
	return err
}

var sabPriorities = map[string]string{PriorityLow: "-1", PriorityNormal: "0", PriorityHigh: "1", PriorityForce: "2"}

// SetPriority sets a job's priority (mode=queue name=priority).
func (s *Sabnzbd) SetPriority(ctx context.Context, id, priority string) error {
	v, ok := sabPriorities[priority]
	if !ok {
		return ErrJobPriority
	}
	if id == "" || strings.Contains(id, ",") {
		return ErrNotFound
	}
	_, err := s.call(ctx, "queue", url.Values{"name": {"priority"}, "value": {id}, "value2": {v}}, true)
	return err
}

// --- NZBGet ---

// edit runs one editqueue command on an active job.
func (n *NZBGet) edit(ctx context.Context, id, command, param string) error {
	m, found, err := n.find(ctx, id)
	if err != nil {
		return err
	}
	if !found || m.history {
		return ErrNotFound
	}
	var ok bool
	if err := n.rpc(ctx, "editqueue", []any{command, param, []int{m.row.NZBID}}, &ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("NZBGet refused the change")
	}
	return nil
}

// PauseItem pauses one job.
func (n *NZBGet) PauseItem(ctx context.Context, id string) error {
	return n.edit(ctx, id, "GroupPause", "")
}

// ResumeItem resumes one job.
func (n *NZBGet) ResumeItem(ctx context.Context, id string) error {
	return n.edit(ctx, id, "GroupResume", "")
}

func (n *NZBGet) simple(ctx context.Context, method string) error {
	var ok bool
	if err := n.rpc(ctx, method, nil, &ok); err != nil {
		return err
	}
	if !ok {
		return errors.New("NZBGet refused the change")
	}
	return nil
}

// PauseAll pauses downloading.
func (n *NZBGet) PauseAll(ctx context.Context) error { return n.simple(ctx, "pausedownload") }

// ResumeAll resumes downloading.
func (n *NZBGet) ResumeAll(ctx context.Context) error { return n.simple(ctx, "resumedownload") }

// MoveTop moves a job to the first place.
func (n *NZBGet) MoveTop(ctx context.Context, id string) error {
	return n.edit(ctx, id, "GroupMoveTop", "")
}

// MoveItem moves a job up, down or to the bottom.
func (n *NZBGet) MoveItem(ctx context.Context, id, where string) error {
	switch where {
	case MoveUp:
		return n.edit(ctx, id, "GroupMoveOffset", "-1")
	case MoveDown:
		return n.edit(ctx, id, "GroupMoveOffset", "1")
	case MoveBottom:
		return n.edit(ctx, id, "GroupMoveBottom", "")
	}
	return ErrMove
}

var nzbgetPriorities = map[string]string{PriorityLow: "-50", PriorityNormal: "0", PriorityHigh: "50", PriorityForce: "900"}

// SetPriority sets a job's priority.
func (n *NZBGet) SetPriority(ctx context.Context, id, priority string) error {
	v, ok := nzbgetPriorities[priority]
	if !ok {
		return ErrJobPriority
	}
	return n.edit(ctx, id, "GroupSetPriority", v)
}
