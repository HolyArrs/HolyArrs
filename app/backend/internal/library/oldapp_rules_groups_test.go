package library

import (
	"slices"
	"testing"
)

// Qualities inside one allowed group rank equal in the old apps, so the
// format score decides between them. A group spanning several steps becomes
// equal steps; single qualities and one-step groups add none. Quality still
// leads between a group and what is outside it.
func TestOldProfileMergedGroupsRankEqual(t *testing.T) {
	merged := []string{"WEBRip-720p", "WEBDL-720p", "Bluray-720p", "WEBRip-1080p", "WEBDL-1080p", "Bluray-1080p"}
	cases := []struct {
		name  string
		p     OldProfile
		equal [][]string
	}{
		{"trash merged QPs (HD Bluray + WEB)", OldProfile{
			Qualities: merged, CutoffQualities: merged, UpgradeAllowed: true,
			Groups: [][]string{merged}}, [][]string{{"720p_web", "720p_bluray", "1080p_web", "1080p_bluray"}}},
		{"trash merged with remux above", OldProfile{
			Qualities:       append(append([]string{}, merged...), "Remux-1080p"),
			CutoffQualities: []string{"Remux-1080p"}, UpgradeAllowed: true,
			Groups: [][]string{merged}}, [][]string{{"720p_web", "720p_bluray", "1080p_web", "1080p_bluray"}}},
		{"trash WEB-1080p group (WEBRip+WEBDL, one step)", OldProfile{
			Qualities: []string{"WEBRip-1080p", "WEBDL-1080p"}, CutoffQualities: []string{"WEBRip-1080p", "WEBDL-1080p"}, UpgradeAllowed: true,
			Groups: [][]string{{"WEBRip-1080p", "WEBDL-1080p"}}}, nil},
		{"separate qualities", OldProfile{
			Qualities: []string{"WEBDL-1080p", "Bluray-1080p"}, CutoffQualities: []string{"Bluray-1080p"}, UpgradeAllowed: true},
			nil},
		{"group of unknown names", OldProfile{
			Qualities: []string{"Unknown", "WEBDL-1080p"}, CutoffQualities: []string{"WEBDL-1080p"}, UpgradeAllowed: true,
			Groups: [][]string{{"Unknown", "WEBDL-1080p"}}}, nil},
	}
	for _, c := range cases {
		body, _ := OldProfileRules(c.p)
		if body.Order != "quality_then_points" {
			t.Errorf("%s: order %q, want quality_then_points", c.name, body.Order)
		}
		if !slices.EqualFunc(body.EqualSteps, c.equal, slices.Equal) {
			t.Errorf("%s: equal steps %v, want %v", c.name, body.EqualSteps, c.equal)
		}
	}
}
