package subtitles

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Linking this package in hands the library its rules for subtitle file names.
func init() {
	library.UseSubtitleNames(Names{})
	importer.UseLanguages(NormLanguage)
}

// Names tells the library which files beside a video are its subtitles.
type Names struct{}

// IsBeside reports whether name is a subtitle file for the video stem base.
func (Names) IsBeside(base, name string) bool { return IsBeside(base, name) }

// TextSub reports whether the start of a .sub file is plainly text.
func (Names) TextSub(head []byte) bool { return subtext.TextSub(head) }
