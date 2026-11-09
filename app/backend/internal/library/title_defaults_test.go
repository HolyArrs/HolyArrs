package library

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestTitleDefaultsBuiltIn(t *testing.T) {
	t.Parallel()
	s := open(t)
	d, saved, err := ReadTitleDefaults(context.Background(), s.Read())
	if err != nil {
		t.Fatal(err)
	}
	want := TitleDefaults{Movie: KindDefaults{KeepUpToDate: true, SearchOnAdd: true}, Show: KindDefaults{KeepUpToDate: true}}
	if d != want || len(saved) != 0 {
		t.Fatalf("built in %+v saved %v", d, saved)
	}
}

func TestTitleDefaultsSaveAndValidate(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	movies := addFolder(t, s)
	shows := addShowFolder(t, s)
	ge := presetID(t, s, mediafiles.GoodEverydayName)
	lossless := presetID(t, s, mediafiles.LosslessFirstName)
	bad := []struct {
		d    TitleDefaults
		want error
	}{
		{TitleDefaults{Movie: KindDefaults{LibraryFolderID: shows.ID}}, ErrBadFolder},
		{TitleDefaults{Show: KindDefaults{LibraryFolderID: movies.ID}}, ErrBadFolder},
		{TitleDefaults{Movie: KindDefaults{LibraryFolderID: 999}}, ErrBadFolder},
		{TitleDefaults{Movie: KindDefaults{PickRulesID: 999}}, ErrNoPickRules},
		{TitleDefaults{Movie: KindDefaults{PickRulesID: lossless}}, ErrNotVideoRules},
		{TitleDefaults{Show: KindDefaults{PickRulesID: lossless}}, ErrNotVideoRules},
	}
	for _, b := range bad {
		if err := SetTitleDefaults(ctx, s, b.d, nil, domain.System); !errors.Is(err, b.want) {
			t.Errorf("%+v: got %v want %v", b.d, err, b.want)
		}
	}
	if _, saved, _ := ReadTitleDefaults(ctx, s.Read()); len(saved) != 0 {
		t.Fatalf("a refused save stored %v", saved)
	}
	d := TitleDefaults{Movie: KindDefaults{LibraryFolderID: movies.ID, PickRulesID: ge, KeepUpToDate: false, SearchOnAdd: false},
		Show: KindDefaults{LibraryFolderID: shows.ID, PickRulesID: ge, KeepUpToDate: true, SearchOnAdd: true}}
	if err := SetTitleDefaults(ctx, s, d, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	got, saved, err := ReadTitleDefaults(ctx, s.Read())
	if err != nil || got != d || len(saved) != 8 || !saved["movie_search_on_add"] || !saved["show_pick_rules_id"] {
		t.Fatalf("read back %+v saved %v err %v", got, saved, err)
	}
}
