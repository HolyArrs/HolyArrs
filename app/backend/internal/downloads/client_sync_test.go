package downloads

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

const testHash = "0123456789abcdef0123456789abcdef01234567"

const parityInfo = `[
{"hash":"AABBCCDDEEFF00112233445566778899AABBCCDD","name":"Upper","category":"holyarrs","state":"downloading","size":1000,"progress":0.25,"dlspeed":500,"upspeed":10,"eta":300,"added_on":1700000000,"tracker":"https://t.example/abc/announce","num_seeds":3},
{"hash":"1111111111111111111111111111111111111111","infohash_v2":"2222222222222222222222222222222222222222222222222222222222222222","name":"Hybrid","category":"holyarrs","state":"stalledUP","size":50,"progress":1,"ratio":1.5,"completion_on":1700000500,"eta":8640000},
{"hash":"3333333333333333333333333333333333333333","infohash_v2":"4444444444444444444444444444444444444444444444444444444444444444","name":"V2 only","category":"linux","state":"pausedDL","size":9}
]`

const parityMainFull = `{"rid":1,"full_update":true,"torrents":{
"AABBCCDDEEFF00112233445566778899AABBCCDD":{"infohash_v1":"aabbccddeeff00112233445566778899aabbccdd","name":"Upper","category":"holyarrs","state":"downloading","size":1000,"progress":0.25,"dlspeed":500,"upspeed":10,"eta":300,"added_on":1700000000,"tracker":"https://t.example/abc/announce","num_seeds":3},
"1111111111111111111111111111111111111111":{"infohash_v2":"2222222222222222222222222222222222222222222222222222222222222222","name":"Hybrid","category":"holyarrs","state":"stalledUP","size":50,"progress":1,"ratio":1.5,"completion_on":1700000500,"eta":8640000},
"3333333333333333333333333333333333333333":{"infohash_v2":"4444444444444444444444444444444444444444444444444444444444444444","name":"V2 only","category":"linux","state":"pausedDL","size":9}
},"server_state":{"up_info_speed":10}}`

const parityInfoAfter = `[
{"hash":"AABBCCDDEEFF00112233445566778899AABBCCDD","name":"Upper","category":"holyarrs","state":"uploading","size":1000,"progress":1,"dlspeed":0,"upspeed":10,"eta":300,"added_on":1700000000,"tracker":"https://t.example/abc/announce","num_seeds":3},
{"hash":"1111111111111111111111111111111111111111","infohash_v2":"2222222222222222222222222222222222222222222222222222222222222222","name":"Hybrid","category":"holyarrs","state":"stalledUP","size":50,"progress":1,"ratio":1.5,"completion_on":1700000500,"eta":8640000}
]`

const parityMainDelta = `{"rid":2,"torrents":{"AABBCCDDEEFF00112233445566778899AABBCCDD":{"state":"uploading","progress":1,"dlspeed":0}},"torrents_removed":["3333333333333333333333333333333333333333"]}`

var notOptional = map[string]bool{"Engine": true, "Publisher": true, "SyncSource": true}

var writeMethods = map[string]bool{
	"Add": true, "Remove": true, "PauseItem": true, "ResumeItem": true, "PauseAll": true, "ResumeAll": true, "MoveTop": true, "MoveItem": true, "SetPriority": true,
	"SetSpeedLimit": true, "Recheck": true, "Reannounce": true, "SetFilePriority": true, "Retry": true, "AddTorrents": true,
	"Bulk": true, "RemoveMany": true, "SetShareLimits": true, "SetSlowMode": true, "AddTrackers": true, "EditTracker": true,
	"RemoveTrackers": true, "SetLocation": true, "SetTorrentLimits": true,
}

var readMethods = map[string]bool{
	"Health": true, "List": true, "Get": true, "Status": true, "Trackers": true, "Peers": true, "Properties": true,
	"TorrentFiles": true, "ListAll": true, "ListActive": true, "SaveBase": true, "SyncMain": true, "History": true,
	"Queue": true, "JobStages": true, "ServerStats": true,
}

var syncerIfaces = map[reflect.Type]bool{
	reflect.TypeFor[Syncer](): true, reflect.TypeFor[MainSyncer](): true, reflect.TypeFor[HistoryReader](): true,
	reflect.TypeFor[QueueReader](): true,
}

type syncStub struct {
	Engine
	err error
}

