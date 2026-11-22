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

// The cap a person sets stays between these: under 5 s ordinary writes on
// slow disks would fail, and past an hour one stuck write would hold up
// every other write for too long.
const (
	minWriteCap = 5 * time.Second
	maxWriteCap = time.Hour
)

// DecideWriteCap reads the setting: a duration such as "90s" or "2m", or
// plain seconds such as "90". Unset gives 0, which the database reads as
// its own default. A value it cannot read, or one outside 5 s to 1 h, is
// odd and gives 0 too, so a typo never removes the cap.
func DecideWriteCap(env string) (limit time.Duration, odd bool) {
	v := strings.TrimSpace(env)
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		n, nerr := strconv.Atoi(v)
		// The range is checked on the number itself, before it becomes
		// a duration, so a huge number cannot wrap into range.
		if nerr != nil || n < int(minWriteCap/time.Second) || n > int(maxWriteCap/time.Second) {
			return 0, true
		}
		d = time.Duration(n) * time.Second
	}
	if d < minWriteCap || d > maxWriteCap {
		return 0, true
	}
	return d, false
}

// WriteCap reads the setting at start and logs a line when it is set or
// odd. A big Library on slow disks can need writes longer than the
// database's default cap; 0 keeps that default.
func WriteCap() time.Duration {
	v := os.Getenv(WriteCapEnv)
	limit, odd := DecideWriteCap(v)
	switch {
	case odd:
		log.Printf("holyarrs: %s=%q is not a time from 5s to 1h; keeping the default write cap", WriteCapEnv, v)
	case limit > 0:
		log.Printf("holyarrs: one database write may run up to %s (%s)", limit, WriteCapEnv)
	}
	return limit
}
