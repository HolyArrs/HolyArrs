package main

import (
	"context"
	"encoding/gob"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// linkSites are the artist links a chart row's own artist id is found by.
var linkSites = []string{"music.apple.com", "itunes.apple.com", "open.spotify.com/artist/", "open.spotify.com/intl-"}

// linkIndex is the slim copy's Apple Music and Spotify artist links by id
// ("applemusic:<id>": MusicBrainz artists). Reading them scans every link
// (about a minute), so a copy's index is kept in dir and read back at the
// next start; a new copy's is built in the background while the last
// one answers. Only the first build, with none kept, is waited for.
type linkIndex struct {
	date  func() string
	links func(ctx context.Context, parts []string) [][2]string
	dir   string // "" keeps none

	mu       sync.Mutex
	byID     map[string][]string
	forDate  string
	building chan struct{} // closed when the build under way ends
	triedFor string
	triedAt  time.Time
}

// get answers the artists linked from key, after the index is up to date
// with the copy in use (see linkIndex).
func (x *linkIndex) get(ctx context.Context, key string) []string {
	x.refresh(ctx)
	x.mu.Lock()
	defer x.mu.Unlock()
	return x.byID[key]
}

// refresh starts a build for a new copy: in the background when an index
// answers meanwhile, else waited for. A build that failed or found none is
// tried again a minute later, not on every row.
func (x *linkIndex) refresh(ctx context.Context) {
	d := x.date()
	if d == "" {
		return
	}
	x.mu.Lock()
	has, wait := x.byID != nil, x.building
	start := d != x.forDate && wait == nil && (d != x.triedFor || time.Since(x.triedAt) > time.Minute)
	if start {
		wait = make(chan struct{})
		x.building, x.triedFor, x.triedAt = wait, d, time.Now()
	}
	x.mu.Unlock()
	if start {
		go x.build(d, wait)
	}
	if has || wait == nil {
		return
	}
	select {
	case <-wait:
	case <-ctx.Done():
	}
}

// build reads copy d's index from dir, else from the copy (then keeps it).
func (x *linkIndex) build(d string, done chan struct{}) {
	defer close(done)
	byID, ok := x.load(d)
	if !ok {
		t := time.Now()
		links := x.links(context.Background(), linkSites)
		if len(links) > 0 {
			byID = map[string][]string{}
			for _, l := range links {
				for _, src := range []string{"applemusic", "spotify"} {
					if id := imgstore.LinkedID([]string{l[1]}, src); id != "" {
						byID[src+":"+id] = append(byID[src+":"+id], l[0])
					}
				}
			}
			log.Printf("chart artist links: copy %s, %d ids in %s", d, len(byID), time.Since(t).Round(time.Second))
			x.save(d, byID)
		}
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(byID) > 0 {
		x.byID, x.forDate = byID, d
	}
	x.building = nil
}

func (x *linkIndex) file(d string) string {
	return filepath.Join(x.dir, "chart-artist-links-"+d+".gob")
}

func (x *linkIndex) load(d string) (map[string][]string, bool) {
	if x.dir == "" {
		return nil, false
	}
	f, err := os.Open(x.file(d))
	if err != nil {
		return nil, false
	}
	defer f.Close()
	var byID map[string][]string
	if gob.NewDecoder(f).Decode(&byID) != nil || len(byID) == 0 {
		return nil, false
	}
	return byID, true
}

// save keeps copy d's index (written aside, then renamed) and drops the
// older copies'.
func (x *linkIndex) save(d string, byID map[string][]string) {
	if x.dir == "" {
		return
	}
	tmp := x.file(d) + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	err = gob.NewEncoder(f).Encode(byID)
	if cerr := f.Close(); err != nil || cerr != nil || os.Rename(tmp, x.file(d)) != nil {
		_ = os.Remove(tmp)
		return
	}
	old, _ := filepath.Glob(filepath.Join(x.dir, "chart-artist-links-*.gob"))
	for _, o := range old {
		if !strings.HasSuffix(o, "-"+d+".gob") {
			_ = os.Remove(o)
		}
	}
}
