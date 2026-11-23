package startup

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// WriteCapEnv is the setting's name.
const WriteCapEnv = "HOLYARRS_WRITE_CAP"

// The cap a person sets stays between these. Below the database's own
// 20 s default, ordinary writes would start to fail, so a lower value
// keeps the default. Past an hour one stuck write would hold up every
// other write for too long, so a higher value is cut to an hour.
const (
	minWriteCap = 20 * time.Second
	maxWriteCap = time.Hour
)

// DecideWriteCap reads the setting: a duration such as "90s" or "5m", or
// plain seconds such as "90". Unset gives 0, which the database reads as
// its own default. A value it cannot read, or one under 20 s, gives 0 and
// odd, so a typo never lowers or removes the cap. A value over an hour
// gives an hour and odd.
func DecideWriteCap(env string) (limit time.Duration, odd bool) {
	v := strings.TrimSpace(env)
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		n, nerr := strconv.ParseInt(v, 10, 64)
		if nerr != nil || n <= 0 {
			return 0, true
		}
		// The range is checked on the number itself, before it becomes
		// a duration, so a huge number cannot wrap into range.
		if n > int64(maxWriteCap/time.Second) {
			return maxWriteCap, true
		}
		d = time.Duration(n) * time.Second
	}
	switch {
	case d < minWriteCap:
		return 0, true
	case d > maxWriteCap:
		return maxWriteCap, true
	}
	return d, false
}

// WriteCap reads the setting at start and logs one line when it changes
// the cap or is odd. 0 keeps the database's default.
func WriteCap() time.Duration {
	v := os.Getenv(WriteCapEnv)
	limit, odd := DecideWriteCap(v)
	switch {
	case odd && limit == 0:
		log.Printf("holyarrs: %s=%q is not a time from 20s to 1h; keeping the 20s write cap", WriteCapEnv, v)
	case odd:
		log.Printf("holyarrs: %s=%q is over 1h; write cap set to %s", WriteCapEnv, v, fmtCap(limit))
	case limit > minWriteCap:
		log.Printf("holyarrs: write cap set to %s by %s", fmtCap(limit), WriteCapEnv)
	}
	return limit
}

// fmtCap writes a cap the way people type it: "5m", "1m30s", "1h".
func fmtCap(d time.Duration) string {
	s := d.String()
	s = strings.Replace(s, "m0s", "m", 1)
	return strings.Replace(s, "h0m", "h", 1)
}
