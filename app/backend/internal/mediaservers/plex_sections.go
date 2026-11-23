package mediaservers

import (
	"context"
	"encoding/xml"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// plexSection is one Plex library and its folders.
type plexSection struct {
	Key       string `xml:"key,attr"`
	Type      string `xml:"type,attr"`
	Title     string `xml:"title,attr"`
	Locations []struct {
		Path string `xml:"path,attr"`
	} `xml:"Location"`
}

func (l *Linker) plexSections(ctx context.Context, m domain.MediaServer) ([]plexSection, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/library/sections", nil)
	if err != nil {
		return nil, err
	}
	var mc struct {
		XMLName     xml.Name
		Directories []plexSection `xml:"Directory"`
	}
	// A page or list that is not Plex's means another app answers there.
	if err := xml.Unmarshal(b, &mc); err != nil || mc.XMLName.Local != "MediaContainer" {
		return nil, doctor.Mark(doctor.WrongApp, nil)
	}
	var out []plexSection
	for _, d := range mc.Directories {
		if d.Type == "movie" || d.Type == "show" || d.Type == "artist" {
			out = append(out, d)
		}
	}
	return out, nil
}

// plexType is the Plex library type for a title kind.
func plexType(kind string) string {
	if kind == "show" || kind == "artist" {
		return kind
	}
	return "movie"
}
