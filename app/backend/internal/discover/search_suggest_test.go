package discover

import (
	"fmt"
	"testing"
)

func TestSuggestScoreAndPick(t *testing.T) {
	t.Parallel()
	if s := suggestScore(1, 1, "have"); s != 1 {
		t.Fatalf("score %v", s)
	}
	if s := suggestScore(0.5, 0, ""); s != 0.3 {
		t.Fatalf("score %v", s)
	}
	if suggestText("Radiohead", "radi") != 1 || suggestText("The Radio Dept", "radi") != 0.8 || suggestText("Paradise", "radi") != 0.5 {
		t.Fatal("text match")
	}
	rows := []SuggestRow{}
	for i := 0; i < 5; i++ {
		rows = append(rows, SuggestRow{Kind: "song", Name: fmt.Sprint("Radi ", i), score: float64(i)})
	}
	words, out := pickSuggest(rows, "radi")
	if len(out) != 3 || out[0].Name != "Radi 4" || len(words) != 3 || words[0] != "radi 4" {
		t.Fatalf("pick %v %+v", words, out)
	}
}