func TestGuardRegistryEveryOptionalCallPassesBegin(t *testing.T) {
	var requests atomic.Int64
	srv := failingServer(t, &requests)
	for kind, raw := range adapterEngines(t, srv.URL) {
		t.Run(kind, func(t *testing.T) {
			var begins int
			r := outbound.New([]byte("registry-test-key"), outbound.Options{Threshold: 1 << 20})
			g := &guardedEngine{Engine: raw, target: outbound.Target{Kind: "downloader", Identity: []string{kind}},
				begin: func(ctx context.Context, tg outbound.Target) (*outbound.Operation, error) {
					begins++
					return r.Begin(ctx, tg)
				}}
			m := &Manager{Engine: func(domain.Downloader) (Engine, error) { return g, nil }}
			ifaces := append([]reflect.Type{reflect.TypeFor[Engine]()}, optionalInterfaces...)
			for _, it := range ifaces {
				if it == reflect.TypeFor[Syncer]() {
					continue
				}
				if !reflect.TypeOf(raw).Implements(it) {
					continue
				}
				for i := range it.NumMethod() {
					meth := it.Method(i)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					before := begins
					args := callArgs(ctx, meth)
					switch {
					case it == reflect.TypeFor[Engine]() || syncerIfaces[it]:
						_ = callMethod(t, g, it, meth, args)
					default:
						_ = m.ControlEngine(ctx, domain.Downloader{ID: 1}, func(e Engine) error { return callMethod(t, e, it, meth, args) })
					}
					cancel()
					if begins != before+1 {
						t.Errorf("%s.%s: Begin ran %d times", it.Name(), meth.Name, begins-before)
					}
				}
			}
			for _, it := range []reflect.Type{reflect.TypeFor[MainSyncer](), reflect.TypeFor[HistoryReader](), reflect.TypeFor[QueueReader](), reflect.TypeFor[AllLister]()} {
				meth := it.Method(0)
				before := begins
				err := callMethod(t, g, it, meth, callArgs(context.Background(), meth))
				has := reflect.TypeOf(raw).Implements(it)
				if has && begins != before+1 || !has && (begins != before || !errors.Is(err, ErrNotSupported)) {
					t.Errorf("Syncer %s (adapter has it: %v): begins %d err %v", meth.Name, has, begins-before, err)
				}
			}
		})
	}
}

func TestRegistryListsEveryExportedInterface(t *testing.T) {
	listed := map[string]bool{}
	for _, it := range optionalInterfaces {
		listed[it.Name()] = true
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasPrefix(f, "snapshot") { // the reader's files are not engine adapters
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if _, isIface := ts.Type.(*ast.InterfaceType); isIface && ts.Name.IsExported() && !listed[ts.Name.Name] && !notOptional[ts.Name.Name] {
				t.Errorf("exported interface %s (%s) is not in optionalInterfaces", ts.Name.Name, f)
			}
			return true
		})
	}
}

func TestEveryRegistryMethodIsClassified(t *testing.T) {
	for _, it := range append([]reflect.Type{reflect.TypeFor[Engine]()}, optionalInterfaces...) {
		for i := range it.NumMethod() {
			n := it.Method(i).Name
			if writeMethods[n] == readMethods[n] {
				t.Errorf("%s.%s: classify it as a read or a write", it.Name(), n)
			}
		}
	}
}

func TestReadOnlyRegistry(t *testing.T) {
	var requests atomic.Int64
	srv := failingServer(t, &requests)
	for kind, raw := range adapterEngines(t, srv.URL) {
		t.Run(kind, func(t *testing.T) {
			ro := readOnly(raw)
			if !isReadOnly(ro) || unwrapReadOnly(ro) != raw {
				t.Fatal("not marked read only")
			}
			for _, it := range append([]reflect.Type{reflect.TypeFor[Engine]()}, optionalInterfaces...) {
				has, roHas := reflect.TypeOf(raw).Implements(it), reflect.TypeOf(ro).Implements(it)
				if has != roHas {
					t.Fatalf("%s: inner has it %v, read-only wrapper %v", it.Name(), has, roHas)
				}
				if !has {
					continue
				}
				for i := range it.NumMethod() {
					meth := it.Method(i)
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					before := requests.Load()
					err := callMethod(t, ro, it, meth, callArgs(ctx, meth))
					cancel()
					sent := requests.Load() - before
					if writeMethods[meth.Name] {
						if !errors.Is(err, ErrReadOnly) || sent != 0 {
							t.Errorf("write %s.%s: err %v, %d requests", it.Name(), meth.Name, err, sent)
						}
					} else if errors.Is(err, ErrReadOnly) || sent == 0 {
						t.Errorf("read %s.%s did not forward: err %v, %d requests", it.Name(), meth.Name, err, sent)
					}
				}
			}
		})
	}
}

