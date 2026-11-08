package downloads

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const ownMarker = "nzbget-1111111111111111111111111111111111111111111111111111111111111111"

type nzbAll struct {
	mu      sync.Mutex
	groups  []map[string]any
	history []map[string]any
	rate    int64
	calls   []string
	edits   [][]any
}

func (f *nzbAll) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in.Method)
	var result any
	switch in.Method {
	case "listgroups":
		result = f.groups
	case "history":
		result = f.history
	case "status":
		result = map[string]any{"DownloadRateLo": f.rate, "DownloadRateHi": 0, "PostJobCount": 0}
	case "listfiles":
		result = []map[string]any{}
	case "editqueue":
		var cmd string
		var ids []int
		_ = json.Unmarshal(in.Params[0], &cmd)
		_ = json.Unmarshal(in.Params[2], &ids)
		f.edits = append(f.edits, []any{cmd, ids})
		result = true
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"name": "JSONRPCError", "code": 3, "message": "no"}})
		return
	}
	if result == nil {
		result = []map[string]any{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "result": result})
}

func TestNZBGetQueueAndHistoryListEveryJob(t *testing.T) {
	_, n := nzbAllRig(t)
	ctx := context.Background()
	q, err := n.Queue(ctx)
	if err != nil || len(q.Jobs) != 4 {
		t.Fatalf("queue %v %+v", err, q)
	}
	by := map[string]Job{}
	for _, j := range q.Jobs {
		by[j.ID] = j
	}
	dune := by["nzbid-11"]
	if dune.State != StateDownloading || dune.Size != 1000<<20 || dune.Speed != 2<<20 || dune.ETA != 300 || dune.Category != "movies" || dune.QueuePos != 1 {
		t.Fatalf("dune %+v", dune)
	}
	if by["nzbid-12"].State != StatePaused || by["nzbid-13"].Detail != "REPAIRING" || by["nzbid-13"].Progress != 1 || by["nzbid-13"].Speed != 0 {
		t.Fatalf("paused/repair %+v %+v", by["nzbid-12"], by["nzbid-13"])
	}
	if _, ok := by[ownMarker]; !ok {
		t.Fatal("a HolyArrs job keeps its marker id")
	}
	if q.Head.Speed != 2<<20 || q.Head.TimeLeft != int64((600<<20)+10)/(2<<20) {
		t.Fatalf("head %+v", q.Head)
	}
	h, err := n.History(ctx, 0)
	if err != nil || !h.Changed || len(h.Jobs) != 3 {
		t.Fatalf("history %v %+v", err, h)
	}
	if j := h.Jobs[0]; j.ID != "nzbid-21" || j.State != StateFailed || j.Detail != "Marked bad in NZBGet (FAILURE/BAD)" || j.DoneAt != 1700000000 {
		t.Fatalf("bad %+v", j)
	}
	if j := h.Jobs[1]; j.State != StateFailed || !strings.HasPrefix(j.Detail, "Deleted in NZBGet by hand") {
		t.Fatalf("deleted %+v", j)
	}
	if j := h.Jobs[2]; j.State != StateFinished || j.ContentPath != "/data/done/Good" {
		t.Fatalf("good %+v", j)
	}
	st, err := n.Status(ctx)
	if err != nil || st.Speed != 2<<20 || st.TimeLeft != q.Head.TimeLeft {
		t.Fatalf("status %v %+v", err, st)
	}
}

