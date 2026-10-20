package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/api"
	"github.com/CoderLuii/holyarrs/internal/health"
	"github.com/CoderLuii/holyarrs/internal/ytdlp"
)

// canaryKey is a long-lived official upload (Big Buck Bunny, the Blender
// Foundation's open film) used to check a freshly updated yt-dlp.
const canaryKey = "aqz-KE-bpKQ"

// startYTDLP sets where trailers find yt-dlp (HOLYARRS_YTDLP, the checked
// update in <config>/bin, the copy bundled next to this program, PATH), runs the weekly
// update while "Keep yt-dlp up to date" is on, and feeds the trailers
// health check.
func startYTDLP(ctx context.Context, configDir string, a *api.API, he *health.Engine) {
	self, _ := os.Executable()
	tool := &ytdlp.Tool{DataDir: filepath.Join(configDir, "bin"), Self: self, Canary: ytdlpCanary}
	api.YTDLPTool = tool
	var mu sync.Mutex
	var ver, verBin string
	var verAt time.Time
	he.Trailers = func(ctx context.Context) health.Trailers {
		failed, on := a.TrailerFailures()
		t := health.Trailers{On: on, Failed: failed}
		if !on {
			return t
		}
		bin := tool.Find()
		mu.Lock()
		if bin != verBin || time.Since(verAt) > time.Hour {
			v, err := ytdlp.Version(ctx, bin)
			if err != nil {
				v = ""
			}
			ver, verBin, verAt = v, bin, time.Now()
		}
		t.Version = ver
		mu.Unlock()
		t.Update, _ = tool.LastResult()
		t.UpdateFailed = strings.HasPrefix(t.Update, "update failed")
		return t
	}
	go func() {
		wait := 3 * time.Minute
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = ytdlp.Every
			// An explicit HOLYARRS_YTDLP is the owner's own choice: left alone.
			if os.Getenv(ytdlp.Env) != "" || !a.KeepYTDLPUpdated(ctx) {
				continue
			}
			uctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			_, _ = tool.Update(uctx)
			cancel()
		}
	}()
}

// ytdlpCanary resolves the canary video with a new yt-dlp and needs a
// playable format (the muxed MP4 or an HLS manifest).
func ytdlpCanary(ctx context.Context, bin string) error {
	out, err := ytdlp.Run(ctx, bin, 60*time.Second, "-J", "--no-playlist", "--no-warnings", "--no-cache-dir", "--no-check-formats", "--skip-download", "--ignore-no-formats-error",
		"--extractor-args", "youtube:player_client=visionos;player_skip=configs", "--", "https://www.youtube.com/watch?v="+canaryKey)
	if err != nil {
		return errors.New("canary lookup failed")
	}
	var j struct {
		Formats []struct {
			FormatID    string `json:"format_id"`
			URL         string `json:"url"`
			ManifestURL string `json:"manifest_url"`
		} `json:"formats"`
	}
	if json.Unmarshal(out, &j) != nil {
		return errors.New("canary answer unreadable")
	}
	for _, f := range j.Formats {
		if strings.Contains(f.ManifestURL, "/hls_variant/") || strings.Contains(f.URL, ".googlevideo.com/") {
			return nil
		}
	}
	return errors.New("canary found no playable format")
}
