package downloads

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeAria2 is an aria2 serving JSON-RPC, with the quirks measured on 1.36.0:
// every fault is HTTP 400 with code 1, unknown options are dropped, a second
// add of a live hash makes a new job in error 12, and removes never delete
// files.
type fakeAria2 struct {
	mu       sync.Mutex
	secret   string
	version  string
	features []string
	opts     map[string]string
	jobs     []*a2Job
	next     int
	adds     []a2Sent
	calls    []string
	path     string

	loseAnswer bool   // keep the add but cut the connection before answering
	dropAdd    bool   // answer the add with a GID that never shows
	tooLarge   bool   // cut the connection on an add, as for a body over the size limit
	hideFor    int    // tellStatus answers "not found" this many times
	addError   string // a fresh add goes straight to this error code
	addHash    string // a fresh add carries this hash instead of its own
	capsIgnore bool   // changeGlobalOption answers OK and keeps nothing
	cuts       int    // connections cut

	optionFault bool // getOption answers with a fault
}

// a2Sent is one add as it reached the fake.
type a2Sent struct {
	method string
	uris   []string
	opts   map[string]any
}

type a2Job struct {
	gid, status, hash, dir, name, mode string
	total, done, up, down, upSpeed     int64
	conns, seeds                       int64
	seeder                             bool
	errCode, errMsg                    string
	followedBy                         []string
	following                          string
	files                              []a2FakeFile
}

type a2FakeFile struct {
	path         string
	length, done int64
	notSelected  bool
}

var a2NameRe = regexp.MustCompile(`4:name(\d+):`)
var a2LengthRe = regexp.MustCompile(`6:lengthi(\d+)e`)

func startAria2(t *testing.T, f *fakeAria2) string {
	t.Helper()
	if f.version == "" {
		f.version = "1.36.0"
	}
	if f.features == nil {
		f.features = []string{"Async DNS", "BitTorrent", "GZip", "HTTPS", "Message Digest", "Metalink", "XML-RPC"}
	}
	if f.opts == nil {
		f.opts = map[string]string{}
	}
	if _, ok := f.opts["dir"]; !ok {
		f.opts["dir"] = "/downloads"
	}
	for _, k := range []string{"max-overall-download-limit", "max-overall-upload-limit"} {
		if _, ok := f.opts[k]; !ok {
			f.opts[k] = "0"
		}
	}
	for _, k := range []string{"check-integrity", "bt-seed-unverified"} {
		if _, ok := f.opts[k]; !ok {
			f.opts[k] = "false"
		}
	}
	if f.path == "" {
		f.path = "/jsonrpc"
	}
	s := httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(s.Close)
	return s.URL
}

type a2FakeFault struct{ msg string }

func (f *fakeAria2) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != f.path {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var req struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params []any  `json:"params"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"id":null,"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error."}}`)
		return
	}
	f.mu.Lock()
	cut := (f.tooLarge || f.loseAnswer) && strings.HasPrefix(req.Method, "aria2.add")
	var v any
	var fault *a2FakeFault
	if !(f.tooLarge && cut) {
		v, fault = f.dispatch(req.Method, req.Params)
	}
	f.mu.Unlock()
	if cut {
		f.mu.Lock()
		f.cuts++
		f.mu.Unlock()
		if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
			_ = conn.Close()
		}
		return
	}
	out := map[string]any{"id": req.ID, "jsonrpc": "2.0"}
	status := http.StatusOK
	if fault != nil {
		out["error"] = map[string]any{"code": 1, "message": fault.msg}
		status = http.StatusBadRequest
	} else {
		out["result"] = v
	}
	w.Header().Set("Content-Type", "application/json-rpc")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(out)
}

