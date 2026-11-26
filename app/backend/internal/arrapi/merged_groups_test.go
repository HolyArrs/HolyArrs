package arrapi

import "testing"

// A profile pushed through the arr API with a group spanning WEB and
// Blu-ray keeps that group, so points lead in its Pick rules.
func TestOldProfileKeepsMergedGroups(t *testing.T) {
	q := func(id int64, n string) wireItem {
		return wireItem{Quality: &wireQuality{ID: id, Name: n}, Allowed: true}
	}
	p := wireProfile{UpgradeAllowed: true, Cutoff: 1001, Items: []wireItem{
		{Quality: &wireQuality{ID: 1, Name: "SDTV"}},
		{ID: 1001, Name: "Merged QPs", Allowed: true, Items: []wireItem{q(5, "WEBDL-720p"), q(6, "Bluray-720p"), q(3, "WEBDL-1080p"), q(7, "Bluray-1080p")}},
		{ID: 1002, Name: "Off", Items: []wireItem{q(14, "WEBRip-720p"), q(30, "Remux-1080p")}},
	}}
	h := &handler{side: "radarr"}
	op := h.oldProfile(p)
	if len(op.Groups) != 1 || len(op.Groups[0]) != 4 {
		t.Fatalf("groups %v", op.Groups)
	}
}
