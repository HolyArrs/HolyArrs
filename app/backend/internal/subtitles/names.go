package subtitles

import "github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"

// Names answers which files beside a video are its subtitles.
type Names struct{}

// IsBeside reports whether name is a subtitle file for the video stem base.
func (Names) IsBeside(base, name string) bool { return IsBeside(base, name) }

// TextSub reports whether the head of a .sub file is clearly text.
func (Names) TextSub(head []byte) bool { return subtext.TextSub(head) }
