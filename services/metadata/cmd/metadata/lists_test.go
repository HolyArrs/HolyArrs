package main

import (
	"context"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/lists"
)

func TestOpenListsNeedsKeyAndShowsBudget(t *testing.T) {
	c := config{DataDir: t.TempDir(), MusicTrending: true}
	var o api.Options
	ls, err := openLists(c, func(string) string { return "" }, nil, nil, nil, nil, &o)
	if err != nil || ls != nil || o.Shelves != nil {
		t.Fatal("collector on without a key")
	}
	env := map[string]string{"LISTS_KEY": "k", "LISTS_MONTH_CAP": "6000", "LISTS_PLAN_LIMIT": "12000"}
	ls, err = openLists(c, func(k string) string { return env[k] }, nil, nil, nil, nil, &o)
	if err != nil || ls == nil || o.Shelves == nil || o.StatusExtra == nil {
		t.Fatal("collector not wired")
	}
	defer ls.Close()
	st, ok := o.StatusExtra(context.Background(), time.Hour)["lists"].(lists.Status)
	if !ok || st.MonthCap != 6000 || st.Sources < 100 {
		t.Fatalf("status %+v", st)
	}
	// Without a paid plan, a big cap is held to half the free plan.
	ls3, _ := openLists(config{DataDir: t.TempDir(), MusicTrending: true}, func(k string) string { return map[string]string{"LISTS_KEY": "k", "LISTS_MONTH_CAP": "6000"}[k] }, nil, nil, nil, nil, &api.Options{})
	defer ls3.Close()
	if c := ls3.Status(context.Background()).MonthCap; c != 500 {
		t.Fatalf("cap %d over half the free plan", c)
	}
	ls2, _ := openLists(config{DataDir: t.TempDir(), MusicTrending: true}, func(k string) string { return map[string]string{"LISTS_KEY": "k"}[k] }, nil, nil, nil, nil, &api.Options{})
	defer ls2.Close()
	if ls2.Status(context.Background()).MonthCap != lists.DefaultMonthCap {
		t.Fatal("default cap not 500")
	}
}

func TestMBQueueLeavesListsAShareWhileNamesWait(t *testing.T) {
	sp := &speed{}
	if sp.listsReserve() != 0 {
		t.Fatal("reserve without lists")
	}
	waiting := true
	f := func() bool { return waiting }
	sp.listsWaiting.Store(&f)
	if sp.listsReserve() != listsShare {
		t.Fatal("no share while names wait")
	}
	waiting = false
	if sp.listsReserve() != 0 {
		t.Fatal("share kept after the backlog")
	}
}
