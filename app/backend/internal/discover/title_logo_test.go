package discover

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestDetailsTextShowsLogoInReadersLanguage(t *testing.T) {
	d := metadata.Details{Logo: "en.png", Logos: map[string]string{"fr": "fr.png"}}
	detailsText(&d, metadata.TitleText{}, "fr")
	if d.Logo != "fr.png" || d.Logos != nil {
		t.Fatalf("fr: logo %q logos %v", d.Logo, d.Logos)
	}
	d = metadata.Details{Logo: "en.png", Logos: map[string]string{"fr": "fr.png"}}
	detailsText(&d, metadata.TitleText{}, "de")
	if d.Logo != "en.png" {
		t.Fatalf("de falls back to English, got %q", d.Logo)
	}
}

func TestDetailsTextFallsBackToOriginalLogo(t *testing.T) {
	d := metadata.Details{Logo: "en.png", Logos: map[string]string{"pt-br": "br.png", metadata.OrigLogo: "ja.png"}}
	detailsText(&d, metadata.TitleText{}, "pt-pt")
	if d.Logo != "ja.png" {
		t.Fatalf("pt-pt: got %q", d.Logo)
	}
	d = metadata.Details{Logo: "en.png", Logos: map[string]string{metadata.OrigLogo: "ja.png"}}
	detailsText(&d, metadata.TitleText{}, "en")
	if d.Logo != "en.png" {
		t.Fatalf("en reader keeps English, got %q", d.Logo)
	}
}

func TestDetailsTTLReadsPreLogoPagesOnce(t *testing.T) {
	vid := []metadata.DetailVideo{{Key: "k"}}
	if got := DetailsTTL(Entry{Details: &metadata.Details{Videos: vid}}); got != 0 {
		t.Fatalf("kept before logos: ttl %v", got)
	}
	if got := DetailsTTL(Entry{Details: &metadata.Details{Videos: vid, LogosRead: true}}); got != TTL["details"] {
		t.Fatalf("read with logos: ttl %v", got)
	}
}
