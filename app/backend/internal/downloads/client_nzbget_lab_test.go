package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const labNZBGetRel = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"

func TestNZBGetLab(t *testing.T) {
	base := os.Getenv("HOLYARRS_LAB_NZBGET_URL")
	mode := os.Getenv("HOLYARRS_LAB_NZBGET_MODE")
	if base == "" || mode == "" {
		t.Skip("set HOLYARRS_LAB_NZBGET_URL and HOLYARRS_LAB_NZBGET_MODE")
	}
	password := os.Getenv("HOLYARRS_LAB_NZBGET_PASS")
	if path := os.Getenv("HOLYARRS_LAB_NZBGET_PASS_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil || len(b) == 0 || len(b) > 4096 {
			t.Fatal("read disposable NZBGet password file failed")
		}
		password = string(b)
	}
	n := NewNZBGet(base, os.Getenv("HOLYARRS_LAB_NZBGET_USER"), password, nil)
	n.Category = "holyarrs"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := n.Health(ctx)
	if err != nil || h.Version == "" {
		t.Fatalf("health %+v: %v", h, err)
	}
	fmt.Printf("LAB_NZBGET_VERSION=%s\n", h.Version)

	switch mode {
	case "add":
		marker := os.Getenv("HOLYARRS_LAB_NZBGET_MARKER")
		if marker == "" {
			t.Fatal("set HOLYARRS_LAB_NZBGET_MARKER")
		}
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
  <file poster="holyarrs@example.invalid" date="1" subject="HolyArrs disposable acceptance">
    <groups><group>alt.binaries.test</group></groups>
    <segments><segment bytes="16" number="1">holyarrs-acceptance@example.invalid</segment></segments>
  </file>
</nzb>`))
		}))
		defer source.Close()
		id, err := n.Add(ctx, AddRequest{URL: source.URL + "/acceptance.nzb", Name: "HolyArrs.NZBGet.Acceptance", Category: "holyarrs",
			AttemptMarker: marker, AllowLocal: true})
		if err != nil || id == "" {
			t.Fatalf("add %q: %v", id, err)
		}
		if _, err := n.Get(ctx, id); err != nil {
			active, history, rowsErr := n.rows(ctx)
			t.Logf("disposable rows active=%+v history=%+v err=%v", active, history, rowsErr)
			t.Fatalf("get after add: %v", err)
		}
		fmt.Printf("LAB_NZBGET_ID=%s\n", id)
	case "reconcile":
		marker := os.Getenv("HOLYARRS_LAB_NZBGET_MARKER")
		id, err := n.Add(ctx, AddRequest{Name: "HolyArrs.NZBGet.Acceptance", Category: "holyarrs",
			AttemptMarker: marker, ReconcileOnly: true})
		if err != nil || id != marker {
			t.Fatalf("reconcile %q: %v", id, err)
		}
		fmt.Printf("LAB_NZBGET_RECONCILED_ID=%s\n", id)
	case "check":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		j, err := n.Get(ctx, id)
		if err != nil {
			t.Fatalf("get after restart: %v", err)
		}
		fmt.Printf("LAB_NZBGET_STATE=%s DETAIL=%s\n", j.State, j.Detail)
	case "history":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		active, _, err := n.rows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		native := 0
		for _, row := range active {
			if nzbgetMarkerID(row.Parameters) == id {
				native = row.NZBID
			}
		}
		if native == 0 {
			t.Fatal("owned active row was not found")
		}
		var ok bool
		if err := n.rpc(ctx, "editqueue", []any{"GroupDelete", "", []int{native}}, &ok); err != nil || !ok {
			t.Fatalf("move to history: %v ok=%v", err, ok)
		}
		var j Job
		for i := 0; i < 20; i++ {
			j, err = n.Get(ctx, id)
			if err == nil && j.State == StateFailed {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil || j.State != StateFailed {
			t.Fatalf("history marker %+v: %v", j, err)
		}
		fmt.Printf("LAB_NZBGET_HISTORY_STATE=%s DETAIL=%s\n", j.State, j.Detail)
	case "remove":
		id := os.Getenv("HOLYARRS_LAB_NZBGET_ID")
		if err := n.Remove(ctx, id, false); err != nil {
			t.Fatalf("remove keeping files: %v", err)
		}
		if err := n.Remove(ctx, id, true); err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatalf("remove: %v", err)
		}
		if _, err := n.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("get after remove: %v", err)
		}
	case "clear":
		labNZBGetClearFinished(t, n)
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}

func labNZBGetClearFinished(t *testing.T, n *NZBGet) {
	ctx := context.Background()
	var conf []struct{ Name, Value string }
	if err := n.rpc(ctx, "loadconfig", nil, &conf); err != nil || len(conf) == 0 {
		t.Fatalf("read NZBGet settings: %v", err)
	}
	dest := "/data/downloads/usenet/nzbget-lab"
	set := map[string]string{"Server1.Active": "yes", "Server1.Host": "127.0.0.1", "Server1.Port": "1119", "Server1.Encryption": "no",
		"Server1.Username": "", "Server1.Password": "", "Server1.Connections": "2", "DestDir": dest, "InterDir": dest + "-inter", "AppendCategoryDir": "no", "ParCheck": "manual", "Unpack": "no"}
	changed := make([]map[string]string, 0, len(conf))
	seen := map[string]bool{}
	for _, o := range conf {
		v := o.Value
		if nv, ok := set[o.Name]; ok {
			v, seen[o.Name] = nv, true
		}
		changed = append(changed, map[string]string{"Name": o.Name, "Value": v})
	}
	for k, v := range set {
		if !seen[k] {
			changed = append(changed, map[string]string{"Name": k, "Value": v})
		}
	}
	apply := func(opts any) {
		var ok bool
		if err := n.rpc(ctx, "saveconfig", []any{opts}, &ok); err != nil || !ok {
			t.Fatalf("save NZBGet settings: %v %v", ok, err)
		}
		_ = n.rpc(ctx, "reload", nil, &ok)
		for i := 0; i < 60; i++ {
			time.Sleep(time.Second)
			if _, err := n.Health(ctx); err == nil {
				return
			}
		}
		t.Fatal("NZBGet did not come back after a reload")
	}
	orig := make([]map[string]string, 0, len(conf))
	for _, o := range conf {
		orig = append(orig, map[string]string{"Name": o.Name, "Value": o.Value})
	}
	nntp := startFakeNNTP(t, ":1119")
	apply(changed)
	defer apply(orig)

	data := make([]byte, 600_000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	nzb := nntp.makeNZB(labNZBGetRel+".mkv", data, 200_000)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(nzb)) }))
	defer src.Close()
	marker := fmt.Sprintf("nzbget-%032x", time.Now().UnixNano())
	id, err := n.Add(ctx, AddRequest{URL: src.URL + "/lab.nzb", Name: labNZBGetRel, Category: "holyarrs", AttemptMarker: marker, AllowLocal: true})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	var j Job
	for i := 0; i < 120; i++ {
		if j, err = n.Get(ctx, id); err == nil && (j.State == StateFinished || j.State == StateFailed) {
			break
		}
		time.Sleep(time.Second)
	}
	if j.State != StateFinished {
		_ = n.Remove(ctx, id, true)
		t.Fatalf("job %s (%s): %v", j.State, j.Detail, err)
	}
	folder := j.ContentPath
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		t.Fatalf("NZBGet's folder %q is not on the shared disk: %v", folder, err)
	}
	t.Logf("NZBGet finished %s into %s", labNZBGetRel, folder)

	w, m, dlID, _ := usenetDownload(t)
	m.Engine = func(domain.Downloader) (Engine, error) { return n, nil }
	if err := execSQL(w, `UPDATE downloaders SET adapter = 'nzbget', path_map = '[{"theirs":"/data/downloads","ours":"/data/downloads"}]'`); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w, `UPDATE downloads SET external_id = '`+id+`'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, dlID).DownloaderID)
	m.ClearFinished(ctx, dlID)
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("the job folder %s is still on disk: %v", folder, err)
	}
	if _, err := n.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the cleared job is still in NZBGet: %v", err)
	}
	if countEvents(t, w, "download_removed", dlID) != 1 {
		t.Fatal("no History entry")
	}
	fmt.Printf("LAB_CLEAR_FINISHED=gone %s\n", folder)
}
