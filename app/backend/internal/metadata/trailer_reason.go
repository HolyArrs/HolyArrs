package metadata

import "github.com/HolyArrs/holyarrs/app/backend/internal/metadata/youtube"

// TrailerReason says why a trailer gives no address. Modules above metadata
// read it here instead of importing the reader's own package.
type TrailerReason = youtube.Reason

// The reasons other modules tell apart.
const (
	TrailerUnavailable = youtube.Unavailable
	TrailerUnplayable  = youtube.Unplayable
	TrailerBotCheck    = youtube.BotCheck
	TrailerNoAddress   = youtube.NoAddress
	TrailerRefused     = youtube.Refused
	TrailerNetwork     = youtube.Network
)
