package mediaservers

import (
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// View is a media server with its token masked.
type View struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	URL        string     `json:"url"`
	Token      string     `json:"token"`
	Enabled    bool       `json:"enabled"`
	OnImport   bool       `json:"on_import"`
	OnRename   bool       `json:"on_rename"`
	OnDelete   bool       `json:"on_delete"`
	PathFrom   string     `json:"path_from"`
	PathTo     string     `json:"path_to"`
	AllowLocal bool       `json:"allow_local_network"`
	LastOKAt   *time.Time `json:"last_ok_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
}

// ViewOf is m as the app shows it.
func ViewOf(m domain.MediaServer) View {
	v := View{ID: m.ID, Name: m.Name, Kind: m.Kind, URL: m.URL, Enabled: m.Enabled, OnImport: m.OnImport, OnRename: m.OnRename,
		OnDelete: m.OnDelete, PathFrom: m.PathFrom, PathTo: m.PathTo, AllowLocal: m.AllowLocal, LastOKAt: m.LastOKAt, LastError: m.LastError}
	if m.TokenSecretID != nil {
		v.Token = secrets.Masked
	}
	return v
}
