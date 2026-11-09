package discover

import (
	"context"
	"path/filepath"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// VersionView is one wanted copy of a title as the API shows it.
type VersionView struct {
	ID              int64    `json:"id"`
	Label           string   `json:"label"`
	Main            bool     `json:"main"`
	PickRulesID     int64    `json:"pick_rules_id"`
	PickRulesName   string   `json:"pick_rules_name"`
	LibraryFolderID int64    `json:"library_folder_id"`
	FolderPath      string   `json:"folder_path"`
	KeepUpToDate    bool     `json:"keep_up_to_date"`
	Edition         string   `json:"edition"`
	AudioLanguages  []string `json:"audio_languages"`
}

func VersionViewOf(ctx context.Context, db database.Querier, v mediafiles.Version) VersionView {
	out := VersionView{ID: v.ID, Label: v.Label, Main: v.IsMain, PickRulesID: v.PickRulesID, LibraryFolderID: v.LibraryFolderID,
		KeepUpToDate: v.KeepUpToDate, Edition: v.Edition, AudioLanguages: v.AudioLanguages}
	if out.AudioLanguages == nil {
		out.AudioLanguages = []string{}
	}
	m, _ := ctx.Value(lookupMemoKey{}).(*lookupMemo)
	if path, ok := m.folder(ctx, db, v.LibraryFolderID); ok {
		out.FolderPath = filepath.Join(path, v.FolderName)
	}
	if name, ok := m.rules(ctx, db, v.PickRulesID); ok {
		out.PickRulesName = name
	}
	return out
}

// lookupMemo keeps the Library folder paths and Pick rules names read during
// one request, so listing every Title reads each row once, not per Version.
type lookupMemo struct{ folders, picks map[int64]*string }

type lookupMemoKey struct{}

// WithLookupMemo keeps a memo on ctx for VersionViewOf.
func WithLookupMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, lookupMemoKey{}, &lookupMemo{folders: map[int64]*string{}, picks: map[int64]*string{}})
}

// memoRead answers id from seen, or reads it once with get and keeps it
// (a nil memo just reads).
func memoRead(seen map[int64]*string, id int64, get func() (string, bool)) (string, bool) {
	if seen != nil {
		if v, ok := seen[id]; ok {
			return deref(v), v != nil
		}
	}
	v, ok := get()
	if seen != nil {
		if ok {
			seen[id] = &v
		} else {
			seen[id] = nil
		}
	}
	return v, ok
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (m *lookupMemo) folder(ctx context.Context, db database.Querier, id int64) (string, bool) {
	var seen map[int64]*string
	if m != nil {
		seen = m.folders
	}
	return memoRead(seen, id, func() (string, bool) {
		var lf mediafiles.LibraryFolder
		err := database.Get(ctx, db, "library_folders", id, &lf)
		return lf.Path, err == nil
	})
}

func (m *lookupMemo) rules(ctx context.Context, db database.Querier, id int64) (string, bool) {
	var seen map[int64]*string
	if m != nil {
		seen = m.picks
	}
	return memoRead(seen, id, func() (string, bool) {
		var pr mediafiles.PickRules
		err := database.Get(ctx, db, "pick_rules", id, &pr)
		return pr.Name, err == nil
	})
}