// dispatch answers one call. The lock is held.
func (f *fakeAria2) dispatch(method string, params []any) (any, *a2FakeFault) {
	f.calls = append(f.calls, method)
	if method == "system.multicall" {
		list, _ := params[0].([]any)
		out := []any{}
		for _, c := range list {
			mc, _ := c.(map[string]any)
			name, _ := mc["methodName"].(string)
			ps, _ := mc["params"].([]any)
			v, fault := f.dispatch(name, ps)
			if fault != nil {
				out = append(out, map[string]any{"code": 1, "message": fault.msg})
			} else {
				out = append(out, []any{v})
			}
		}
		return out, nil
	}
	if !strings.HasPrefix(method, "system.") {
		tok := ""
		if len(params) > 0 {
			if s, ok := params[0].(string); ok && strings.HasPrefix(s, "token:") {
				tok, params = strings.TrimPrefix(s, "token:"), params[1:]
			}
		}
		if f.secret != "" && tok != f.secret {
			return nil, &a2FakeFault{"Unauthorized"}
		}
	}
	str := func(i int) string {
		if i < len(params) {
			s, _ := params[i].(string)
			return s
		}
		return ""
	}
	keys := func(i int) []string {
		if i >= len(params) {
			return nil
		}
		list, _ := params[i].([]any)
		out := []string{}
		for _, k := range list {
			s, _ := k.(string)
			out = append(out, s)
		}
		return out
	}
	switch method {
	case "aria2.getVersion":
		return map[string]any{"version": f.version, "enabledFeatures": f.features}, nil
	case "aria2.getGlobalOption":
		return f.opts, nil
	case "aria2.changeGlobalOption":
		m, _ := params[0].(map[string]any)
		if !f.capsIgnore {
			for k, v := range m {
				f.opts[k], _ = v.(string)
			}
		}
		return "OK", nil
	case "aria2.getGlobalStat":
		n := map[string]int{}
		for _, j := range f.jobs {
			switch j.status {
			case "active":
				n["a"]++
			case "waiting", "paused":
				n["w"]++
			default:
				n["s"]++
			}
		}
		return map[string]any{"downloadSpeed": "300", "uploadSpeed": "40", "numActive": strconv.Itoa(n["a"]), "numWaiting": strconv.Itoa(n["w"]), "numStopped": strconv.Itoa(n["s"]), "numStoppedTotal": strconv.Itoa(n["s"])}, nil
	case "aria2.tellActive":
		return f.list(func(j *a2Job) bool { return j.status == "active" }, keys(0)), nil
	case "aria2.tellWaiting":
		return f.list(func(j *a2Job) bool { return j.status == "waiting" || j.status == "paused" }, keys(2)), nil
	case "aria2.tellStopped":
		return f.list(func(j *a2Job) bool { return j.status == "complete" || j.status == "error" || j.status == "removed" }, keys(2)), nil
	case "aria2.addTorrent", "aria2.addUri":
		return f.add(method, params)
	case "aria2.pauseAll", "aria2.unpauseAll", "aria2.forcePauseAll":
		return "OK", nil
	}
	gid := str(0)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(gid) {
		return nil, &a2FakeFault{"Invalid GID " + gid}
	}
	j := f.job(gid)
	if j == nil || method == "aria2.tellStatus" && f.hideFor > 0 {
		if j != nil {
			f.hideFor--
		}
		return nil, &a2FakeFault{"GID " + gid + " is not found"}
	}
	switch method {
	case "aria2.tellStatus":
		return f.status(j, keys(1)), nil
	case "aria2.getOption":
		if f.optionFault {
			return nil, &a2FakeFault{"No such download for GID#" + gid}
		}
		out := map[string]string{}
		for k, v := range f.opts {
			out[k] = v
		}
		out["dir"] = j.dir
		return out, nil
	case "aria2.forcePause", "aria2.pause":
		if j.status != "active" && j.status != "waiting" {
			return nil, &a2FakeFault{"GID#" + gid + " cannot be paused now"}
		}
		j.status = "paused"
		return gid, nil
	case "aria2.unpause":
		if j.status != "paused" {
			return nil, &a2FakeFault{"GID#" + gid + " cannot be unpaused now"}
		}
		j.status = "active"
		return gid, nil
	case "aria2.forceRemove", "aria2.remove":
		switch j.status {
		case "paused", "waiting":
			f.drop(gid)
		case "active":
			if j.seeder || j.total > 0 && j.done == j.total {
				j.status, j.errCode, j.seeder = "complete", "0", false
			} else {
				j.status, j.errCode = "removed", "31"
			}
		default:
			return nil, &a2FakeFault{"Active Download not found for GID#" + gid}
		}
		return gid, nil
	case "aria2.removeDownloadResult":
		if j.status == "active" || j.status == "waiting" || j.status == "paused" {
			return nil, &a2FakeFault{"Could not remove download result of GID#" + gid}
		}
		f.drop(gid)
		return "OK", nil
	}
	return nil, &a2FakeFault{"No such method: " + method}
}

