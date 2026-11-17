//go:build lab

package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// One movie grab through the real lab NZBGet, from the host:
//
//	HOLYARRS_LAB_NZBGET_URL=http://127.0.0.1:18712 HOLYARRS_LAB_NZBGET_USER=admin \
//	HOLYARRS_LAB_NZBGET_PASS_FILE=<file> HOLYARRS_LAB_DOWNLOADS=<lab>/downloads \
//	go test -tags lab -run TestLabNZBGetGrabImports ./cmd/holyarrs/
//
// A news server of our own on 127.0.0.1:1119 holds the test file; NZBGet
// reaches it as host.docker.internal for this run only and gets its old
// settings back after. The NZB link answers with a redirect first, like a
// Prowlarr link. HolyArrs sends the job, finds it by its marker in the queue
// and in the history, and the importer places the file.
func TestLabNZBGetGrabImports(t *testing.T) {
	base, downloadsDir := os.Getenv("HOLYARRS_LAB_NZBGET_URL"), os.Getenv("HOLYARRS_LAB_DOWNLOADS")
	if base == "" || downloadsDir == "" {
		t.Skip("set HOLYARRS_LAB_NZBGET_URL and HOLYARRS_LAB_DOWNLOADS")
	}
	b, err := os.ReadFile(os.Getenv("HOLYARRS_LAB_NZBGET_PASS_FILE"))
	if err != nil || len(b) == 0 {
		t.Fatal("read the lab NZBGet password file failed")
	}
	user, pass := os.Getenv("HOLYARRS_LAB_NZBGET_USER"), strings.TrimSpace(string(b))
	const rel = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	const cat = "holyarrs-lab422"
	ctx := context.Background()
	rpc := labNZBGetRPC(base, user, pass)

	data := video(7, 600<<10)
	news := startLabNews(t)
	nzb := news.add(rel+".mkv", data)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getnzb/1" {
			http.Redirect(w, r, "/files/"+rel+".nzb", http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte(nzb))
	}))
	t.Cleanup(src.Close)

	var conf []struct{ Name, Value string }
	if err := rpc("loadconfig", nil, &conf); err != nil || len(conf) == 0 {
		t.Fatalf("read NZBGet settings: %v", err)
	}
	dest := "/data/downloads/usenet/nzbget-lab422"
	set := map[string]string{"Server1.Active": "yes", "Server1.Host": "host.docker.internal", "Server1.Port": "1119", "Server1.Encryption": "no",
		"Server1.Username": "", "Server1.Password": "", "Server1.Connections": "2", "DestDir": dest, "InterDir": dest + "-inter",
		"AppendCategoryDir": "no", "ParCheck": "manual", "Unpack": "no", "DupeCheck": "no"}
	orig, changed := []map[string]string{}, []map[string]string{}
	set2 := map[string]bool{}
	for _, o := range conf {
		orig = append(orig, map[string]string{"Name": o.Name, "Value": o.Value})
		v := o.Value
		if nv, ok := set[o.Name]; ok {
			v, set2[o.Name] = nv, true
		}
		changed = append(changed, map[string]string{"Name": o.Name, "Value": v})
	}
	for k, v := range set {
		if !set2[k] {
			changed = append(changed, map[string]string{"Name": k, "Value": v})
		}
	}
	apply := func(opts []map[string]string) {
		var ok bool
		if err := rpc("saveconfig", []any{opts}, &ok); err != nil || !ok {
			t.Fatalf("save NZBGet settings: %v %v", ok, err)
		}
		_ = rpc("reload", nil, &ok)
		for range 60 {
			time.Sleep(time.Second)
			var v string
			if rpc("version", nil, &v) == nil {
				return
			}
		}
		t.Fatal("NZBGet did not come back after a reload")
	}
	apply(changed)
	t.Cleanup(func() {
		apply(orig)
		_ = os.RemoveAll(filepath.Join(downloadsDir, "usenet", "nzbget-lab422"))
		_ = os.RemoveAll(filepath.Join(downloadsDir, "usenet", "nzbget-lab422-inter"))
	})

	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	labNoSizeFloors(t, s)
	movies := t.TempDir()
	var slotID, relID int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		y := 2024
		lf := &mediafiles.LibraryFolder{Path: movies, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: &y, Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		so := &domain.Source{Name: "Lab Usenet", Protocol: "usenet", Format: "newznab", URL: src.URL + "/api", AllowLocal: true, Kinds: []string{},
			Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", so, domain.System); err != nil {
			return err
		}
		r := &domain.Release{SourceID: so.ID, Name: rel, LinkRef: src.URL + "/getnzb/1", Protocol: "usenet", Size: int64(len(data)),
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", r.ID, sl.ID); err != nil {
			return err
		}
		ref := int64(1)
		d := &domain.Downloader{Name: "Lab NZBGet", Adapter: "nzbget", Protocol: "usenet", URL: base, Username: user, SecretRef: &ref, Category: cat,
			Priority: 1, PathMap: []domain.PathMap{{Theirs: "/data/downloads", Ours: downloadsDir}}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		slotID, relID = sl.ID, r.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &downloads.Manager{Store: s, FromVault: func(context.Context, int64) (string, error) { return pass, nil }}
	im := &importer.Importer{Store: s}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}), config{}))
	t.Cleanup(srv.Close)
	view := func() downloads.DownloadRow {
		var out struct {
			Downloads []downloads.DownloadRow `json:"downloads"`
		}
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/downloads/view", "", 200), &out)
		if len(out.Downloads) != 1 {
			t.Fatalf("rows %+v", out.Downloads)
		}
		return out.Downloads[0]
	}

	id, err := m.Grab(ctx, slotID, relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var ext string
	_ = s.Read().QueryRow("SELECT external_id FROM downloads WHERE id = ?", id).Scan(&ext)
	e, err := m.EngineFor(ctx, domain.Downloader{Adapter: "nzbget", URL: base, Username: user, SecretRef: &slotID, Category: cat, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Remove(context.Background(), ext, true) })
	fmt.Printf("LAB422_SENT external_id=%s\n", ext)

	seen := []string{view().Phase}
	inQueue := false
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if j, err := e.Get(ctx, ext); err == nil && j.State != downloads.StateFinished {
			inQueue = true
		}
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
		r := view()
		if seen[len(seen)-1] != r.Phase {
			seen = append(seen, r.Phase)
		}
		if r.Phase == "waiting_to_import" {
			break
		}
		if r.Phase == "failed" || r.Phase == "vanished" || time.Now().After(deadline) {
			t.Fatalf("phases %v, last %+v", seen, r)
		}
		time.Sleep(time.Second)
	}
	j, err := e.Get(ctx, ext)
	if err != nil || j.State != downloads.StateFinished {
		t.Fatalf("history by marker: %+v %v", j, err)
	}
	fmt.Printf("LAB422_FOUND queue=%v history=%s path=%s\n", inQueue, j.State, j.ContentPath)
	imp, err := im.Import(ctx, id, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	r := view()
	seen = append(seen, r.Phase)
	var st string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	if r.Phase != "done" || (st != string(mediafiles.SlotInLibrary) && st != string(mediafiles.SlotBetterWanted)) {
		t.Fatalf("after import %s: slot %s, row %+v, phases %v", imp.State, st, r, seen)
	}
	var placed []string
	_ = filepath.Walk(movies, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			placed = append(placed, strings.TrimPrefix(p, movies))
		}
		return nil
	})
	fmt.Printf("LAB422_IMPORTED phases=%v import=%s slot=%s placed=%v\n", seen, imp.State, st, placed)
}

