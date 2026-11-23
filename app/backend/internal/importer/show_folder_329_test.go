package importer

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A show stored in a letter or genre folder keeps that folder when
// episodes are imported; a show with no folder yet follows the letter setting.
func TestShowFolderKeepsGroupFolder(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	plain := mediafiles.ShowFolder(ns, mediafiles.NameFacts{Title: "The Office", Year: 2005})
	letter := ns
	letter.ShowGroupByLetter = true
	for _, c := range []struct {
		ns     mediafiles.NamingSettings
		stored string
		want   string
	}{
		{ns, "O/The Office (2005)", "O/The Office (2005)"},
		{ns, "Anime/Frieren (2023)", "Anime/Frieren (2023)"},
		{ns, "The Office (2005)", "The Office (2005)"},
		{ns, "../O/./The Office", "O/The Office"},
		{ns, "", plain},
		{ns, "..", plain},
		{letter, "", "O/" + plain},
		{letter, "The Office", "The Office"},
	} {
		if got := showFolder(c.ns, c.stored, "The Office", 2005, nil); got != c.want {
			t.Errorf("showFolder(%q) = %q, want %q", c.stored, got, c.want)
		}
	}
}
