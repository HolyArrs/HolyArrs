package migrate

import (
	"errors"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// blocklist plans each release on Sonarr's or Radarr's blocklist for the
// Turned down list of its title, so it is never taken again here either.
func (b *builder) blocklist(s *Snapshot) {
	b.cover("blocklist")
	byID := map[int64]Title{}
	for _, t := range s.Titles {
		byID[t.SourceID] = t
	}
	for i, x := range s.Blocklist {
		from := b.from("blocklist", i)
		t, ok := byID[x.TitleID]
		kind, id := idFor(s.Kind, t)
		name := clip(x.Name, maxName)
		switch {
		case !ok || id == "" || kind == "artist":
			b.not(GroupTurnedDown, from, name, "its title did not come over")
			continue
		case name == "" || x.Protocol != "torrent" && x.Protocol != "usenet":
			b.not(GroupTurnedDown, from, name, "the old app gave no release name or protocol for it")
			continue
		}
		ids := map[string]string{}
		for _, k := range []string{"tvdb", "tmdb", "imdb"} {
			if v := clip(t.IDs[k], 60); v != "" {
				ids[k] = v
			}
		}
		b.add(GroupTurnedDown, Item{Key: "turned_down:" + kind + ":" + id + ":" + strings.ToLower(name), Name: name, From: from, Detail: clip(t.Name, maxName),
			TurnDown: &TurnDownPlan{Kind: kind, ID: id, IDs: ids, Name: name, InfoHash: x.InfoHash, Protocol: x.Protocol, Indexer: x.Indexer, Reason: clip(x.Reason, 300), Episodes: x.Episodes}})
	}
}

func (r *run) turnDown(it Item) (string, string) {
	tp := it.TurnDown
	title := r.titleHere(&TitlePlan{Kind: tp.Kind, ID: tp.ID, IDs: tp.IDs})
	if r.dry {
		// Its title may come over in this same move.
		return ResCreate, it.Detail
	}
	if title == 0 {
		return ResFailed, "its title is not in the Library"
	}
	reason := tp.Reason
	if reason == "" {
		reason = "on the old app's blocklist"
	}
	_, made, err := sources.ImportTurnedDown(r.ctx, r.m.Store, sources.TurnDownIn{TitleID: title, Name: tp.Name, InfoHash: tp.InfoHash,
		Protocol: tp.Protocol, Indexer: tp.Indexer, Reason: reason, Episodes: tp.Episodes}, r.by, r.markWith(it.Key, "turned_down"))
	switch {
	case errors.Is(err, sources.ErrNoSourceToFile), errors.Is(err, sources.ErrNoSlotMatches):
		return ResFailed, err.Error()
	case err == nil && !made:
		return ResAlready, "already on the Turned down list"
	}
	return result(err, ResCreated, it.Detail)
}
