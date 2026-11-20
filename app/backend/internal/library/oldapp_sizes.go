package library

import (
	"math"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// OldSize is one row of a Sonarr or Radarr size table: a quality by name
// and its smallest and largest size in MB a minute (0 largest: no limit).
type OldSize struct {
	Name string
	Min  float64
	Max  float64
}

const mib = int64(1) << 20

// NoSizeMax stands for "no largest size" on a step that has a smallest one:
// Pick rules want largest >= smallest, and 1 TiB a minute is never reached.
const NoSizeMax = int64(1) << 40

// OldSizeLimits turns an old app's size table into per-minute limits per
// quality step. Several old qualities on one step take the widest pair; a
// quality with no step is left out.
func OldSizeLimits(rows []OldSize) map[string]mediafiles.SizeLimit {
	type pair struct{ lo, hi int64 } // hi -1: no limit
	steps := map[string]*pair{}
	for _, row := range rows {
		step := QualityStep(row.Name)
		if !slices.Contains(mediafiles.Ladder, step) {
			continue
		}
		lo, hi := mbToBytes(row.Min), mbToBytes(row.Max)
		if hi == 0 {
			hi = -1
		}
		p, ok := steps[step]
		if !ok {
			steps[step] = &pair{lo, hi}
			continue
		}
		p.lo = min(p.lo, lo)
		if p.hi != -1 && (hi == -1 || hi > p.hi) {
			p.hi = hi
		}
	}
	out := map[string]mediafiles.SizeLimit{}
	for step, p := range steps {
		l := mediafiles.SizeLimit{Min: p.lo, Max: p.hi, PerMinute: true}
		switch {
		case p.hi == -1 && p.lo == 0:
			l.Max = 0
		case p.hi == -1 || p.hi < p.lo:
			l.Max = max(NoSizeMax, p.lo)
		}
		out[step] = l
	}
	return out
}

// mbToBytes reads MB a minute as bytes a minute, held to 0..1 TiB.
func mbToBytes(mb float64) int64 {
	if math.IsNaN(mb) || mb <= 0 {
		return 0
	}
	return int64(math.Min(mb, float64(NoSizeMax/mib)) * float64(mib))
}
