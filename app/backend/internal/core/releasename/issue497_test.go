package releasename

import "testing"

// German groups write web releases as "WebHD" or "Web-HD".
func TestWebHDSource(t *testing.T) {
	for _, name := range []string{
		"PAW.Patrol.S02E31.German.1080p.WebHD.x264-FuN",
		"PAW.Patrol.S02E31.German.1080p.Web-HD.x264-FuN",
	} {
		p := Parse(name, Hints{})
		if p.Source.Value != "web_dl" || p.Source.Conf == Unknown {
			t.Errorf("%s: source %+v, want web_dl", name, p.Source)
		}
		if p.Group.Value != "FuN" {
			t.Errorf("%s: group %q, want FuN", name, p.Group.Value)
		}
		if p.Title.Value != "PAW Patrol" {
			t.Errorf("%s: title %q", name, p.Title.Value)
		}
	}
}
