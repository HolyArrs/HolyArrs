package music

import "testing"

func TestNonStudioTitle(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"Non-Album Demos":             true,
		"Born to Die Demos":           true,
		"Yeezus Demos":                true,
		"Nevermind Demos & Outtakes":  true,
		"Unreleased Tracks":           true,
		"Lust For Life: B-Sides":      true,
		"Bootleg Vol. 2":              true,
		"Ultraviolence":               false,
		"Democrazy":                   false,
		"Running From Demons":         false,
		"A Rush of Blood to the Head": false,
	} {
		if got := nonStudioTitle(name); got != want {
			t.Errorf("nonStudioTitle(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNearTasteOK(t *testing.T) {
	t.Parallel()
	const seed, inLib = "coldplay", "radiohead"
	lib := map[string]int{seed: 0, inLib: 1}
	related := map[string]bool{"chris-martin": true, "young-money": true}
	for _, c := range []struct {
		id, name string
		want     bool
	}{
		{"keane", "Keane", true},
		{seed, "Coldplay", false},
		{inLib, "Radiohead", false},
		{"chris-martin", "Chris Martin", false},
		{"young-money", "Young Money", false},
		{"asutc", "Artists Stand Up to Cancer", false},
		{"va", "Various Artists", false},
		{"band-aid", "Band Aid", false},
		{"", "Nobody", false},
	} {
		if got := nearTasteOK(c.id, c.name, seed, lib, related); got != c.want {
			t.Errorf("nearTasteOK(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
