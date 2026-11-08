package library

import "sync/atomic"

// SubtitleNames tells which files beside a video are its subtitles. The
// subtitles module answers it; startup hands it over with UseSubtitleNames
// so library does not import a higher layer.
type SubtitleNames interface {
	// IsBeside reports whether name is a subtitle file for the video stem base.
	IsBeside(base, name string) bool
	// TextSub reports whether the head of a .sub file is clearly text.
	TextSub(head []byte) bool
}

var subNames atomic.Pointer[SubtitleNames]

// UseSubtitleNames sets the subtitle naming rules used when files move.
func UseSubtitleNames(n SubtitleNames) { subNames.Store(&n) }

// noSubtitles is used before startup wires the rules: no file counts as a subtitle.
type noSubtitles struct{}

func (noSubtitles) IsBeside(string, string) bool { return false }
func (noSubtitles) TextSub([]byte) bool          { return false }

func subtitleNames() SubtitleNames {
	if p := subNames.Load(); p != nil && *p != nil {
		return *p
	}
	return noSubtitles{}
}

// CurrentSubtitleNames returns the subtitle rules in use.
func CurrentSubtitleNames() SubtitleNames { return subtitleNames() }