func TestReadOnlyInsideGuardNeverCounts(t *testing.T) {
	var requests atomic.Int64
	srv := failingServer(t, &requests)
	var begins int
	r := outbound.New([]byte("ro-test-key"), outbound.Options{Threshold: 1})
	g := &guardedEngine{Engine: readOnly(NewQbittorrent(srv.URL, nil)), target: outbound.Target{Kind: "downloader"},
		begin: func(ctx context.Context, tg outbound.Target) (*outbound.Operation, error) {
			begins++
			return r.Begin(ctx, tg)
		}}
	if _, err := g.Add(context.Background(), AddRequest{URL: "magnet:?xt=urn:btih:" + testHash}); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if err := g.Remove(context.Background(), testHash, true); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if begins != 0 {
		t.Fatalf("Begin ran %d times for refused writes", begins)
	}
	m := &Manager{Engine: func(domain.Downloader) (Engine, error) { return g, nil }}
	for range 3 {
		err := m.ControlEngine(context.Background(), domain.Downloader{ID: 1}, func(e Engine) error { return e.(Controller).PauseAll(context.Background()) })
		if !errors.Is(err, ErrReadOnly) {
			t.Fatal(err)
		}
	}
	if len(r.States()) != 0 || requests.Load() != 0 {
		t.Fatalf("states %v requests %d", r.States(), requests.Load())
	}
}

func TestReadOnlyListedByIDOrAddress(t *testing.T) {
	d := domain.Downloader{ID: 7, URL: "http://10.0.0.5:8080/"}
	for list, want := range map[string]bool{"7": true, " 3, 7 ": true, "http://10.0.0.5:8080": true, "HTTP://10.0.0.5:8080/": true, "70": false, "": false, "http://10.0.0.5": false} {
		if readOnlyListed(list, d) != want {
			t.Errorf("%q: %v", list, !want)
		}
	}
}

func TestReadOnlyFollowsSwitchAndEnv(t *testing.T) {
	t.Setenv(ReadOnlyEnv, "")
	off := domain.Downloader{ID: 3, URL: "http://nzbget:6789"}
	on := off
	on.Writable = true
	if !ReadOnlyFor(off) || ReadOnlyFor(on) || ReadOnlyByEnv(on) {
		t.Fatal("switch off must be read only, switch on writable")
	}
	t.Setenv(ReadOnlyEnv, "1,3")
	if !ReadOnlyFor(on) || !ReadOnlyByEnv(on) {
		t.Fatal("a listed Downloader stays read only with its switch on")
	}
}

func TestManagerEngineFollowsSwitch(t *testing.T) {
	t.Setenv(ReadOnlyEnv, "")
	var requests atomic.Int64
	srv := failingServer(t, &requests)
	m := &Manager{}
	d := domain.Downloader{ID: 9, Adapter: "qbittorrent", Protocol: "torrent", URL: srv.URL, AllowLocal: true}
	e, err := m.engine(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(context.Background(), testHash, false); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("switch off: %v", err)
	}
	m.Forget(d.ID)
	d.Writable = true
	if e, err = m.engine(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if err := e.Remove(context.Background(), testHash, false); errors.Is(err, ErrReadOnly) {
		t.Fatal("switch on still read only")
	}
}

func (s syncStub) SyncMain(context.Context, int64) (MainData, error) { return MainData{}, s.err }

func (s syncStub) History(context.Context, int64) (HistoryPage, error) { return HistoryPage{}, s.err }

func TestGuardSyncClassification(t *testing.T) {
	parse := &guardedEngine{Engine: syncStub{err: ErrSyncParse}, target: outbound.Target{Kind: "downloader"}}
	r := outbound.New([]byte("class-key"), outbound.Options{Threshold: 2})
	parse.begin = r.Begin
	for range 2 {
		_, _ = parse.SyncMain(context.Background(), 5)
	}
	if len(r.States()) != 1 {
		t.Fatal("parse errors did not count as passing failures")
	}
	r2 := outbound.New([]byte("class-key"), outbound.Options{Threshold: 1})
	auth := &guardedEngine{Engine: syncStub{err: adapterHTTPError{401}}, target: outbound.Target{Kind: "downloader"}, begin: r2.Begin}
	for range 3 {
		_, _ = auth.History(context.Background(), 1)
	}
	if len(r2.States()) != 0 {
		t.Fatal("401 tripped the breaker as a passing failure")
	}
	if _, err := parseMainData([]byte("{not json")); !errors.Is(err, ErrSyncParse) {
		t.Fatal(err)
	}
}

func TestJobFromTorrentParityListVsMaindata(t *testing.T) {
	info, main := parityInfo, parityMainFull
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(info))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(main))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	q := NewQbittorrent(srv.URL, nil)
	ctx := context.Background()
	var st MainState
	check := func(step string) {
		listed, err := q.ListAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		d, err := q.SyncMain(ctx, st.Rid)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.Apply(d); err != nil {
			t.Fatal(err)
		}
		got, want := sortedJobs(st.Jobs("")), sortedJobs(listed)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s:\nmaindata %+v\nlist     %+v", step, got, want)
		}
		var own []Job
		for _, j := range listed {
			if j.Category == "holyarrs" {
				own = append(own, j)
			}
		}
		if !reflect.DeepEqual(sortedJobs(st.Jobs("holyarrs")), sortedJobs(own)) {
			t.Fatalf("%s: category set differs", step)
		}
	}
	check("full")
	if st.Rid != 1 {
		t.Fatalf("rid %d", st.Rid)
	}
	for _, j := range st.Jobs("") {
		if j.ID != strings.ToLower(j.ID) {
			t.Fatalf("id %s not lower case", j.ID)
		}
	}
	info, main = parityInfoAfter, parityMainDelta
	check("delta")
	if len(st.Jobs("")) != 2 {
		t.Fatal("removal lost")
	}
}

