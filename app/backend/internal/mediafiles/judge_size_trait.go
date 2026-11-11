package mediafiles

import (
	"math"
	"strconv"
	"strings"
)

// A size test is written "over-upto" in GB, either end left empty for
// open: "60-" is over 60 GB, "-8" up to 8 GB. A size fits when it is over
// the low end and up to and including the high end, on the whole Release.

const bytesPerGB = 1 << 30

// sizeRange is a size test in bytes; 0 is an open end.
type sizeRange struct{ over, upTo int64 }

// ParseSizeRange reads "over-upto" in GB; ok is false when it cannot be
// read, both ends are open, or the high end is not above the low end.
func ParseSizeRange(v string) (over, upTo float64, ok bool) {
	lo, hi, found := strings.Cut(strings.TrimSpace(v), "-")
	if !found {
		return 0, 0, false
	}
	read := func(s string) (float64, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, true
		}
		n, err := strconv.ParseFloat(s, 64)
		return n, err == nil && n >= 0 && n < 1e6 && !math.IsNaN(n)
	}
	var ok1, ok2 bool
	over, ok1 = read(lo)
	upTo, ok2 = read(hi)
	if !ok1 || !ok2 || (over == 0 && upTo == 0) || (upTo > 0 && upTo <= over) {
		return 0, 0, false
	}
	return over, upTo, true
}

// SizeRangeValue writes a size test back as "over-upto" (0 is open).
func SizeRangeValue(over, upTo float64) string {
	w := func(n float64) string {
		if n <= 0 {
			return ""
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return w(over) + "-" + w(upTo)
}

func compileSizeRange(v string) (sizeRange, bool) {
	over, upTo, ok := ParseSizeRange(v)
	if !ok {
		return sizeRange{}, false
	}
	return sizeRange{over: int64(over * bytesPerGB), upTo: int64(upTo * bytesPerGB)}, true
}

// fits says whether a known size is in the range; an unknown size (0) never fits.
func (r sizeRange) fits(size int64) bool {
	if size <= 0 {
		return false
	}
	return size > r.over && (r.upTo == 0 || size <= r.upTo)
}
