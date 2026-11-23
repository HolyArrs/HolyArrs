package mediaservers

// subtitleWritten is a subtitle file written beside the video (kinds from
// the subtitles package). New content, so it follows the Imports switch,
// and servers hear it as Modified.
const subtitleWritten = "Subtitle"

// sentChange is the change a server hears for a gathered one.
func sentChange(ch string) string {
	if ch == subtitleWritten {
		return Modified
	}
	return ch
}
