package migrate

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// fileRules plans Sonarr's or Radarr's "unmonitor deleted" switch. Off is
// this server's default too, so only a switch that is on becomes an item.
func (b *builder) fileRules(s *Snapshot) {
	v, ok := s.Settings["unkeep_deleted"]
	kind := libraryKind(s.Kind)
	if !ok || kind != "movie" && kind != "show" {
		return
	}
	from := b.from("settings", -1) + ".unkeep_deleted"
	if !boolean(v) {
		b.p.Mapped = append(b.p.Mapped, from)
		return
	}
	name, detail := "Stop looking for deleted movies", "Keep up to date turns off for a movie whose file is deleted from disk"
	if kind == "show" {
		name, detail = "Stop looking for deleted episodes", "Keep up to date turns off for an episode whose file is deleted from disk"
	}
	b.add(GroupNaming, Item{Key: "file_rules:unkeep_deleted:" + kind, Name: name, From: from, Detail: detail,
		FileRules: &FileRulesPlan{Kind: kind, UnkeepDeleted: true}})
}

// setUnkeep sets the switch of kind in r.
func setUnkeep(r *importer.FileRules, kind string, on bool) {
	if kind == "movie" {
		r.UnkeepDeletedMovies = on
	} else {
		r.UnkeepDeletedEpisodes = on
	}
}

func unkeepOf(r importer.FileRules, kind string) bool {
	if kind == "movie" {
		return r.UnkeepDeletedMovies
	}
	return r.UnkeepDeletedEpisodes
}

func (r *run) fileRules(it Item) (string, string) {
	fp := it.FileRules
	cur, err := importer.ReadFileRules(r.ctx, r.m.Store.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	if unkeepOf(cur, fp.Kind) == fp.UnkeepDeleted {
		return ResAlready, "already set this way"
	}
	if r.dry {
		return ResChange, it.Detail
	}
	before, _ := json.Marshal(FileRulesPlan{Kind: fp.Kind, UnkeepDeleted: unkeepOf(cur, fp.Kind)})
	after, _ := json.Marshal(fp)
	next := cur
	setUnkeep(&next, fp.Kind, fp.UnkeepDeleted)
	err = importer.SaveFileRules(r.ctx, r.m.Store, next, r.by, func(tx *sql.Tx) error {
		return r.markTx(tx, it.Key, "file_rules", 0, string(before), string(after))
	})
	return result(err, ResChanged, it.Detail)
}

// undoFileRules puts the switch back while it still holds what the move set.
func undoFileRules(ctx context.Context, s *database.Store, before, after string, by domain.Actor) (string, string) {
	var old, moved FileRulesPlan
	if json.Unmarshal([]byte(before), &old) != nil || json.Unmarshal([]byte(after), &moved) != nil || old.Kind != moved.Kind {
		return ResKept, keptUnknown
	}
	cur, err := importer.ReadFileRules(ctx, s.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	if unkeepOf(cur, moved.Kind) != moved.UnkeepDeleted {
		return ResKept, keptChanged
	}
	setUnkeep(&cur, old.Kind, old.UnkeepDeleted)
	if err := importer.SaveFileRules(ctx, s, cur, by); err != nil {
		return ResFailed, err.Error()
	}
	return ResRemoved, "the deleted-file switch is back as it was"
}