func labNZBGetRPC(base, user, pass string) func(method string, params []any, out any) error {
	return func(method string, params []any, out any) error {
		if params == nil {
			params = []any{}
		}
		body, _ := json.Marshal(map[string]any{"method": method, "params": params, "id": 1})
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(base, "/")+"/jsonrpc", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.SetBasicAuth(user, pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var in struct {
			Result json.RawMessage `json:"result"`
			Error  any             `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&in); err != nil {
			return err
		}
		if in.Error != nil {
			return fmt.Errorf("%s: %v", method, in.Error)
		}
		return json.Unmarshal(in.Result, out)
	}
}

// labNews is a one-group news server that holds whole files as one yEnc
// article each.
type labNews struct{ articles map[string][]byte }

func startLabNews(t *testing.T) *labNews {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:1119")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	n := &labNews{articles: map[string][]byte{}}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go n.serve(c)
		}
	}()
	return n
}

func (n *labNews) add(name string, data []byte) string {
	id := "lab422." + strings.ReplaceAll(name, " ", "_") + "@holyarrs.test"
	var b bytes.Buffer
	fmt.Fprintf(&b, "=ybegin line=128 size=%d name=%s\r\n", len(data), name)
	col := 0
	for _, c := range data {
		o := c + 42
		if o == 0 || o == '\n' || o == '\r' || o == '=' || (col == 0 && (o == '.' || o == '\t' || o == ' ')) || (col == 127 && (o == '\t' || o == ' ')) {
			b.WriteByte('=')
			o += 64
			col++
		}
		b.WriteByte(o)
		if col++; col >= 128 {
			b.WriteString("\r\n")
			col = 0
		}
	}
	if col > 0 {
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "=yend size=%d crc32=%08x\r\n", len(data), crc32.ChecksumIEEE(data))
	n.articles[id] = b.Bytes()
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
<file poster="lab@holyarrs.test" date="1700000000" subject="[1/1] - &quot;%s&quot; yEnc (1/1)">
<groups><group>alt.binaries.holyarrs.test</group></groups>
<segments><segment bytes="%d" number="1">%s</segment></segments>
</file>
</nzb>
`, name, b.Len(), id)
}

func (n *labNews) serve(c net.Conn) {
	defer c.Close()
	r, w := bufio.NewReader(c), bufio.NewWriter(c)
	say := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }
	say("200 holyarrs lab news server ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		arg := ""
		if len(f) > 1 {
			arg = f[1]
		}
		switch strings.ToUpper(f[0]) {
		case "CAPABILITIES":
			say("101 list follows\r\nVERSION 2\r\nREADER\r\n.")
		case "MODE":
			say("200 reader mode")
		case "AUTHINFO":
			say("281 welcome")
		case "GROUP":
			say("211 1 1 1 " + arg)
		case "DATE":
			say("111 20261116000000")
		case "QUIT":
			say("205 bye")
			return
		case "BODY", "ARTICLE", "STAT", "HEAD":
			id := strings.Trim(arg, "<>")
			body, ok := n.articles[id]
			if !ok {
				say("430 no such article")
				continue
			}
			switch strings.ToUpper(f[0]) {
			case "STAT":
				say("223 0 <" + id + ">")
				continue
			case "HEAD":
				say("221 0 <" + id + ">\r\nMessage-ID: <" + id + ">\r\nSubject: lab\r\n.")
				continue
			case "ARTICLE":
				_, _ = w.WriteString("220 0 <" + id + ">\r\nMessage-ID: <" + id + ">\r\nSubject: lab\r\n\r\n")
			default:
				_, _ = w.WriteString("222 0 <" + id + ">\r\n")
			}
			for _, l := range strings.SplitAfter(string(body), "\r\n") {
				if strings.HasPrefix(l, ".") {
					_, _ = w.WriteString(".")
				}
				_, _ = w.WriteString(l)
			}
			say(".")
		default:
			say("500 unknown command")
		}
	}
}
