package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/musicdata"
	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// openMusicData serves popularity, top songs and album details, and puts
// Apple Music's similar artists ahead of ListenBrainz's. It needs the
// picture chains' resolver (the speed system) and the music job; without
// them the routes answer 404 and similar artists stay ListenBrainz only.
func openMusicData(c config, sp *speed, music *musictrend.Service, o *api.Options) (*musicdata.Service, error) {
	if sp == nil || music == nil {
		return nil, nil
	}
	md, err := musicdata.Open(filepath.Join(c.DataDir, "musicdata.sqlite"), musicdata.Options{Sources: sp.res,
		Stored: func(ctx context.Context, kind, mbid string) ([]byte, bool) {
			if sp.handler == nil {
				return nil, false
			}
			return sp.handler.StoredData(ctx, kind, mbid)
		}})
	if err != nil {
		return nil, err
	}
	o.MusicData = md
	// Answers kept before tagging are asked for again, slowly, a few
	// minutes after start (the old answers are served meanwhile).
	go func() {
		time.Sleep(3 * time.Minute)
		if n := md.Retag(context.Background(), 2*time.Second); n > 0 {
			log.Printf("music data: %d answers asked for again to take a source tag", n)
		}
	}()
	if sim, ok := o.Similar.(*musictrend.Similar); ok && sp.res.AppleMusic != nil {
		sim.Apple = md.AppleSimilar
		sim.Names = music.NameMatcher()
	}
	return md, nil
}
