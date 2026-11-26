package releasename

import (
	"reflect"
	"testing"
)

// An "&amp;" that was left as "amp" in a group and a title stays as written.
func TestAmpInGroupAndTitle(t *testing.T) {
	p := Parse("[7³ACG  amp  343 Labs] New Panty  amp  Stocking S2 [BDRip 1080p AV1 OPUS]", Hints{})
	if p.Kind.Value != KindSeasonPack || p.Title.Value != "New Panty amp Stocking" || p.Group.Value != "7³ACG  amp  343 Labs" {
		t.Errorf("kind %q title %q group %q", p.Kind.Value, p.Title.Value, p.Group.Value)
	}
	if p.Show == nil || !reflect.DeepEqual(p.Show.Seasons, []int{2}) {
		t.Errorf("show %+v, want season 2", p.Show)
	}
	if p.Resolution.Value != 1080 || p.Source.Value != "bluray" || p.Codec.Value != "av1" {
		t.Errorf("tags %d %q %q", p.Resolution.Value, p.Source.Value, p.Codec.Value)
	}
}

// Parse leaves a stray "amp" alone because it can be a real word ("Guitar Amp
// Hero"). SameName tries both readings.
func TestAmpWordKept(t *testing.T) {
	for _, c := range []struct{ id, in, title, group string }{
		{"A31", "Law amp Order S01E01 1080p WEB h264-GRP", "Law amp Order", "GRP"},
		{"A32", "Camp amp Rock S01E01 1080p WEB h264-GRP", "Camp amp Rock", "GRP"},
		{"A33", "Law Amp Order S01E01 1080p WEB h264-GRP", "Law Amp Order", "GRP"},
		{"A34", "Amp S01E01 1080p WEB h264-GRP", "Amp", "GRP"},
		{"A35", "Tesla amp S01E01 1080p WEB h264-GRP", "Tesla amp", "GRP"},
		{"A36", "Tube amp Rock S01E01 1080p WEB h264-GRP", "Tube amp Rock", "GRP"},
		{"A37", "Law amp Order 1080p WEB h264-GRP", "Law amp Order", "GRP"},
		{"A38", "Amp.2019.1080p.WEB.h264-GRP", "Amp", "GRP"},
		{"A39", "[Fan amp Sub] Starfall Kitchen - 112 [1080p]", "Starfall Kitchen", "Fan amp Sub"},
	} {
		p := Parse(c.in, Hints{})
		if p.Title.Value != c.title || p.Group.Value != c.group {
			t.Errorf("%s %s: title %q group %q, want %q %q", c.id, c.in, p.Title.Value, p.Group.Value, c.title, c.group)
		}
	}
}
