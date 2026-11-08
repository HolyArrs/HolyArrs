package model

import "testing"

// A Downloader's folder as HolyArrs sees it, for Linux and Windows
// Downloaders, with HolyArrs on Linux (Docker) or on Windows.
func TestMapPath(t *testing.T) {
	for _, c := range []struct {
		name, p string
		maps    []PathMap
		want    string
		ok      bool
	}{
		{"linux to linux", "/data/downloads/holyarrs", []PathMap{{"/data/downloads", "/srv/dl"}}, "/srv/dl/holyarrs", true},
		{"the folder itself", "/data/downloads", []PathMap{{"/data/downloads/", "/srv/dl/"}}, "/srv/dl", true},
		{"whole parts only", "/data/downloads2/x", []PathMap{{"/data/downloads", "/srv/dl"}}, "/data/downloads2/x", true},
		{"linux is case sensitive", "/Data/Downloads/x", []PathMap{{"/data/downloads", "/srv/dl"}}, "/Data/Downloads/x", true},
		{"windows downloader, holyarrs in docker", `D:\Downloads\holyarrs\Dune`, []PathMap{{`D:\Downloads`, "/downloads"}}, "/downloads/holyarrs/Dune", true},
		{"windows any case", `d:\downloads\holyarrs`, []PathMap{{`D:\Downloads`, "/downloads"}}, "/downloads/holyarrs", true},
		{"windows forward slashes", "D:/Downloads/holyarrs", []PathMap{{`D:\Downloads`, "/downloads"}}, "/downloads/holyarrs", true},
		{"windows to windows", `D:\Downloads\holyarrs`, []PathMap{{`D:\Downloads`, `E:\Media\dl`}}, `E:\Media\dl\holyarrs`, true},
		{"spaces around the pair", "/data/downloads/x", []PathMap{{" /data/downloads ", " /srv/dl "}}, "/srv/dl/x", true},
		{"no mapping", "/data/downloads/x", nil, "/data/downloads/x", true},
		{"dot dot refused", "/data/downloads/../../etc", []PathMap{{"/data/downloads", "/srv/dl"}}, "/data/downloads/../../etc", false},
		{"windows dot dot refused", `D:\Downloads\..\Windows`, []PathMap{{`D:\Downloads`, "/downloads"}}, `D:\Downloads\..\Windows`, false},
		{"dots in a name are fine", "/data/downloads/Dune..2024", []PathMap{{"/data/downloads", "/srv/dl"}}, "/srv/dl/Dune..2024", true},
	} {
		got, ok := MapPath(c.p, c.maps)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: %q %v, want %q %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
