package trailers

import (
	"context"

	"github.com/CoderLuii/holyarrs-metadata/internal/youtube"
)

// Replays reads YouTube's "most replayed" point with HolyArrs's own reader
// (nothing is downloaded). Reader nil: one shared default reader.
type Replays struct{ Reader *youtube.Reader }

var defaultReader = &youtube.Reader{}

// Look is the reader's lookup as the server needs it. The server plays
// nothing, so a video that gives no plain address (only SABR or scrambled
// formats) still counts when the watch page gave its graph or storyboard.
// With neither, the failure stays: the door may have changed. A bot check
// on the player call alone keeps the page's graph the same way.
func (r Replays) Look(ctx context.Context, id string) (youtube.Video, error) {
	rd := r.Reader
	if rd == nil {
		rd = defaultReader
	}
	v, err := rd.Look(ctx, id)
	if youtube.ReasonOf(err) == youtube.NoAddress && (v.PeakFound || v.Storyboard != "") {
		err = nil
	}
	// YouTube's bot check refuses the player call from the server's
	// address at times while the watch page, graph included, still reads:
	// the most replayed start is kept (only the storyboard is missing).
	if youtube.ReasonOf(err) == youtube.BotCheck && v.Page && v.PeakFound {
		err = nil
	}
	// An unread watch page is not "no graph": nothing is stored, and the
	// video is read again later.
	if err == nil && !v.Page {
		err = &youtube.Error{Reason: youtube.Network, Detail: "watch page not read"}
	}
	return v, err
}

// Heatmap answers the start of a video's most replayed part and its
// duration in seconds. The opening is skipped, since every play starts
// there, so a start is not 0:00 just for that. found is false when YouTube
// shows no graph. A failure is the reader's *youtube.Error, whose reason
// tells one video's trouble from the door's.
func (r Replays) Heatmap(ctx context.Context, id string) (peak, duration float64, found bool, err error) {
	v, err := r.Look(ctx, id)
	if err != nil {
		return 0, 0, false, err
	}
	return v.Peak, v.Duration, v.PeakFound, nil
}
