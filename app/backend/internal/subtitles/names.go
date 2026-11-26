package subtitles

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// Linking this package in hands the library its rules for subtitle file names.
func init() {
	library.UseSubtitleNames(Names{})
	importer.UseLanguages(sideTag)
}

// sideTag is the language part of an imported side file's new name, in the
// same form as the files HolyArrs fetches: zh-TW gives zh-Hant, pt-br gives pt-BR.
func sideTag(word string) string {
	if code := NormLanguage(word); code != "" {
		return LanguageTag(code)
	}
	return ""
}

// Names tells the library which files beside a video are its subtitles.
type Names struct{}

// IsBeside reports whether name is a subtitle file for the video stem base.
func (Names) IsBeside(base, name string) bool { return IsBeside(base, name) }

// TextSub reports whether the start of a .sub file is plainly text.
func (Names) TextSub(head []byte) bool { return subtext.TextSub(head) }
