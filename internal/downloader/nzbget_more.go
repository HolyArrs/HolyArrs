package downloader

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

// The rest of NZBGet for the Downloads page: every queue and history item
// (also the ones HolyArrs did not send), the client's speed and time left,
// one job's steps, and Try again on a failed job.
//
// A job HolyArrs sent is known by its marker parameter. Any other job is
// known as "nzbid-" plus NZBGet's own number, which the job keeps when it
// moves from the queue to the history.

const nzbgetOutside = "nzbid-"

// nzbgetStatus is the part of status() the page uses.
type nzbgetStatus struct {
	DownloadRate   int64  `json:"DownloadRate"`
	DownloadRateLo uint64 `json:"DownloadRateLo"`
	DownloadRateHi uint64 `json:"DownloadRateHi"`
}

func (s nzbgetStatus) rate() int64 {
	if r := nzbgetSize(s.DownloadRateLo, s.DownloadRateHi); r > 0 {
		return r
	}
	return max(s.DownloadRate, 0)
}

// anyID is the job's id: its HolyArrs marker, or nzbid-<number>.
func (r nzbgetRow) anyID() string {
	if id := nzbgetMarkerID(r.Parameters); id != "" {
		return id
	}
	if r.NZBID <= 0 {
		return ""
	}
	return nzbgetOutside + strconv.Itoa(r.NZBID)
}

func (r nzbgetRow) anyJob(history bool) Job {
	j := r.job(history)
	j.ID = r.anyID()
	return j
}

// find is lookup for any id. An nzbid- id is a job HolyArrs did not send:
// it is found by number whatever its category, and never when it carries
// a HolyArrs marker (that job answers to its marker).
func (n *NZBGet) find(ctx context.Context, id string) (nzbgetMatch, bool, error) {
	num, ok := strings.CutPrefix(id, nzbgetOutside)
	if !ok {
		return n.lookup(ctx, id, n.Category)
	}
	want, err := strconv.Atoi(num)
	if err != nil || want <= 0 {
		return nzbgetMatch{}, false, nil
	}
	active, history, err := n.rows(ctx)
	if err != nil {
		return nzbgetMatch{}, false, err
	}
	for _, r := range active {
		if r.NZBID == want && nzbgetMarkerID(r.Parameters) == "" {
			return nzbgetMatch{row: r}, true, nil
		}
	}
	for _, r := range history {
		if r.NZBID == want && nzbgetMarkerID(r.Parameters) == "" {
			return nzbgetMatch{row: r, history: true}, true, nil
		}
	}
	return nzbgetMatch{}, false, nil
}

// Queue reads every active job with the client's speed and time left. One
// overall rate is shared evenly by the jobs moving; a job's time left is
// its bytes left at that share.
func (n *NZBGet) Queue(ctx context.Context) (QueuePage, error) {
	var active []nzbgetRow
	if err := n.rpc(ctx, "listgroups", []any{0}, &active); err != nil {
		return QueuePage{}, err
	}
	var st nzbgetStatus
	if err := n.rpc(ctx, "status", nil, &st); err != nil {
		return QueuePage{}, err
	}
	rate := st.rate()
	p := QueuePage{Head: QueueHead{Speed: rate}, Jobs: make([]Job, 0, len(active))}
	moving, left := 0, int64(0)
	for i, r := range active {
		j := r.anyJob(false)
		if j.ID == "" {
			continue
		}
		j.QueuePos = i + 1
		if j.State == StateDownloading && j.Progress < 1 {
			moving++
		}
		if j.State != StatePaused {
			left += nzbgetSize(r.RemainingSizeLo, r.RemainingSizeHi)
		}
		p.Jobs = append(p.Jobs, j)
	}
	if rate > 0 {
		p.Head.TimeLeft = left / rate
		for i := range p.Jobs {
			j := &p.Jobs[i]
			if j.State == StateDownloading && j.Progress < 1 && moving > 0 {
				j.Speed = rate / int64(moving)
				if j.Speed > 0 {
					j.ETA = int64(float64(j.Size)*(1-j.Progress)) / j.Speed
				}
			}
		}
	}
	return p, nil
}

