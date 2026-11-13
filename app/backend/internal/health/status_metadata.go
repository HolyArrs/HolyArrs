package health

import "github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

// metadataModeItems adds the Health lines for where details come from: red
// for a Docker value that was not understood (lookups are then off), grey
// for Your own keys and Off.
func metadataModeItems(in StatusInput, known func(string), add func(StatusItem, string)) {
	const page = "/settings/tmdb"
	switch {
	case in.MetadataBad != "":
		known(page)
		v := in.MetadataBad
		if len(v) > 40 {
			v = v[:40] + "..."
		}
		add(StatusItem{"library", page, metadata.EnvMode + " is \"" + v + "\", which is not full, own or off, so lookups are off on this install", "error"}, "metadata-mode")
	case in.MetadataMode == string(metadata.ModeOff):
		// Off is the owner's choice, not something to fix: no item.
		known(page)
	case in.MetadataMode == string(metadata.ModeOwn):
		known(page)
		add(StatusItem{"library", page, "Details come from your own keys; some rows are off on this install", "info"}, "metadata-mode")
	}
}
