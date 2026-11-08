package importer

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestCopyRequirementsNeverUseLanguageLabelsAsAudioProof(t *testing.T) {
	v := mediafiles.Version{Edition: "Director's Cut", AudioLanguages: []string{"en", "fr"}}
	name := "Harbor.of.Glass.2018.Directors.Cut.1080p.BluRay.ENGLISH.FRENCH"
	facts := map[string]any{"inspection_status": "complete", "audio_languages": []string{"en"}}
	if why := copyRefusal(v, name, name, "", facts); why == "" {
		t.Fatal("release label supplied uninspected French audio")
	}
	facts["audio_languages"] = []string{"en", "fr", "ja"}
	if why := copyRefusal(v, name, name, "", facts); why != "" {
		t.Fatal(why)
	}
	if why := copyRefusal(v, "Harbor.of.Glass.2018.Theatrical.1080p.BluRay", "", "", facts); why == "" {
		t.Fatal("theatrical replaced director's cut")
	}
	if why := copyRefusal(v, "Harbor.of.Glass.2018.1080p.BluRay", "", "", facts); why == "" {
		t.Fatal("unknown edition guessed")
	}
	if why := copyRefusal(v, "Harbor.of.Glass.2018.1080p.BluRay", "", "Director's Cut", facts); why != "" {
		t.Fatal(why)
	}
	if facts["edition_source"] != "confirmed" {
		t.Fatal("manual choice was not recorded")
	}
	if why := copyRefusal(mediafiles.Version{}, "unknown", "", "", map[string]any{}); why != "" {
		t.Fatal("old quality-only Version changed:", why)
	}
}

func TestMultipleEditionLabelsRequireExplicitConfirmation(t *testing.T) {
	v := mediafiles.Version{Edition: "Directors Cut Extended"}
	facts := map[string]any{}
	name := "Harbor.of.Glass.2018.Directors.Cut.Extended.1080p.BluRay"
	if why := copyRefusal(v, name, name, "", facts); !strings.Contains(why, "ambiguous") {
		t.Fatalf("multiple edition labels were accepted automatically: %q", why)
	}
	if why := copyRefusal(v, name, name, "Directors Cut Extended", facts); why != "" {
		t.Fatalf("explicit edition confirmation was refused: %s", why)
	}
	if facts["edition_source"] != "confirmed" {
		t.Fatalf("edition source = %v", facts["edition_source"])
	}
}

func TestMultipleEditionLabelsDoNotConstrainLegacyVersion(t *testing.T) {
	facts := map[string]any{}
	name := "Harbor.of.Glass.2018.Directors.Cut.Extended.1080p.BluRay"
	if why := copyRefusal(mediafiles.Version{}, name, name, "", facts); why != "" {
		t.Fatalf("unconstrained Version refused ambiguous labels: %s", why)
	}
	if edition := facts["edition"]; edition != "" {
		t.Fatalf("ambiguous labels were guessed as edition %q", edition)
	}
	if source := facts["edition_source"]; source != "unknown" {
		t.Fatalf("edition source = %v, want unknown", source)
	}
}