func TestSabHistoryCounter(t *testing.T) {
	var lastSent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastSent = append(lastSent, r.URL.Query().Get("last_history_update"))
		if r.URL.Query().Get("last_history_update") == "42" {
			_, _ = w.Write([]byte(`{"history":{"slots":[],"last_history_update":42}}`))
			return
		}
		_, _ = w.Write([]byte(`{"history":{"slots":[{"nzo_id":"SABnzbd_nzo_1","name":"A","category":"tv","bytes":5,"status":"Completed","storage":"/d/tv/A"}],"last_history_update":"42"}}`))
	}))
	defer srv.Close()
	s := NewSabnzbd(srv.URL, "k", nil)
	p, err := s.History(context.Background(), 0)
	if err != nil || p.Last != 42 || !p.Changed || len(p.Jobs) != 1 || p.Jobs[0].State != StateFinished || p.Jobs[0].SavePath != "/d/tv" {
		t.Fatalf("%v %+v", err, p)
	}
	p, err = s.History(context.Background(), 42)
	if err != nil || p.Changed || len(p.Jobs) != 0 {
		t.Fatalf("unchanged: %v %+v", err, p)
	}
	if lastSent[0] != "" || lastSent[1] != "42" {
		t.Fatalf("sent %v", lastSent)
	}
}

func adapterEngines(t *testing.T, base string) map[string]Engine {
	t.Helper()
	out := map[string]Engine{}
	for _, kind := range []string{"qbittorrent", "sabnzbd", "transmission", "deluge", "rtorrent", "aria2", "nzbget"} {
		e, err := (&Manager{}).EngineFor(context.Background(), domain.Downloader{Adapter: kind, URL: base, AllowLocal: true})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		out[kind] = e
	}
	return out
}

func callArgs(ctx context.Context, m reflect.Method) []reflect.Value {
	ft := m.Type
	args := make([]reflect.Value, ft.NumIn())
	for i := range ft.NumIn() {
		in := ft.In(i)
		switch {
		case i == 0:
			args[i] = reflect.ValueOf(ctx)
		case in.Kind() == reflect.String:
			args[i] = reflect.ValueOf(testHash).Convert(in)
		case in.Kind() == reflect.Slice && in.Elem().Kind() == reflect.String:
			args[i] = reflect.ValueOf([]string{testHash}).Convert(in)
		default:
			args[i] = reflect.Zero(in)
		}
	}
	return args
}

func callMethod(t *testing.T, target any, it reflect.Type, m reflect.Method, args []reflect.Value) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s.%s panicked: %v", it.Name(), m.Name, r)
		}
	}()
	out := reflect.ValueOf(target).Convert(it).MethodByName(m.Name).Call(args)
	if last := out[len(out)-1]; !last.IsNil() {
		err = last.Interface().(error)
	}
	return err
}

func failingServer(t *testing.T, requests *atomic.Int64) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sortedJobs(js []Job) []Job {
	sort.Slice(js, func(i, k int) bool { return js[i].ID < js[k].ID })
	return js
}
