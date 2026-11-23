package logs

import (
	"strings"
	"testing"
)

func TestForPluginTable(t *testing.T) {
	for _, c := range []struct {
		in       string
		kept     bool
		mustNot  []string
		mustKeep string
	}{
		{"downloads: sent Dune to qBittorrent", true, nil, "sent Dune"},
		{"auth: first-run setup made the owner \"luis\" from 127.0.0.1:5000", false, nil, ""},
		{"accounts: password changed for ana", false, nil, ""},
		{"invite used by new user ana", false, nil, ""},
		{"plugin demo: GET /api/v1/titles from 192.168.1.20", true, []string{"192.168.1.20"}, "plugin demo"},
		{"plugin x used hap_ABCDEF1234567890abcdef", true, []string{"hap_ABCDEF"}, "plugin x"},
		{"mail failed for ana@example.com", true, []string{"ana@example.com"}, "mail failed"},
		{"source: http://user:pw@idx.example/api?apikey=abc123def456", true, []string{"pw@", "abc123def456"}, "source"},
		{"peer [fe80::1:2] refused", true, []string{"fe80::1:2"}, "refused"},
		{"library: renamed \"Ana's Home Movies\"", true, []string{"Ana's Home Movies"}, "renamed"},
		{"HOLYARRS session abc expired", false, nil, ""},
	} {
		got := (*Log)(nil).ForPlugin([]Line{{Text: c.in}})
		if (len(got) == 1) != c.kept {
			t.Errorf("%q: kept %v", c.in, len(got) == 1)
			continue
		}
		if !c.kept {
			continue
		}
		for _, s := range c.mustNot {
			if strings.Contains(got[0].Text, s) {
				t.Errorf("%q: still shows %q: %q", c.in, s, got[0].Text)
			}
		}
		if !strings.Contains(got[0].Text, c.mustKeep) {
			t.Errorf("%q: lost %q: %q", c.in, c.mustKeep, got[0].Text)
		}
	}
}
