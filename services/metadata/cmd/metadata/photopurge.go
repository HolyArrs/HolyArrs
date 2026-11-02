package main

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
)

// photoPurgeFlag marks the one-off clean-up of artist photos that are a
// blank default or an album cover (audit 2026-10-29) done.
const photoPurgeFlag = "artist_photo_purge_v1"

// slimGroupIDs answers an artist's release-group ids from the slim copy
// (no call): the covers an artist photo must not be.
func slimGroupIDs(slim *mbread.Reader) func(ctx context.Context, mbid string) []string {
	return func(ctx context.Context, mbid string) []string {
		a, ok := slim.Artist(ctx, mbid)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(a.Groups))
		for _, g := range a.Groups {
			out = append(out, strings.ToLower(g.ID))
		}
		return out
	}
}

// purgeBadArtistPhotos runs the clean-up once: the dropped photos go to a
// backup folder first and are asked for again (the next source).
func purgeBadArtistPhotos(ctx context.Context, imgs *imgstore.Store, imgq *fetchq.Queue, dataDir string, now func() time.Time) {
	if imgs.KV(photoPurgeFlag) != "" {
		return
	}
	dir := filepath.Join(dataDir, "backups", "artist-photos-"+now().UTC().Format("20060102-150405"))
	res, err := imgs.PurgeBadArtistPhotos(ctx, dir, func(id string) {
		imgq.Enqueue(ctx, "img:"+imgstore.KindArtist+":"+id, fetchq.P3Refresh, false)
	})
	if err != nil {
		log.Printf("artist photo clean-up: %v", err)
		return
	}
	log.Printf("artist photo clean-up: %d checked, %d blank defaults and %d album covers dropped and asked again (backup %s)", res.Checked, res.Placeholders, res.Covers, dir)
	_ = imgs.SetKV(photoPurgeFlag, now().UTC().Format(time.RFC3339))
}
