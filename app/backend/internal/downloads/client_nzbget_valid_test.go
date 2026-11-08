package downloads

import "testing"

func TestValidNZBTakesTheUsualDoctype(t *testing.T) {
	body := `<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb"><file poster="a" date="1" subject="x"><groups><group>g</group></groups><segments><segment bytes="1" number="1">a@b</segment></segments></file></nzb>`
	for _, c := range []struct {
		name string
		doc  string
		ok   bool
	}{
		{"plain", `<?xml version="1.0"?>` + body, true},
		{"NZB 1.1 doctype", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.1//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.1.dtd">` + body, true},
		{"doctype with declarations", `<?xml version="1.0"?><!DOCTYPE nzb [<!ENTITY x "y">]>` + body, false},
		{"other directive", `<?xml version="1.0"?><!ENTITY x "y">` + body, false},
		{"not nzb", `<?xml version="1.0"?><rss></rss>`, false},
	} {
		if got := validNZB([]byte(c.doc)); got != c.ok {
			t.Errorf("%s: validNZB = %v, want %v", c.name, got, c.ok)
		}
	}
}
