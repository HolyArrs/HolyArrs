package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// An episode keeps its release name when show renaming is off, even
// with movie renaming on, and is renamed when only show renaming is on.
func TestEpisodeUsesShowRenameRule(t *testing.T) {
	ns := mediafiles.NamingDefaults()
	ns.RenameOnImport, ns.ShowRenameOnImport = true, false
	eps := []Episode{{Ref: mediafiles.EpisodeRef{Season: 1, Episode: 2}, Name: "Two"}}
	name := "Trapped.S01E02.1080p.WEB.H264-GRP"
	p := releasename.Parse(name, releasename.Hints{Show: true})
	tgt := ShowTarget{FolderName: "Trapped (2015)", Show: mediafiles.TitleRef{Name: "Trapped", Year: 2015}, Order: mediafiles.OrderAired, Naming: &ns}
	if got := tgt.episodePath(p, name+".mkv", eps); !strings.HasSuffix(got, name+".mkv") {
		t.Fatalf("show rename off: %s", got)
	}
	ns.RenameOnImport, ns.ShowRenameOnImport = false, true
	if got := tgt.episodePath(p, name+".mkv", eps); strings.HasSuffix(got, name+".mkv") {
		t.Fatalf("show rename on: %s", got)
	}
}
