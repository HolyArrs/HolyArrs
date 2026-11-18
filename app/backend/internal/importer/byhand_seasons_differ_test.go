package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// A show whose seasons the old app numbers differently gets no episode
// guessed from the numbers in a file name: "S01E03" there is another
// episode here, so the row starts with no episode and is never ticked.
func TestByHandSeasonsDifferGuessesNoEpisode(t *testing.T) {
	w := showSetup(t, nil)
	ctx := context.Background()
	rel := w.folder + "/Season 1/Copper Valley - S01E03 - Old Numbers.mkv"
	write(t, filepath.Join(w.lib, filepath.FromSlash(rel)), 2048)
	im := &Importer{Store: w.s}
	guess := func() ByHandFile {
		t.Helper()
		sc, err := im.ScanByHand(ctx, w.lib)
		if err != nil || len(sc.Files) != 1 {
			t.Fatalf("scan %v %+v", err, sc.Files)
		}
		return sc.Files[0]
	}
	if f := guess(); f.Guess.Episode == nil || *f.Guess.Episode != 3 {
		t.Fatalf("an ordinary show is guessed by number: %+v", f.Guess)
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET seasons_differ = 1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f := guess()
	if f.Guess.TitleID == 0 {
		t.Fatalf("the show itself is still offered: %+v", f.Guess)
	}
	if f.Guess.Season != nil || f.Guess.Episode != nil || f.Guess.SlotID != 0 {
		t.Fatalf("guessed an episode by number: %+v", f.Guess)
	}
}
