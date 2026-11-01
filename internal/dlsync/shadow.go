package dlsync

// The old setting downloads.sync (group "links", key "downloads_sync").
// The background reader is always on now; a stored value of any kind is
// read as on and nothing else reads it.
const (
	SettingGroup = "links"
	SettingSync  = "downloads_sync"
	ModeOn       = "on"
)