func TestNZBGetOutsideIDsGetRemoveRetry(t *testing.T) {
	f, n := nzbAllRig(t)
	n.Category = "movies"
	ctx := context.Background()
	if j, err := n.Get(ctx, "nzbid-12"); err != nil || j.Name != "Arcane.S01E01.1080p" || j.ID != "nzbid-12" {
		t.Fatalf("get %v %+v", err, j)
	}
	for _, id := range []string{"nzbid-14", "nzbid-99", "nzbid-x", "nzbid--1"} {
		if _, err := n.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if _, err := n.Retry(ctx, "nzbid-23"); err == nil {
		t.Fatal("a finished job is not tried again")
	}
	if _, err := n.Retry(ctx, "nzbid-11"); err == nil {
		t.Fatal("a queued job is not tried again")
	}
	id, err := n.Retry(ctx, "nzbid-21")
	if err != nil || id != "nzbid-21" {
		t.Fatalf("retry %v %s", err, id)
	}
	if err := n.Remove(ctx, "nzbid-22", false); err != nil {
		t.Fatal(err)
	}
	if err := n.Remove(ctx, "nzbid-12", true); err != nil {
		t.Fatal(err)
	}
	want := []string{"HistoryRedownload 21", "HistoryDelete 22", "GroupFinalDelete 12"}
	if len(f.edits) != 3 {
		t.Fatalf("edits %v", f.edits)
	}
	for i, e := range f.edits {
		if got := e[0].(string) + " " + itoaInt(e[1].([]int)[0]); got != want[i] {
			t.Fatalf("edit %d: %s, want %s", i, got, want[i])
		}
	}
}

func TestNZBGetJobStages(t *testing.T) {
	f, n := nzbAllRig(t)
	f.history = append(f.history, map[string]any{"NZBID": 24, "NZBName": "Broken", "Status": "FAILURE/UNPACK", "ParStatus": "SUCCESS", "UnpackStatus": "FAILURE", "MoveStatus": "NONE", "ScriptStatus": "FAILURE"})
	st, err := n.JobStages(context.Background(), "nzbid-24")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, s := range st {
		got[s.Stage] = s.Lines
	}
	if got["download"][0] != "Result: Unpack failed (FAILURE/UNPACK)" || got["repair"][0] != "Verify and repair: done" || got["unpack"][0] != "Unpack: failed" ||
		got["script"][0] != "Scripts: failed" || got["move"] != nil {
		t.Fatalf("stages %+v", st)
	}
	if st, err := n.JobStages(context.Background(), "nzbid-13"); err != nil || len(st) != 1 || st[0].Lines[0] != "Now: repairing" {
		t.Fatalf("live %v %+v", err, st)
	}
	if _, err := n.JobStages(context.Background(), "nzbid-404"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	ss, err := n.ServerStats(context.Background())
	if err != nil || len(ss.Servers) != 0 {
		t.Fatalf("servers %v %+v", err, ss)
	}
}

func TestNZBGetReadOnlyWrapper(t *testing.T) {
	f, n := nzbAllRig(t)
	ro := readOnly(n)
	if _, err := ro.(Retrier).Retry(context.Background(), "nzbid-21"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := ro.(Controller).PauseItem(context.Background(), "nzbid-11"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := ro.Remove(context.Background(), "nzbid-11", true); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("writes reached NZBGet: %v", f.calls)
	}
	if q, err := ro.(QueueReader).Queue(context.Background()); err != nil || len(q.Jobs) != 4 {
		t.Fatalf("read %v", err)
	}
}

func nzbAllRig(t *testing.T) (*nzbAll, *NZBGet) {
	t.Helper()
	f := &nzbAll{rate: 2 << 20,
		groups: []map[string]any{
			{"NZBID": 11, "NZBName": "Dune.Part.Two.2024.2160p", "Category": "movies", "DestDir": "/data/inter/Dune", "FileSizeLo": 1000 << 20, "RemainingSizeLo": 600 << 20, "Status": "DOWNLOADING"},
			{"NZBID": 12, "NZBName": "Arcane.S01E01.1080p", "Category": "tv_shows", "FileSizeLo": 500 << 20, "RemainingSizeLo": 500 << 20, "Status": "PAUSED"},
			{"NZBID": 13, "NZBName": "Lab.Lanterns.FLAC", "Category": "music", "FileSizeLo": 24 << 20, "RemainingSizeLo": 0, "Status": "REPAIRING", "ParStatus": "NONE"},
			{"NZBID": 14, "NZBName": "Own.Movie", "Category": "movies", "FileSizeLo": 10, "RemainingSizeLo": 10, "Status": "QUEUED",
				"Parameters": []map[string]any{{"Name": nzbgetMarker, "Value": ownMarker}}},
		},
		history: []map[string]any{
			{"NZBID": 21, "NZBName": "Bad.One", "Category": "movies", "FileSizeLo": 100, "Status": "FAILURE/BAD", "MarkStatus": "BAD", "DeleteStatus": "MANUAL", "HistoryTime": 1700000000},
			{"NZBID": 22, "NZBName": "Gone.One", "Category": "tv_shows", "FileSizeLo": 100, "Status": "DELETED/MANUAL", "DeleteStatus": "MANUAL"},
			{"NZBID": 23, "NZBName": "Good.One", "Category": "music", "FileSizeLo": 100, "Status": "SUCCESS/UNPACK", "ParStatus": "SUCCESS", "UnpackStatus": "SUCCESS", "FinalDir": "/data/done/Good"},
		}}
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	return f, NewNZBGet(s.URL, "u", "p", nil)
}

func itoaInt(n int) string { b, _ := json.Marshal(n); return string(b) }
