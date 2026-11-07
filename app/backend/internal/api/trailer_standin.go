package api

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/youtube"
)

// UseYouTube points the trailer reader at base, a stand-in YouTube for the
// browser tests (main checks the address first). Call it before the trailer
// pass starts.
func UseYouTube(base string) {
	trailerReader = &youtube.Reader{Base: base}
}

// TrailerPassTiming sets when the trailer pass first runs after start and
// how often it runs after that, for the browser tests; a zero or negative
// value keeps the one in use. Call it before the trailer pass starts.
func TrailerPassTiming(first, every time.Duration) {
	if first > 0 {
		trailerPassFirst = first
	}
	if every > 0 {
		trailerPassEvery = every
	}
}
