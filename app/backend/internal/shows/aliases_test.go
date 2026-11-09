package shows

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestApplyFillsAliasesForShow(t *testing.T) {
	sh := mediafiles.Title{Aliases: []string{"Kept Name"}}
	applyShow(&sh, metadata.Show{Name: "Money Heist", Status: "ended", OtherNames: []string{"La Casa de Papel", "Haus des Geldes"}})
	if !slices.Equal(sh.Aliases, []string{"Kept Name", "La Casa de Papel", "Haus des Geldes"}) {
		t.Errorf("show aliases %q", sh.Aliases)
	}
}
