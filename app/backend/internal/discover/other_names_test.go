package discover

import (
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestOtherNamesDropsOwnNameAndRepeats(t *testing.T) {
	orig := "氷剣の魔術師が世界を統べる"
	ti := mediafiles.Title{Name: "The Iceblade Sorcerer Shall Rule the World", OriginalName: &orig,
		Aliases: []string{"the iceblade sorcerer shall rule the world", "Hyouken no Majutsushi ga Sekai o Suberu", " ", "Hyouken no Majutsushi ga Sekai o Suberu", orig}}
	got := otherNames(ti)
	want := []string{orig, "Hyouken no Majutsushi ga Sekai o Suberu"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if otherNames(mediafiles.Title{Name: "X"}) != nil {
		t.Fatal("no other names should give nil")
	}
}
