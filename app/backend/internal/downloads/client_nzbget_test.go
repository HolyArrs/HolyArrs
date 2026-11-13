package downloads

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

var _ Engine = (*NZBGet)(nil)

type fakeNZBGet struct {
	mu              sync.Mutex
	user            string
	pass            string
	groups          []map[string]any
	history         []map[string]any
	files           map[int][]map[string]any
	adds            int
	appends         [][]any
	edits           []string
	rpcError        bool
	appendHTTPError bool
	appendRefusal   bool // append answers an RPC error and adds nothing
	hideAppend      bool
	hidden          []map[string]any
	config          []nzbParam // the options "config" answers
}

func (f *fakeNZBGet) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/jsonrpc" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	u, p, ok := r.BasicAuth()
	if !ok || u != f.user || p != f.pass {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var in struct {
		Version string            `json:"version"`
		Method  string            `json:"method"`
		Params  []json.RawMessage `json:"params"`
		ID      any               `json:"id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.Version == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if f.rpcError {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "id": in.ID,
			"error": map[string]any{"name": "JSONRPCError", "code": 401, "message": "password " + f.pass + " refused"}})
		return
	}
	result := any(nil)
	switch in.Method {
	case "version":
		result = "26.3"
	case "config":
		result = f.config
	case "listgroups":
		if f.groups == nil {
			result = []map[string]any{}
		} else {
			result = f.groups
		}
	case "history":
		if f.history == nil {
			result = []map[string]any{}
		} else {
			result = f.history
		}
	case "listfiles":
		var id int
		if len(in.Params) == 3 {
			_ = json.Unmarshal(in.Params[2], &id)
		}
		if f.files[id] == nil {
			result = []map[string]any{}
		} else {
			result = f.files[id]
		}
	case "append":
		if f.appendRefusal {
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "id": in.ID,
				"error": map[string]any{"name": "JSONRPCError", "code": 2, "message": "Invalid parameter (AddTop) " + f.pass}})
			return
		}
		var args []any
		for _, raw := range in.Params {
			var v any
			_ = json.Unmarshal(raw, &v)
			args = append(args, v)
		}
		f.appends = append(f.appends, args)
		f.adds++
		id := 100 + f.adds
		name, _ := args[0].(string)
		cat, _ := args[2].(string)
		var params []nzbParam
		if pairs, ok := args[10].([]any); ok {
			for _, pair := range pairs {
				if fields, ok := pair.([]any); ok && len(fields) == 2 {
					params = append(params, nzbParam{Name: fields[0].(string), Value: fields[1].(string)})
				}
			}
		}
		f.groups = append(f.groups, map[string]any{"NZBID": id, "NZBFilename": name, "NZBName": strings.TrimSuffix(name, ".nzb"),
			"Category": cat, "DestDir": "/downloads/" + cat, "FileSizeLo": 1024, "FileSizeHi": 0,
			"RemainingSizeLo": 1024, "RemainingSizeHi": 0, "Status": "QUEUED", "Parameters": params})
		if f.hideAppend {
			f.hidden = append(f.hidden, f.groups[len(f.groups)-1])
			f.groups = f.groups[:len(f.groups)-1]
		}
		if f.appendHTTPError {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		result = id
	case "editqueue":
		var command string
		var ids []int
		_ = json.Unmarshal(in.Params[0], &command)
		_ = json.Unmarshal(in.Params[2], &ids)
		f.edits = append(f.edits, command)
		for _, id := range ids {
			if strings.HasPrefix(command, "Group") {
				f.groups = slices.DeleteFunc(f.groups, func(x map[string]any) bool { return int(x["NZBID"].(int)) == id })
			} else {
				f.history = slices.DeleteFunc(f.history, func(x map[string]any) bool { return int(x["NZBID"].(int)) == id })
			}
		}
		result = true
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "id": in.ID,
			"error": map[string]any{"name": "JSONRPCError", "code": 3, "message": "unsupported action"}})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "id": in.ID, "result": result})
}

func TestNZBGetConformance(t *testing.T) { runConformance(t, nzbgetHarness(t)) }

func TestNZBGetAddUploadsBytesAndKeepsStableIdentity(t *testing.T) {
	f, base := startNZBGet(t)
	r := AddRequest{URL: nzbSource(t), Name: "Case.Sensitive.Name", Category: "movies", OperationKey: "downloader/release/slot", AllowLocal: true}
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), r)
	if err != nil || id == "" || id == "101" {
		t.Fatalf("add id %q: %v", id, err)
	}
	f.mu.Lock()
	args := append([]any(nil), f.appends[0]...)
	f.mu.Unlock()
	if len(args) != 11 || args[2] != "movies" || args[3] != float64(0) || args[4] != false || args[5] != false || args[8] != "SCORE" || args[9] != false {
		t.Fatalf("append positions: %#v", args)
	}
	pairs, ok := args[10].([]any)
	if !ok || len(pairs) != 1 || len(pairs[0].([]any)) != 2 || pairs[0].([]any)[0] != nzbgetMarker || pairs[0].([]any)[1] != id {
		t.Fatalf("append marker: %#v", args[10])
	}
	b, err := base64.StdEncoding.DecodeString(args[1].(string))
	if err != nil || !strings.Contains(string(b), "<nzb>") {
		t.Fatalf("append content %q: %v", args[1], err)
	}
	got, err := NewNZBGet(base, f.user, f.pass, nil).Get(context.Background(), id)
	if err != nil || got.ID != id {
		t.Fatalf("restart get %+v: %v", got, err)
	}
	f.mu.Lock()
	g := f.groups[0]
	f.groups = nil
	g["NZBID"], g["Status"], g["FinalDir"] = 901, "SUCCESS/UNPACK", "/downloads/movies/Case.Sensitive.Name"
	g["UnpackStatus"], g["MoveStatus"] = "SUCCESS", "SUCCESS"
	f.history = []map[string]any{g}
	f.mu.Unlock()
	got, err = NewNZBGet(base, f.user, f.pass, nil).Get(context.Background(), id)
	if err != nil || got.ID != id || got.State != StateFinished || got.ContentPath == "" {
		t.Fatalf("history get %+v: %v", got, err)
	}

	r.Name = "case.sensitive.name"
	again, err := n.Add(context.Background(), r)
	if err != nil || again != id || f.adds != 1 {
		t.Fatalf("duplicate %q %v adds=%d", again, err, f.adds)
	}
	r.Skip = map[string]bool{id: true}
	next, err := n.Add(context.Background(), r)
	if err != nil || next == id || f.adds != 2 {
		t.Fatalf("skipped duplicate %q %v adds=%d", next, err, f.adds)
	}
}

func TestNZBGetFallbackIdentityCaseFoldsNameAndCategory(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	r := AddRequest{URL: nzbSource(t), Name: "Fallback.Name", Category: "Movies", AllowLocal: true}
	id, err := n.Add(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	r.Name, r.Category = "fallback.name", "movies"
	again, err := n.Add(context.Background(), r)
	if err != nil || again != id || f.adds != 1 {
		t.Fatalf("case-folded fallback %q: %v adds=%d", again, err, f.adds)
	}
}

func TestNZBGetReconcilesAnAcceptedAppendWithALostResponse(t *testing.T) {
	f, base := startNZBGet(t)
	f.appendHTTPError = true
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "ambiguous", Category: "movies", OperationKey: "ambiguous", AllowLocal: true})
	if err != nil || id == "" || f.adds != 1 {
		t.Fatalf("ambiguous append %q: %v adds=%d", id, err, f.adds)
	}
	again, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "ambiguous", Category: "movies", OperationKey: "ambiguous", AllowLocal: true})
	if err != nil || again != id || f.adds != 1 {
		t.Fatalf("retry %q: %v adds=%d", again, err, f.adds)
	}
}

func TestNZBGetUncertainAppendOnlyReconcilesItsDurableMarker(t *testing.T) {
	f, base := startNZBGet(t)
	f.appendHTTPError, f.hideAppend = true, true
	r := AddRequest{URL: nzbSource(t), Name: "uncertain", Category: "movies", AllowLocal: true,
		AttemptMarker: "nzbget-opaque-durable-attempt"}
	n := NewNZBGet(base, f.user, f.pass, nil)
	if _, err := n.Add(context.Background(), r); !errors.Is(err, ErrUncertain) || f.adds != 1 {
		t.Fatalf("unknown append: %v adds=%d", err, f.adds)
	}
	n = NewNZBGet(base, f.user, f.pass, nil)
	r.ReconcileOnly = true
	for range 2 {
		if _, err := n.Add(context.Background(), r); !errors.Is(err, ErrUncertain) || f.adds != 1 {
			t.Fatalf("empty reconciliation replayed append: %v adds=%d", err, f.adds)
		}
	}
	f.mu.Lock()
	f.groups, f.hidden = f.hidden, nil
	f.mu.Unlock()
	if id, err := n.Add(context.Background(), r); err != nil || id != r.AttemptMarker || f.adds != 1 {
		t.Fatalf("late positive reconciliation: %v adds=%d", err, f.adds)
	}
	if f.appends[0][10].([]any)[0].([]any)[1] != r.AttemptMarker {
		t.Fatal("durable marker was not sent unchanged")
	}
}

func TestNZBGetFailsClosedOnDuplicateMarkers(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	r := AddRequest{URL: nzbSource(t), Name: "duplicate-marker", Category: "movies", OperationKey: "duplicate-marker", AllowLocal: true}
	id, err := n.Add(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	h := map[string]any{}
	for k, v := range f.groups[0] {
		h[k] = v
	}
	h["NZBID"], h["Status"], h["FinalDir"] = 909, "SUCCESS/ALL", "/downloads/movies/duplicate-marker"
	f.history = []map[string]any{h}
	f.mu.Unlock()

	if _, err := n.List(context.Background(), "movies"); !errors.Is(err, errNZBGetAmbiguousMarker) {
		t.Fatalf("list ambiguity: %v", err)
	}
	if _, err := n.Get(context.Background(), id); !errors.Is(err, errNZBGetAmbiguousMarker) {
		t.Fatalf("get ambiguity: %v", err)
	}
	if _, err := n.Add(context.Background(), r); !errors.Is(err, errNZBGetAmbiguousMarker) || f.adds != 1 {
		t.Fatalf("add ambiguity: %v adds=%d", err, f.adds)
	}
	if err := n.Remove(context.Background(), id, true); !errors.Is(err, errNZBGetAmbiguousMarker) || len(f.edits) != 0 {
		t.Fatalf("remove ambiguity: %v edits=%v", err, f.edits)
	}
}

func TestNZBGetDedupesQueueHistoryTransitionByNativeID(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	n.Category = "movies"
	r := AddRequest{URL: nzbSource(t), Name: "transition", Category: "movies", OperationKey: "transition", AllowLocal: true}
	id, err := n.Add(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	h := map[string]any{}
	for k, v := range f.groups[0] {
		h[k] = v
	}
	h["Status"], h["FinalDir"] = "SUCCESS/ALL", "/downloads/movies/transition"
	f.history = []map[string]any{h}
	f.mu.Unlock()

	jobs, err := n.List(context.Background(), "movies")
	if err != nil || len(jobs) != 1 || jobs[0].State != StateFinished {
		t.Fatalf("transition list = %+v, %v", jobs, err)
	}
	job, err := n.Get(context.Background(), id)
	if err != nil || job.State != StateFinished {
		t.Fatalf("transition get = %+v, %v", job, err)
	}
	reconciled, err := n.Add(context.Background(), AddRequest{Name: r.Name, Category: r.Category, AttemptMarker: id, ReconcileOnly: true})
	if err != nil || reconciled != id || f.adds != 1 {
		t.Fatalf("transition reconcile = %q, %v adds=%d", reconciled, err, f.adds)
	}
}

func TestNZBGetTransitionKeepsARenamedCategoryRow(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	n.Category = "movies"
	r := AddRequest{URL: nzbSource(t), Name: "transition-foreign", Category: "movies", OperationKey: "transition-foreign", AllowLocal: true}
	id, err := n.Add(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	h := map[string]any{}
	for k, v := range f.groups[0] {
		h[k] = v
	}
	h["Category"], h["Status"] = "music", "SUCCESS/ALL"
	f.history = []map[string]any{h}
	f.mu.Unlock()
	// The marker is ours, so a category change on the way to history keeps the job.
	if j, err := n.Get(context.Background(), id); err != nil || j.State != StateFinished {
		t.Fatalf("transition renamed get = %+v, %v", j, err)
	}
	if jobs, err := n.List(context.Background(), "movies"); err != nil || len(jobs) != 1 {
		t.Fatalf("transition renamed list = %+v, %v", jobs, err)
	}
}

// NZBGet renames the category of a job when the category has an Alias (lab:
// tv becomes Series). The HolyArrsID marker still makes the job ours.
func TestNZBGetAdoptsItsMarkerAfterACategoryAliasRename(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	n.Category = "tv"
	r := AddRequest{URL: nzbSource(t), Name: "renamed", Category: "tv", OperationKey: "renamed", AllowLocal: true}
	id, err := n.Add(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.groups[0]["Category"] = "Series"
	f.groups = append(f.groups, map[string]any{"NZBID": 9001, "NZBName": "other-app", "Category": "Series", "Status": "QUEUED"})
	f.mu.Unlock()

	// Queue.
	jobs, err := n.List(context.Background(), "tv")
	if err != nil || len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("queue list = %+v, %v", jobs, err)
	}
	if j, err := n.Get(context.Background(), id); err != nil || j.ID != id {
		t.Fatalf("queue get = %+v, %v", j, err)
	}
	// Same send again: no second copy.
	if got, err := n.Add(context.Background(), r); err != nil || got != id || f.adds != 1 {
		t.Fatalf("resend = %q, %v adds=%d", got, err, f.adds)
	}
	// Unknown-send recheck.
	if got, err := n.Add(context.Background(), AddRequest{Name: r.Name, Category: "tv", AttemptMarker: id, ReconcileOnly: true}); err != nil || got != id {
		t.Fatalf("recheck = %q, %v", got, err)
	}

	// History (where imports read the finished job).
	f.mu.Lock()
	h := f.groups[0]
	f.groups = f.groups[1:]
	h["NZBID"], h["Status"], h["FinalDir"] = 812, "SUCCESS/ALL", "/downloads/Series/renamed"
	f.history = []map[string]any{h}
	f.mu.Unlock()
	jobs, err = n.List(context.Background(), "tv")
	if err != nil || len(jobs) != 1 || jobs[0].ID != id || jobs[0].State != StateFinished {
		t.Fatalf("history list = %+v, %v", jobs, err)
	}
	j, err := n.Get(context.Background(), id)
	if err != nil || j.State != StateFinished || j.ContentPath != "/downloads/Series/renamed" {
		t.Fatalf("history get = %+v, %v", j, err)
	}
	if got, err := n.Add(context.Background(), AddRequest{Name: r.Name, Category: "tv", AttemptMarker: id, ReconcileOnly: true}); err != nil || got != id {
		t.Fatalf("history recheck = %q, %v", got, err)
	}
}

func TestNZBGetRejectsAnythingButAnNZBDocument(t *testing.T) {
	for name, body := range map[string]string{
		"html":     "<html><body>sign in</body></html>",
		"plain":    "not an nzb",
		"external": `<!DOCTYPE nzb [<!ENTITY remote SYSTEM "http://example.invalid/x">]><nzb>&remote;</nzb>`,
	} {
		t.Run(name, func(t *testing.T) {
			f, base := startNZBGet(t)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer s.Close()
			_, err := NewNZBGet(base, f.user, f.pass, nil).Add(context.Background(), AddRequest{URL: s.URL, Name: name, Category: "movies", OperationKey: name, AllowLocal: true})
			if err == nil || f.adds != 0 {
				t.Fatalf("invalid document: %v adds=%d", err, f.adds)
			}
		})
	}
}

func TestNZBGetFinalAndPostProcessStates(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "states", Category: "movies", OperationKey: "states", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.groups[0]["Status"], f.groups[0]["RemainingSizeLo"] = "UNPACKING", 0
	f.mu.Unlock()
	j, err := n.Get(context.Background(), id)
	if err != nil || j.State != StateDownloading || j.Progress != 1 {
		t.Fatalf("post process %+v %v", j, err)
	}
	f.mu.Lock()
	h := f.groups[0]
	f.groups = nil
	h["NZBID"], h["Status"], h["FinalDir"] = 777, "SUCCESS/UNPACK", "/downloads/movies/states"
	h["UnpackStatus"], h["MoveStatus"] = "FAILURE", "SUCCESS"
	f.history = []map[string]any{h}
	f.mu.Unlock()
	j, err = n.Get(context.Background(), id)
	if err != nil || j.State != StateFailed || !strings.Contains(j.Detail, "SUCCESS/UNPACK") {
		t.Fatalf("failed component %+v %v", j, err)
	}
	f.mu.Lock()
	h["UnpackStatus"], h["Status"] = "SPACE", "WARNING/SPACE"
	f.mu.Unlock()
	j, err = n.Get(context.Background(), id)
	if err != nil || j.State != StatePaused || !strings.Contains(j.Detail, "SPACE") {
		t.Fatalf("disk full must not fail %+v %v", j, err)
	}
	f.mu.Lock()
	h["UnpackStatus"], h["Status"], h["FinalDir"] = "SUCCESS", "SUCCESS/UNPACK", ""
	f.mu.Unlock()
	j, err = n.Get(context.Background(), id)
	if err != nil || j.State != StateFinished || j.ContentPath != "/downloads/movies" {
		t.Fatalf("destination fallback %+v %v", j, err)
	}
}

func TestNZBGetRemoveKeepsFilesWithNonFinalDelete(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "keep", Category: "movies", OperationKey: "keep", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Remove(context.Background(), id, false); err != nil || !slices.Equal(f.edits, []string{"GroupParkDelete"}) {
		t.Fatalf("keep-files removal: %v edits=%v", err, f.edits)
	}
	if err := n.Remove(context.Background(), id, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}
}

func TestNZBGetRemovesHistoryWithTheHistoryCommand(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "done", Category: "movies", OperationKey: "done", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	h := f.groups[0]
	f.groups = nil
	h["NZBID"], h["Status"], h["FinalDir"] = 808, "SUCCESS/ALL", "/downloads/movies/done"
	f.history = []map[string]any{h}
	f.mu.Unlock()
	if err := n.Remove(context.Background(), id, true); err != nil || !slices.Equal(f.edits, []string{"HistoryFinalDelete"}) {
		t.Fatalf("history removal: %v edits=%v", err, f.edits)
	}
}

func TestNZBGetRemoveFollowsTheMarkerAcrossCategories(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	id, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "owned", Category: "music", OperationKey: "owned", AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	n.Category = "movies"
	// The marker is ours, so a job in another category is still ours to remove.
	if err := n.Remove(context.Background(), id, true); err != nil || len(f.edits) != 1 {
		t.Fatalf("cross-category removal: %v edits=%v", err, f.edits)
	}
}

func TestNZBGetDoesNotBypassTheSourceNetworkPermission(t *testing.T) {
	f, base := startNZBGet(t)
	n := NewNZBGet(base, f.user, f.pass, nil)
	_, err := n.Add(context.Background(), AddRequest{URL: nzbSource(t), Name: "local", Category: "movies", OperationKey: "local"})
	if err == nil || f.adds != 0 {
		t.Fatalf("local source without permission: %v adds=%d", err, f.adds)
	}
}

func TestNZBGetRPCErrorsAreAuthAndSecretSafe(t *testing.T) {
	f, base := startNZBGet(t)
	f.rpcError = true
	_, err := NewNZBGet(base, f.user, f.pass, nil).Health(context.Background())
	if !errors.Is(err, ErrAuth) || strings.Contains(err.Error(), f.pass) {
		t.Fatalf("rpc auth error: %v", err)
	}
}

func startNZBGet(t *testing.T) (*fakeNZBGet, string) {
	t.Helper()
	f := &fakeNZBGet{user: "holyarrs", pass: "nzbget-test-secret", files: map[int][]map[string]any{}}
	s := httptest.NewServer(f)
	t.Cleanup(s.Close)
	return f, s.URL
}

func nzbSource(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<?xml version=\"1.0\"?><nzb></nzb>"))
	}))
	t.Cleanup(s.Close)
	return s.URL + "/owned.nzb"
}

func nzbgetHarness(t *testing.T) conformance {
	f, base := startNZBGet(t)
	url := nzbSource(t)
	release := AddRequest{URL: url, Name: "Lab.Usenet.Movie.2026.1080p.WEB.H264-HOLYARRS", Category: "movies",
		OperationKey: "download-1/release-1/slot-1", AllowLocal: true}
	return conformance{
		name:    "nzbget",
		engine:  NewNZBGet(base, f.user, f.pass, nil),
		badKey:  NewNZBGet(base, f.user, "wrong", nil),
		at:      func(b string) Engine { return NewNZBGet(b, f.user, f.pass, nil) },
		secret:  f.pass,
		release: release,
		unknown: "nzbget-0000000000000000000000000000000000000000000000000000000000000000",
		appear: func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			g := f.groups[0]
			g["Status"], g["RemainingSizeLo"] = "DOWNLOADING", 512
			id := g["NZBID"].(int)
			f.files[id] = []map[string]any{{"NZBID": id, "Filename": "/downloads/movies/movie.mkv", "FileSizeLo": 1024, "FileSizeHi": 0,
				"RemainingSizeLo": 512, "RemainingSizeHi": 0, "Progress": 500}}
		},
		adds:   func() int { f.mu.Lock(); defer f.mu.Unlock(); return f.adds },
		states: func(word string) { f.mu.Lock(); defer f.mu.Unlock(); f.groups[0]["Status"] = word },
		words: map[string]string{"QUEUED": StateQueued, "PAUSED": StatePaused, "DOWNLOADING": StateDownloading,
			"FETCHING": StateQueued, "PP_QUEUED": StateQueued, "UNPACKING": StateDownloading,
			"PP_FINISHED": StateDownloading, "brand-new-word": StateDownloading},
	}
}