func (f *fakeAria2) add(method string, params []any) (any, *a2FakeFault) {
	opts := map[string]any{}
	sent := a2Sent{method: method, opts: opts}
	j := &a2Job{status: "active", dir: f.opts["dir"], mode: "single"}
	if method == "aria2.addTorrent" {
		raw, _ := params[0].(string)
		b, err := base64.StdEncoding.DecodeString(raw)
		h, herr := torrentFileHash(b)
		if err != nil || herr != nil {
			return nil, &a2FakeFault{"Bencode decoding failed"}
		}
		if m := a2NameRe.FindSubmatchIndex(b); m != nil {
			n, _ := strconv.Atoi(string(b[m[2]:m[3]]))
			j.name = string(b[m[1] : m[1]+n])
		}
		if m := a2LengthRe.FindSubmatch(b); m != nil {
			j.total, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		j.hash = h
		if len(params) > 2 {
			opts, _ = params[2].(map[string]any)
		}
	} else {
		list, _ := params[0].([]any)
		for _, u := range list {
			s, _ := u.(string)
			sent.uris = append(sent.uris, s)
		}
		if len(sent.uris) != 1 {
			return nil, &a2FakeFault{"No URI to download."}
		}
		h, ok := magnetHash(sent.uris[0])
		if !ok {
			return nil, &a2FakeFault{"No URI to download."}
		}
		j.hash = h
		dn := h
		if u, err := url.Parse(sent.uris[0]); err == nil && u.Query().Get("dn") != "" {
			dn = u.Query().Get("dn")
		}
		j.files = []a2FakeFile{{path: "[METADATA]" + dn}}
		if len(params) > 1 {
			opts, _ = params[1].(map[string]any)
		}
	}
	sent.opts = opts
	f.adds = append(f.adds, sent)
	if d, ok := opts["dir"].(string); ok {
		j.dir = d
	}
	if j.name != "" {
		j.files = []a2FakeFile{{path: j.dir + "/" + j.name, length: j.total}}
	}
	if f.addHash != "" {
		j.hash = f.addHash
	}
	f.next++
	j.gid = fmt.Sprintf("%016x", 0xa000+f.next)
	for _, o := range f.jobs {
		if o.hash == j.hash && (o.status == "active" || o.status == "waiting" || o.status == "paused") {
			j.status, j.errCode, j.errMsg = "error", "12", "InfoHash "+j.hash+" is already registered."
		}
	}
	if f.addError != "" && j.status != "error" {
		j.status, j.errCode, j.errMsg = "error", f.addError, "File "+j.dir+"/"+j.name+" exists, but a control file(*.aria2) does not exist."
		if f.addError == "12" {
			j.errMsg = "InfoHash " + j.hash + " is already registered."
		}
	}
	if !f.dropAdd {
		f.jobs = append(f.jobs, j)
	}
	return j.gid, nil
}

func (f *fakeAria2) job(gid string) *a2Job {
	for _, j := range f.jobs {
		if j.gid == gid {
			return j
		}
	}
	return nil
}

func (f *fakeAria2) drop(gid string) {
	for i, j := range f.jobs {
		if j.gid == gid {
			f.jobs = append(f.jobs[:i], f.jobs[i+1:]...)
			return
		}
	}
}

func (f *fakeAria2) list(ok func(*a2Job) bool, keys []string) []any {
	out := []any{}
	for _, j := range f.jobs {
		if ok(j) {
			out = append(out, f.status(j, keys))
		}
	}
	return out
}

// status is a job as tellStatus reports it, with only the asked keys.
func (f *fakeAria2) status(j *a2Job, keys []string) map[string]any {
	n := func(v int64) string { return strconv.FormatInt(v, 10) }
	files := []any{}
	for i, fl := range j.files {
		files = append(files, map[string]any{"index": n(int64(i + 1)), "path": fl.path, "length": n(fl.length), "completedLength": n(fl.done),
			"selected": strconv.FormatBool(!fl.notSelected), "uris": []any{}})
	}
	m := map[string]any{"gid": j.gid, "status": j.status, "dir": j.dir, "totalLength": n(j.total), "completedLength": n(j.done),
		"uploadLength": n(j.up), "downloadSpeed": n(j.down), "uploadSpeed": n(j.upSpeed), "connections": n(j.conns), "files": files}
	if j.hash != "" {
		m["infoHash"] = j.hash
		m["numSeeders"] = n(j.seeds)
		bt := map[string]any{"announceList": []any{[]any{"http://127.0.0.1:9/announce"}}}
		if j.name != "" {
			bt["info"] = map[string]any{"name": j.name}
			bt["mode"] = j.mode
		}
		m["bittorrent"] = bt
		if j.status == "active" {
			m["seeder"] = strconv.FormatBool(j.seeder)
		}
	}
	if j.status == "complete" || j.status == "error" || j.status == "removed" {
		code := j.errCode
		if code == "" {
			code = "0"
		}
		m["errorCode"], m["errorMessage"] = code, j.errMsg
	}
	if len(j.followedBy) > 0 {
		m["followedBy"] = j.followedBy
	}
	if j.following != "" {
		m["following"] = j.following
	}
	if len(keys) == 0 {
		return m
	}
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// restart forgets finished and removed results, as aria2 does when it starts again.
func (f *fakeAria2) restart() {
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := []*a2Job{}
	for _, j := range f.jobs {
		if j.status != "complete" && j.status != "removed" {
			kept = append(kept, j)
		}
	}
	f.jobs = kept
}

func (f *fakeAria2) called(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == method {
			n++
		}
	}
	return n
}

func (f *fakeAria2) addCount() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.adds) }

// put adds a finished-looking single-file job straight into the fake.
func (f *fakeAria2) put(j *a2Job) *a2Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	if j.gid == "" {
		j.gid = fmt.Sprintf("%016x", 0xb000+f.next)
	}
	if j.mode == "" {
		j.mode = "single"
	}
	if j.files == nil && j.name != "" {
		if j.mode == "multi" {
			j.files = []a2FakeFile{{path: j.dir + "/" + j.name + "/a.mkv", length: j.total / 2, done: j.done / 2}, {path: j.dir + "/" + j.name + "/b.srt", length: j.total / 2, done: j.done / 2}}
		} else {
			j.files = []a2FakeFile{{path: j.dir + "/" + j.name, length: j.total, done: j.done}}
		}
	}
	f.jobs = append(f.jobs, j)
	return j
}
