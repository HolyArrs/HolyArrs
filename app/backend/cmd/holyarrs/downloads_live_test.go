package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

func TestPublishDLAudiences(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, 't', '{}', 't', '{}', 'owner', 'owner', 'x'), (2, 't', '{}', 't', '{}', 'ana', 'member', 'x'), (3, 't', '{}', 't', '{}', 'bo', 'member', 'x')`)
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'approved', t.id, 2, 't' FROM titles t LIMIT 1`)
	exec41(t, x.s, `INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (1, 2, 't')`)
	exec41(t, x.s, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'downloads_sync', 'on', '', '{}', '', '{}')`)
	var dlr int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dlr)

	sy := &downloads.Sync{Store: &downloads.Store{}, DB: x.s, Manager: noClient{x.api.DownloadsWeb().Manager()}}
	x.api.DownloadsWeb().DLSync = sy
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sy.Around(ctx, func(ctx context.Context) { <-ctx.Done() }); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	for !sy.On() {
		time.Sleep(5 * time.Millisecond)
	}
	hub := x.api.DownloadsWeb().Live
	sctx, scancel := context.WithCancel(context.Background())
	defer scancel()
	st, resume := dlStreams(t, sctx, hub, 0, 2, 3)

	own := "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	out := strings.Repeat("e", 40)
	prev := map[string]downloads.Job{
		own: {ID: own, Name: "Salt", Progress: 0.3, Speed: 100, State: downloads.StateDownloading},
		out: {ID: out, Name: "Secret.Outside", Progress: 0.1, State: downloads.StateDownloading},
	}
	cur := map[string]downloads.Job{
		own:           {ID: own, Name: "Salt", Progress: 0.5, Speed: 200, State: downloads.StateDownloading},
		out:           {ID: out, Name: "Secret.Outside", Progress: 0.2, State: downloads.StateDownloading},
		"f" + out[1:]: {ID: "f" + out[1:], Name: "New.Outside", State: downloads.StateQueued},
	}
	x.api.DownloadsWeb().PublishDL(ctx, dlr, 4, 5, prev, cur)
	gone := map[string]downloads.Job{own: cur[own]}
	x.api.DownloadsWeb().PublishDL(ctx, dlr, 5, 6, cur, gone)

	k := func(ext string) string { return downloads.LiveKey(dlr, ext) }
	mgr := dlPatches(t, frames(st[0], 300*time.Millisecond))
	if len(mgr) != 2 {
		t.Fatalf("manager patches %+v", mgr)
	}
	keys := map[string]map[string]any{}
	for _, u := range mgr[0].Upsert {
		keys[u["key"].(string)] = u
	}
	if keys[k(own)]["progress"] != 0.5 || keys[k(out)]["progress"] != 0.2 || keys["f"] != nil || keys[k("f"+out[1:])]["name"] != "New.Outside" {
		t.Fatalf("manager upserts %+v", mgr[0].Upsert)
	}
	if _, ok := keys[k(own)]["name"]; ok {
		t.Fatalf("own row patch carries more than live numbers: %+v", keys[k(own)])
	}
	if _, ok := keys[k(out)]["name"]; ok {
		t.Fatalf("changed outside row carries unchanged fields: %+v", keys[k(out)])
	}
	if mgr[0].FromRev != 4 || mgr[0].ToRev != 5 || mgr[1].FromRev != 5 || len(mgr[1].Remove) != 2 || mgr[0].Totals == nil {
		t.Fatalf("manager revs/removes %+v", mgr)
	}

	ana := dlPatches(t, frames(st[2], 200*time.Millisecond))
	if len(ana) != 2 || ana[0].FromRev != 4 || ana[0].ToRev != 5 || ana[1].FromRev != 5 || ana[1].ToRev != 6 {
		t.Fatalf("member chain %+v", ana)
	}
	if len(ana[0].Upsert) != 1 || ana[0].Upsert[0]["key"] != k(own) || len(ana[0].Remove) != 0 || len(ana[1].Remove) != 0 || ana[0].Totals != nil {
		t.Fatalf("member got %+v", ana)
	}
	bo := dlPatches(t, frames(st[3], 200*time.Millisecond))
	if len(bo) != 2 || len(bo[0].Upsert) != 0 || len(bo[1].Upsert) != 0 || len(bo[0].Remove)+len(bo[1].Remove) != 0 {
		t.Fatalf("other member got %+v", bo)
	}
	for _, f := range frames(st[3], 50*time.Millisecond) {
		if strings.Contains(f.data, "Outside") || strings.Contains(f.data, own) {
			t.Fatalf("leak to other member: %s", f.data)
		}
	}

	// Health events go to managers only, once per change.
	x.api.DownloadsWeb().PublishDLHealth(ctx)
	x.api.DownloadsWeb().PublishDLHealth(ctx)
	hs := dlPatches(t, frames(st[0], 200*time.Millisecond))
	if len(hs) != 1 || hs[0].Kind != downloads.KindHealth || hs[0].Client != dlr {
		t.Fatalf("manager health %+v", hs)
	}
	if m := dlPatches(t, frames(st[2], 100*time.Millisecond)); len(m) != 0 {
		t.Fatalf("member got health %+v", m)
	}

	// Resuming with ?last= replays the same filtered patches.
	for _, f := range frames(resume(3, hub.EventID(0)), 200*time.Millisecond) {
		if strings.Contains(f.data, "Outside") || strings.Contains(f.data, own) || strings.Contains(f.data, `"health"`) {
			t.Fatalf("replay leak to other member: %s", f.data)
		}
	}
	rp := dlPatches(t, frames(resume(2, hub.EventID(0)), 200*time.Millisecond))
	if len(rp) != 2 || len(rp[0].Upsert) != 1 || rp[0].Upsert[0]["key"] != k(own) {
		t.Fatalf("member replay %+v", rp)
	}
}
