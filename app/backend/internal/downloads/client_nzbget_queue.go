package downloads

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const nzbgetOutside = "nzbid-"

var stepWord = map[string]string{"SUCCESS": "done", "FAILURE": "failed", "REPAIR_POSSIBLE": "can be repaired, not run",
	"MANUAL": "left for you to do", "SPACE": "stopped: the disk is full", "PASSWORD": "needs a password",
	"SCAN_SKIPPED": "skipped", "SCAN_FAILURE": "the NZB could not be read"}

var (
	_ QueueReader     = (*NZBGet)(nil)
	_ HistoryReader   = (*NZBGet)(nil)
	_ Controller      = (*NZBGet)(nil)
	_ Retrier         = (*NZBGet)(nil)
	_ UsenetInspector = (*NZBGet)(nil)
)

type nzbgetStatus struct {
	DownloadRate   int64  `json:"DownloadRate"`
	DownloadRateLo uint64 `json:"DownloadRateLo"`
	DownloadRateHi uint64 `json:"DownloadRateHi"`
}

// Queue returns the current queue snapshot.
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

// History returns finished jobs.
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

// Status returns the client state.
func (n *NZBGet) Status(ctx context.Context) (ClientStatus, error) {
	q, err := n.Queue(ctx)
	if err != nil {
		return ClientStatus{}, err
	}
	return ClientStatus{Speed: q.Head.Speed, TimeLeft: q.Head.TimeLeft}, nil
}

// SetSpeedLimit is not supported by this client.
func (n *NZBGet) SetSpeedLimit(context.Context, int64, int64) error { return ErrNotSupported }

// Retry requeues a failed job and returns the new id.
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

// JobStages returns the processing stages of a job.
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

// ServerStats returns usage numbers per news server.
func (n *NZBGet) ServerStats(ctx context.Context) (ServerStats, error) {
	var st nzbgetStatus
	if err := n.rpc(ctx, "status", nil, &st); err != nil {
		return ServerStats{}, err
	}
	return ServerStats{Servers: []ServerStat{}}, nil
}

func (s nzbgetStatus) rate() int64 {
	if r := nzbgetSize(s.DownloadRateLo, s.DownloadRateHi); r > 0 {
		return r
	}
	return max(s.DownloadRate, 0)
}

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

func (n *NZBGet) find(ctx context.Context, id string) (nzbgetMatch, bool, error) {
	num, ok := strings.CutPrefix(id, nzbgetOutside)
	if !ok {
		m, found, err := n.lookup(ctx, id, n.Category)
		for _, c := range n.Others {
			if !errors.Is(err, errNZBGetForeignMarker) {
				break
			}
			m, found, err = n.lookup(ctx, id, c)
		}
		return m, found, err
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
