package main

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// The first build is waited for and kept in dir; a restart reads it back
// without a scan; a new copy is built in the background while the last
// index answers, and the older copy's file is dropped.
func TestLinkIndexKeptAndRebuiltInBackground(t *testing.T) {
	dir := t.TempDir()
	var date atomic.Value
	date.Store("d1")
	var scans atomic.Int32
	release := make(chan struct{}, 4)
	links := func(context.Context, []string) [][2]string {
		scans.Add(1)
		<-release
		if date.Load() == "d1" {
			return [][2]string{{"a1", "https://music.apple.com/us/artist/x/111"}}
		}
		return [][2]string{{"a2", "https://music.apple.com/us/artist/x/111"}}
	}
	newIdx := func() *linkIndex {
		return &linkIndex{date: func() string { return date.Load().(string) }, links: links, dir: dir}
	}
	ctx := context.Background()
	x := newIdx()
	release <- struct{}{}
	if got := x.get(ctx, "applemusic:111"); len(got) != 1 || got[0] != "a1" {
		t.Fatalf("first build: %v", got)
	}
	// A restart: read back, no scan.
	y := newIdx()
	if got := y.get(ctx, "applemusic:111"); len(got) != 1 || got[0] != "a1" || scans.Load() != 1 {
		t.Fatalf("restart: %v, %d scans", got, scans.Load())
	}
	// A new copy: the last index answers while the new one is built.
	date.Store("d2")
	if got := y.get(ctx, "applemusic:111"); len(got) != 1 || got[0] != "a1" {
		t.Fatalf("during the rebuild: %v", got)
	}
	release <- struct{}{}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := y.get(ctx, "applemusic:111"); len(got) == 1 && got[0] == "a2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("new copy never answered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if old, _ := filepath.Glob(filepath.Join(dir, "chart-artist-links-d1.gob")); len(old) != 0 {
		t.Fatal("the older copy's index was kept")
	}
}

// With no copy there is no build and no answer.
func TestLinkIndexNoCopy(t *testing.T) {
	x := &linkIndex{date: func() string { return "" }, links: func(context.Context, []string) [][2]string {
		t.Fatal("scanned with no copy")
		return nil
	}}
	if got := x.get(context.Background(), "spotify:1"); got != nil {
		t.Fatalf("no copy: %v", got)
	}
}
