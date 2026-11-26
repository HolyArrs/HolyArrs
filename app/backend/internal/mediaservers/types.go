package mediaservers

import (
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Input is what a person sets on a media server. An empty token keeps the
// stored one.
type Input struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	URL   string `json:"url"`
	Token string `json:"token"`
	// Username is the sign-in name of a Subsonic-style music server; Token
	// is then its password.
	Username   string `json:"username"`
	Enabled    *bool  `json:"enabled"`
	OnImport   *bool  `json:"on_import"`
	OnRename   *bool  `json:"on_rename"`
	OnDelete   *bool  `json:"on_delete"`
	PathFrom   string `json:"path_from"`
	PathTo     string `json:"path_to"`
	AllowLocal *bool  `json:"allow_local_network"`
	// PlainPassword sends a Subsonic-style server the password itself
	// (LDAP); only over https or to a local address.
	PlainPassword *bool `json:"plain_password"`
	// WebURL is where people's browsers open the server; empty clears it.
	WebURL string `json:"web_url"`
	// UsePlexSignIn takes the token from the owner's Plex sign-in.
	UsePlexSignIn bool `json:"use_plex_sign_in"`

	MonitorPlayback *bool `json:"monitor_playback"`
}

// View is a media server with its token masked.
type View struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Enabled    bool   `json:"enabled"`
	OnImport   bool   `json:"on_import"`
	OnRename   bool   `json:"on_rename"`
	OnDelete   bool   `json:"on_delete"`
	PathFrom   string `json:"path_from"`
	PathTo     string `json:"path_to"`
	AllowLocal bool   `json:"allow_local_network"`
	WebURL     string `json:"web_url"`
	// PlainPassword says the password itself is sent (Subsonic, LDAP).
	PlainPassword bool       `json:"plain_password"`
	LastOKAt      *time.Time `json:"last_ok_at,omitempty"`
	LastError     string     `json:"last_error,omitempty"`

	MonitorPlayback bool `json:"monitor_playback"`
}

// ViewOf is m as the app shows it.
func ViewOf(m domain.MediaServer) View {
	v := View{ID: m.ID, Name: m.Name, Kind: m.Kind, URL: m.URL, Enabled: m.Enabled, MonitorPlayback: m.MonitorPlayback, OnImport: m.OnImport, OnRename: m.OnRename,
		OnDelete: m.OnDelete, PathFrom: m.PathFrom, PathTo: m.PathTo, AllowLocal: m.AllowLocal, WebURL: m.WebURL, PlainPassword: m.PlainPassword, LastOKAt: m.LastOKAt, LastError: m.LastError}
	if m.TokenSecretID != nil {
		v.Token = secrets.Masked
	}
	return v
}
