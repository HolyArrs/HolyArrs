package imgstore

import "testing"

// Names match on NFKC + case folding with only punctuation removed: every
// script is kept, so Japanese and Korean names still match (and never fold
// to nothing, as ASCII stripping did).
func TestFoldNameKeepsEveryScript(t *testing.T) {
	same := [][2]string{
		{"ヨルシカ", "ヨルシカ"},
		{"아이유", "아이유"},
		{"ＢＴＳ", "bts"},                     // full width (NFKC)
		{"Beyoncé", "BEYONCÉ"},            // combining accent (NFKC)
		{"Straße", "STRASSE"},              // case folding, not lower case
		{"Guns N’ Roses", "Guns N' Roses"}, // punctuation removed
		{"Tyler, The Creator", "tyler the creator"},
		{"AC/DC", "acdc"},
		{"Sigur Rós", "SIGUR RÓS"},
	}
	for _, p := range same {
		if !SameName(p[0], p[1]) {
			t.Errorf("%q and %q differ: %q %q", p[0], p[1], FoldName(p[0]), FoldName(p[1]))
		}
	}
	differ := [][2]string{{"ヨルシカ", "ヨルシカ2"}, {"Sigur Rós", "Sigur Ros"}, {"", ""}, {"!!!", "???"}}
	for _, p := range differ {
		if SameName(p[0], p[1]) {
			t.Errorf("%q and %q match", p[0], p[1])
		}
	}
	if FoldName("米津玄師") != "米津玄師" || FoldName("  Earth,  Wind & Fire ") != "earth wind fire" {
		t.Fatalf("%q %q", FoldName("米津玄師"), FoldName("  Earth,  Wind & Fire "))
	}
	if !SameTitle("Blonde (Deluxe)", "Blonde") || !SameTitle("STAY - Single", "Stay") || SameTitle("Encore", "The Encore") {
		t.Fatal("title editions")
	}
	if !amongArtists("Frank Ocean & JAY-Z", "Frank Ocean") || amongArtists("Flowers", "Low") || amongArtists("Lowkey", "Low") {
		t.Fatal("artist line")
	}
}
