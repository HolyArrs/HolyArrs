package library

import (
	"context"
	"errors"
	"os"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Adding a movie needs a Library folder unless hands-off mode is on: then
// it goes under one shared stand-in folder (never listed, never checked on
// disk), or into a real folder of its kind without a disk check, since
// nothing is written there.
func TestHandsOffAddsWithoutAFolder(t *testing.T) {
	cases := []struct {
		name     string
		handsOff bool
		folder   string // "", "ok" or "gone"
		wantErr  error
		standIn  bool
	}{
		{"no folder, normal", false, "", ErrNoFolder, false},
		{"no folder, hands-off", true, "", nil, true},
		{"missing folder, normal", false, "gone", nil, false},
		{"missing folder, hands-off", true, "gone", nil, false},
		{"real folder, hands-off", true, "ok", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := open(t)
			ctx := context.Background()
			if err := parts.SetHandsOff(ctx, s, nil, tc.handsOff, domain.System); err != nil {
				t.Fatal(err)
			}
			if tc.folder != "" {
				lf := addFolder(t, s)
				if tc.folder == "gone" {
					if err := os.RemoveAll(lf.Path); err != nil {
						t.Fatal(err)
					}
				}
			}
			var ids []int64
			for _, m := range []metadata.Movie{dune, future} {
				id, err := AddMovie(ctx, s, m, AddOptions{}, nil, now, domain.System)
				if tc.folder == "gone" && !tc.handsOff {
					var fe *FolderError
					if !errors.As(err, &fe) {
						t.Fatalf("missing folder added: %v", err)
					}
					return
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err %v, want %v", err, tc.wantErr)
				}
				if err != nil {
					return
				}
				ids = append(ids, id)
			}
			var standIns int
			_ = s.Read().QueryRow("SELECT count(*) FROM library_folders WHERE stand_in = 1").Scan(&standIns)
			if (standIns == 1) != tc.standIn || standIns > 1 {
				t.Fatalf("%d stand-in folders", standIns)
			}
			for _, id := range ids {
				if len(slotsOf(t, s, id)) == 0 {
					t.Fatalf("title %d has no Slot", id)
				}
			}
			if tc.standIn {
				if _, found, _ := mediafiles.FirstRealFolder(ctx, s.Read(), "movie"); found {
					t.Fatal("the stand-in folder counts as a real one")
				}
				// With the mode off again, a new movie still needs a real folder.
				if err := parts.SetHandsOff(ctx, s, nil, false, domain.System); err != nil {
					t.Fatal(err)
				}
				other := metadata.Movie{Name: "Arrival", Year: 2016, IDs: map[string]string{"tmdb": "329865"}, CinemaDate: dune.CinemaDate}
				if _, err := AddMovie(ctx, s, other, AddOptions{}, nil, now, domain.System); !errors.Is(err, ErrNoFolder) {
					t.Fatalf("after hands-off: %v", err)
				}
			}
		})
	}
}

// Removing a title in hands-off mode never deletes files, whatever was asked.
func TestHandsOffRemoveKeepsFiles(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ask := RemoveChoice{DeleteFiles: true, NeverAdd: true}
	if got := handsOffChoice(ctx, s.Read(), ask); !got.DeleteFiles {
		t.Fatal("files kept with hands-off off")
	}
	if err := parts.SetHandsOff(ctx, s, nil, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := handsOffChoice(ctx, s.Read(), ask); got.DeleteFiles || !got.NeverAdd {
		t.Fatalf("hands-off choice %+v", got)
	}
}