// History reads the whole visible history. NZBGet has no change counter,
// so every read is a full one (Changed, counter 0).
func (n *NZBGet) History(ctx context.Context, _ int64) (HistoryPage, error) {
	var history []nzbgetRow
	if err := n.rpc(ctx, "history", []any{false}, &history); err != nil {
		return HistoryPage{}, err
	}
	p := HistoryPage{Changed: true, Jobs: make([]Job, 0, len(history))}
	for _, r := range history {
		if j := r.anyJob(true); j.ID != "" {
			p.Jobs = append(p.Jobs, j)
		}
	}
	return p, nil
}

// Status is the client as a whole: speed and time left. NZBGet's free
// space and pause flag are not read (not in the adapter manual yet).
func (n *NZBGet) Status(ctx context.Context) (ClientStatus, error) {
	q, err := n.Queue(ctx)
	if err != nil {
		return ClientStatus{}, err
	}
	return ClientStatus{Speed: q.Head.Speed, TimeLeft: q.Head.TimeLeft}, nil
}

// The queue controls are not offered yet: their commands are not in the
// adapter manual. The server answers ErrNotSupported without a call.
func (n *NZBGet) PauseItem(context.Context, string) error           { return ErrNotSupported }
func (n *NZBGet) ResumeItem(context.Context, string) error          { return ErrNotSupported }
func (n *NZBGet) PauseAll(context.Context) error                    { return ErrNotSupported }
func (n *NZBGet) ResumeAll(context.Context) error                   { return ErrNotSupported }
func (n *NZBGet) MoveTop(context.Context, string) error             { return ErrNotSupported }
func (n *NZBGet) SetSpeedLimit(context.Context, int64, int64) error { return ErrNotSupported }

// Retry sends a failed history job back to the queue (HistoryRedownload).
// The job keeps its number and marker, so its id stays the same.
func (n *NZBGet) Retry(ctx context.Context, id string) (string, error) {
	m, found, err := n.find(ctx, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNotFound
	}
	if !m.history || m.row.job(true).State != StateFailed {
		return "", errors.New("only a failed job in NZBGet's history can be tried again")
	}
	var ok bool
	if err := n.rpc(ctx, "editqueue", []any{"HistoryRedownload", "", []int{m.row.NZBID}}, &ok); err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("NZBGet refused to try the job again")
	}
	return id, nil
}

