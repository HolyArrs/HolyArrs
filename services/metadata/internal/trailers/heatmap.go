package trailers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// Heatmap reads a YouTube video's "most replayed" data with yt-dlp (no
// download) and answers the start of the most replayed part and the
// duration in seconds. found is false when YouTube shows no heatmap.
func (y *YTDLP) Heatmap(ctx context.Context, id string) (peak, duration float64, found bool, err error) {
	out, err := y.VideoJSON(ctx, id)
	if err != nil {
		return 0, 0, false, err
	}
	return ParseHeatmap(out)
}

// VideoJSON is yt-dlp's JSON for one YouTube video (no download): its
// formats (the storyboard sheets among them), duration and heatmap.
func (y *YTDLP) VideoJSON(ctx context.Context, id string) ([]byte, error) {
	if !youtubeID.MatchString(id) {
		return nil, errors.New("bad video id")
	}
	bin := y.Path()
	if bin == "" {
		return nil, errors.New("yt-dlp missing")
	}
	return run(ctx, bin, 60*time.Second, "-J", "--skip-download", "--no-warnings", "--no-playlist", "https://www.youtube.com/watch?v="+id)
}

// ParseHeatmap reads yt-dlp's JSON: the heatmap's highest point.
func ParseHeatmap(b []byte) (peak, duration float64, found bool, err error) {
	var d struct {
		Duration float64 `json:"duration"`
		Heatmap  []struct {
			Start float64 `json:"start_time"`
			Value float64 `json:"value"`
		} `json:"heatmap"`
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return 0, 0, false, err
	}
	best := -1.0
	for _, h := range d.Heatmap {
		if h.Value > best {
			best, peak = h.Value, h.Start
		}
	}
	return peak, d.Duration, best >= 0, nil
}
