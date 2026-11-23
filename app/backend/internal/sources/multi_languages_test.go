package sources

import (
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Source's MULTi list is empty by default, cleaned when sent and kept when not sent.
func TestSourceMultiLanguagesInput(t *testing.T) {
	var src domain.Source
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api"}, &src, true); err != nil {
		t.Fatal(err)
	}
	if src.MultiLanguages == nil || len(src.MultiLanguages) != 0 {
		t.Fatalf("new Source list = %#v, want empty", src.MultiLanguages)
	}
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api", MultiLanguages: []string{" FR", "fr", "xx", "de", ""}}, &src, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(src.MultiLanguages, []string{"fr", "de"}) {
		t.Fatalf("list = %v, want [fr de]", src.MultiLanguages)
	}
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api"}, &src, false); err != nil || !slices.Equal(src.MultiLanguages, []string{"fr", "de"}) {
		t.Fatalf("unsent list changed: %v %v", src.MultiLanguages, err)
	}
}
