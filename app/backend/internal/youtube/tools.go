package youtube

// Tools are the programs downloads need. An empty path means not found.
type Tools struct {
	YTDLP, FFmpeg string
}

// FindTools takes HOLYARRS_YTDLP and HOLYARRS_FFMPEG when set, else looks
// each program up on PATH with look (exec.LookPath in production). A set
// path is checked with look too: one that is not a program counts as
// missing rather than falling back to PATH, so a wrong setting shows.
func FindTools(getenv func(string) string, look func(string) (string, error)) Tools {
	find := func(env, name string) string {
		if p := getenv(env); p != "" {
			name = p
		}
		if p, err := look(name); err == nil {
			return p
		}
		return ""
	}
	return Tools{YTDLP: find("HOLYARRS_YTDLP", "yt-dlp"), FFmpeg: find("HOLYARRS_FFMPEG", "ffmpeg")}
}

// Missing names the programs that were not found.
func (t Tools) Missing() []string {
	var out []string
	if t.YTDLP == "" {
		out = append(out, "yt-dlp")
	}
	if t.FFmpeg == "" {
		out = append(out, "ffmpeg")
	}
	return out
}
