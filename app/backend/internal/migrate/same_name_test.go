package migrate

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A show is the folder's only when the names match the way release names
// are matched, and the years agree when both are known.
func TestSameName(t *testing.T) {
	for _, c := range []struct {
		name   string
		year   int
		folder string
		fyear  int
		want   bool
	}{
		{"The Office", 2005, "The Office (US)", 2005, true},
		{"The Office (US)", 2005, "The Office (US)", 2005, true},
		{"Marvel's Daredevil", 2015, "Marvels Daredevil", 2015, true},
		{"Doctor Who", 2005, "Doctor Who (2005)", 0, true},
		{"Doctor Who", 1963, "Doctor Who (2005)", 2005, false},
		{"Doctor Who", 1963, "Doctor Who", 2005, false},
		{"Cunk on...", 2018, "Cunk on Earth", 2022, false},
		{"Cunk on...", 0, "Cunk on Earth", 0, false},
		{"Home", 2020, "Homeland", 2020, false},
	} {
		if got := sameName(metadata.Show{Name: c.name, Year: c.year}, c.folder, c.fyear); got != c.want {
			t.Errorf("%q (%d) vs folder %q (%d): %v", c.name, c.year, c.folder, c.fyear, got)
		}
	}
}
