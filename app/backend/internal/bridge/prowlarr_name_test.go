package bridge

import "testing"

// An entry in Prowlarr that already uses HolyArrs's name (another HolyArrs)
// is left alone: the new one gets its address in the name instead.
func TestUniqueName(t *testing.T) {
	have := []app{{"name": "HolyArrs TV shows"}, {"name": "Sonarr"}}
	if got := uniqueName(have, "HolyArrs Movies", "http://10.0.0.5:6262/radarr"); got != "HolyArrs Movies" {
		t.Fatalf("free name changed: %q", got)
	}
	if got := uniqueName(have, "HolyArrs TV shows", "http://10.0.0.5:6262/sonarr"); got != "HolyArrs TV shows 10.0.0.5:6262" {
		t.Fatalf("taken name: %q", got)
	}
	have = append(have, app{"name": "holyarrs tv shows 10.0.0.5:6262"})
	if got := uniqueName(have, "HolyArrs TV shows", "http://10.0.0.5:6262/sonarr"); got != "HolyArrs TV shows 10.0.0.5:6262 2" {
		t.Fatalf("taken twice: %q", got)
	}
}
