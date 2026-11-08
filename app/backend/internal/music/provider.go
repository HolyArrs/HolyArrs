package music

import "github.com/HolyArrs/holyarrs/app/backend/internal/parts"

// MusicOn reports whether music belongs in shared answers (live search,
// New & Popular): the part is on, or safe mode keeps every part's answers.
func (h *Handlers) MusicOn() bool {
	return h.PartIsOn(parts.Music) || h.Parts.SafeMode
}

// WarmNew starts New & Popular's Music lists building in the background.
func (h *Handlers) WarmNew() { h.warmMusicNew() }
