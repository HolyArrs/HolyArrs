package plugins_test

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestManifestDownloadTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		types any
		ok    bool
	}{
		{"none", nil, true},
		{"empty", []string{}, true},
		{"one", []string{"soulseek"}, true},
		{"four", []string{"soulseek", "ed2k", "direct-http", "ab"}, true},
		{"five", []string{"aa", "bb", "cc", "dd", "ee"}, false},
		{"torrent", []string{"torrent"}, false},
		{"usenet", []string{"soulseek", "usenet"}, false},
		{"upper case", []string{"Soulseek"}, false},
		{"one letter", []string{"s"}, false},
		{"starts with a digit", []string{"2ch"}, false},
		{"underscore", []string{"soul_seek"}, false},
		{"too long", []string{"abcdefghijklmnopqrstuvwxyzabcdef"}, false},
		{"twice", []string{"soulseek", "soulseek"}, false},
		{"not a list", "soulseek", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := plugins.ParseManifest(manifestJSON(func(m map[string]any) {
				if tc.types != nil {
					m["download_types"] = tc.types
				}
			}))
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok %v", err, tc.ok)
			}
			if want, _ := tc.types.([]string); err == nil && !slices.Equal(m.DownloadTypes, want) && len(want)+len(m.DownloadTypes) > 0 {
				t.Fatalf("download types %v", m.DownloadTypes)
			}
		})
	}
	if _, err := plugins.ParseManifest(manifestJSON(func(m map[string]any) { m["Download_Types"] = []string{"soulseek"} })); err == nil {
		t.Fatal("download_types key variant accepted")
	}
}
