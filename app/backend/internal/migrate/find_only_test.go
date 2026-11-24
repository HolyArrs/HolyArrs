package migrate

import "testing"

// Setup asks about one app: only targets that can answer as it stay, and a
// shared port keeps just the asked kind.
func TestOnlyKindsKeepsTheAskedApp(t *testing.T) {
	all := []findTarget{
		{Host: "a", Port: 7878, Kinds: []Kind{Radarr}},
		{Host: "a", Port: 8989, Kinds: []Kind{Sonarr}},
		{Host: "a", Port: 8080, Kinds: []Kind{SABnzbd, QBittorrent}},
	}
	got := onlyKinds(all, []Kind{Radarr})
	if len(got) != 1 || got[0].Port != 7878 {
		t.Fatalf("radarr only: %+v", got)
	}
	got = onlyKinds(all, []Kind{QBittorrent})
	if len(got) != 1 || len(got[0].Kinds) != 1 || got[0].Kinds[0] != QBittorrent {
		t.Fatalf("qbittorrent only: %+v", got)
	}
	if len(onlyKinds(all, nil)) != len(all) {
		t.Fatal("no kinds must search everything")
	}
	if all[2].Kinds[0] != SABnzbd || len(all[2].Kinds) != 2 {
		t.Fatal("the caller's targets must not change")
	}
}