// JobStages is the job's steps from its own status fields: download,
// verify and repair, unpack, move and script. NZBGet's per-job log is not
// read (not in the adapter manual yet).
func (n *NZBGet) JobStages(ctx context.Context, id string) ([]JobStage, error) {
	m, found, err := n.find(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return nzbgetStages(m.row, m.history), nil
}

// ServerStats: NZBGet's per-server counters are not in the adapter manual,
// so none are given. The status read keeps this a real check of the client.
func (n *NZBGet) ServerStats(ctx context.Context) (ServerStats, error) {
	var st nzbgetStatus
	if err := n.rpc(ctx, "status", nil, &st); err != nil {
		return ServerStats{}, err
	}
	return ServerStats{Servers: []ServerStat{}}, nil
}

var stepWord = map[string]string{"SUCCESS": "done", "FAILURE": "failed", "REPAIR_POSSIBLE": "can be repaired, not run",
	"MANUAL": "left for you to do", "SPACE": "stopped: the disk is full", "PASSWORD": "needs a password",
	"SCAN_SKIPPED": "skipped", "SCAN_FAILURE": "the NZB could not be read"}

func nzbgetStages(r nzbgetRow, history bool) []JobStage {
	line := func(what, v string) []string {
		if v == "" || v == "NONE" {
			return nil
		}
		w := stepWord[v]
		if w == "" {
			w = strings.ToLower(v)
		}
		return []string{what + ": " + w}
	}
	var out []JobStage
	add := func(stage string, lines ...[]string) {
		js := JobStage{Stage: stage, Lines: []string{}}
		for _, l := range lines {
			js.Lines = append(js.Lines, l...)
		}
		if len(js.Lines) > 0 {
			out = append(out, js)
		}
	}
	dl := []string{}
	if history {
		dl = append(dl, "Result: "+nzbgetReason(r))
	} else if r.Status != "" {
		dl = append(dl, "Now: "+strings.ToLower(strings.ReplaceAll(r.Status, "_", " ")))
	}
	add("download", dl, line("Fetching the NZB", r.URLStatus), line("Removed", deleteWord(r.DeleteStatus)), line("Marked", markWord(r.MarkStatus)))
	add("repair", line("Verify and repair", r.ParStatus), line("Extra files repair", r.ExParStatus))
	add("unpack", line("Unpack", r.UnpackStatus))
	add("move", line("Move to the finished folder", r.MoveStatus))
	add("script", line("Scripts", r.ScriptStatus))
	if out == nil {
		out = []JobStage{}
	}
	return out
}

func deleteWord(s string) string {
	switch s {
	case "MANUAL":
		return "by hand"
	case "HEALTH":
		return "too many parts missing"
	case "DUPE":
		return "a duplicate"
	case "BAD":
		return "marked bad"
	case "SCAN":
		return "the NZB could not be read"
	case "COPY":
		return "a copy is already there"
	}
	return s
}

func markWord(s string) string {
	switch s {
	case "GOOD":
		return "good"
	case "BAD":
		return "bad"
	}
	return s
}

// nzbgetReason is a history job's result in plain words, with NZBGet's
// own status after it.
func nzbgetReason(r nzbgetRow) string {
	why := ""
	switch {
	case r.MarkStatus == "BAD":
		why = "Marked bad in NZBGet"
	case r.DeleteStatus == "MANUAL":
		why = "Deleted in NZBGet by hand"
	case r.DeleteStatus == "HEALTH" || r.Status == "FAILURE/HEALTH":
		why = "Too many parts missing on the news servers"
	case r.DeleteStatus == "DUPE":
		why = "Deleted as a duplicate"
	case r.DeleteStatus == "COPY":
		why = "Deleted: a copy is already there"
	case r.DeleteStatus == "SCAN" || r.URLStatus == "SCAN_FAILURE" || r.Status == "FAILURE/SCAN":
		why = "The NZB could not be read"
	case r.URLStatus == "FAILURE":
		why = "Fetching the NZB failed"
	case r.UnpackStatus == "SPACE" || r.Status == "WARNING/SPACE":
		why = "Stopped: the disk is full"
	case r.UnpackStatus == "PASSWORD" || r.Status == "WARNING/PASSWORD":
		why = "Unpack needs a password"
	case r.ParStatus == "FAILURE" || r.ExParStatus == "FAILURE" || r.Status == "FAILURE/PAR" || r.Status == "WARNING/DAMAGED":
		why = "Damaged and could not be repaired"
	case r.ParStatus == "REPAIR_POSSIBLE" || r.Status == "WARNING/REPAIRABLE":
		why = "Can be repaired, but repair did not run"
	case r.UnpackStatus == "FAILURE" || r.Status == "FAILURE/UNPACK":
		why = "Unpack failed"
	case r.MoveStatus == "FAILURE" || r.Status == "FAILURE/MOVE":
		why = "Moving the files failed"
	case r.ScriptStatus == "FAILURE" || r.Status == "WARNING/SCRIPT":
		why = "A post-processing script failed"
	case r.Status == "WARNING/HEALTH":
		why = "Some parts were missing on the news servers"
	case strings.HasPrefix(r.Status, "SUCCESS/"):
		return "Done"
	}
	if why == "" {
		return r.Status
	}
	if r.Status == "" {
		return why
	}
	return why + " (" + r.Status + ")"
}

var (
	_ QueueReader     = (*NZBGet)(nil)
	_ HistoryReader   = (*NZBGet)(nil)
	_ Controller      = (*NZBGet)(nil)
	_ Retrier         = (*NZBGet)(nil)
	_ UsenetInspector = (*NZBGet)(nil)
)
