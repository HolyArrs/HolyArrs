package main

import (
	"context"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// showcaseAsks makes Requests: asks waiting for approval (a member's
// and a Kid's), one approved (added and looked for), one turned down, a new
// season of a show already kept, and an artist.
func (w *world) showcaseAsks(ctx context.Context) error {
	owner, err := ownerPerson(ctx, w.s.Read())
	if err != nil {
		return err
	}
	svc := &requests.Service{Store: w.s, Library: w.lib, Metadata: w.lib.Metadata}
	defer svc.Wait()
	for _, e := range showcaseMovieList {
		key := "ask movie " + e.TMDB
		if e.Ask == "" || w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, e.AskedBy)
		if err != nil {
			return fmt.Errorf("member %s: %w", e.AskedBy, err)
		}
		if old, _ := library.TitleByTMDB(ctx, w.s.Read(), e.TMDB); old != 0 {
			if err := w.claim(ctx, old); err != nil {
				return err
			}
		}
		res, err := svc.Ask(ctx, p, e.TMDB)
		if err != nil {
			w.skip("ask %s by %s: %v", e.Name, p.Name, err)
			w.m.Steps[key] = true
			continue
		}
		switch e.Ask {
		case "approved":
			lf, _ := w.folderOf(ctx, "movie")
			a, err := svc.Approve(ctx, res.Ask.ID, owner, requests.ApproveOptions{LibraryFolderID: lf.ID})
			if err != nil {
				w.skip("approve %s: %v", e.Name, err)
			} else if a.TitleID != nil {
				w.m.Titles["movie:"+e.TMDB] = *a.TitleID
			}
		case "turned_down":
			if _, err := svc.TurnDown(ctx, res.Ask.ID, owner, "Already on a streaming service we have"); err != nil {
				w.skip("turn down %s: %v", e.Name, err)
			}
		}
		w.m.Steps[key] = true
		w.say("   %-48s asked by %s, %s", e.Name, p.Name, e.Ask)
	}
	for _, a := range showcaseShowAsks {
		key := "ask show " + a.TMDB
		if w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, a.AskedBy)
		if err != nil {
			return err
		}
		if _, err := svc.AskShow(ctx, p, "tmdb:"+a.TMDB, a.Seasons); err != nil {
			w.skip("ask show %s: %v", a.Name, err)
		} else {
			w.say("   %-48s seasons %v asked by %s", a.Name, a.Seasons, p.Name)
		}
		w.m.Steps[key] = true
	}
	for _, e := range showcaseArtistList {
		key := "ask artist " + e.MBID
		if !e.Ask || w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, "Maya")
		if err != nil {
			return err
		}
		if old, _ := library.ArtistByMBID(ctx, w.s.Read(), e.MBID); old != 0 {
			if err := w.claim(ctx, old); err != nil {
				return err
			}
		}
		if _, err := svc.AskArtist(ctx, p, e.MBID, nil); err != nil {
			w.skip("ask artist %s: %v", e.Name, err)
		} else {
			w.say("   %-48s asked by %s", e.Name, p.Name)
		}
		w.m.Steps[key] = true
	}
	// Follows of artists now in the Library (the owner's and a member's), so
	// the "New from your artists" row has followed artists to draw on.
	for _, e := range showcaseArtistList {
		id, _ := library.ArtistByMBID(ctx, w.s.Read(), e.MBID) // the current Library title (adopt can replace the id)
		if id != 0 {
			w.m.Titles["artist:"+e.MBID] = id
		}
		for _, who := range e.Follow {
			key := "follow artist " + e.MBID + " " + who
			if id == 0 || w.m.Steps[key] {
				continue
			}
			p := owner
			if who != "owner" {
				if p, err = w.member(ctx, who); err != nil {
					return fmt.Errorf("member %s: %w", who, err)
				}
			}
			if err := svc.Follow(ctx, p, id); err != nil {
				w.skip("follow %s by %s: %v", e.Name, p.Name, err)
				continue
			}
			w.say("   %-48s followed by %s", e.Name, p.Name)
			w.m.Steps[key] = true
		}
	}
	return nil
}
