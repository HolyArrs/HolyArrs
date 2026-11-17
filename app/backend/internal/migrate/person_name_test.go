package migrate

import "testing"

// An imported person's name never keeps the mark a deleted person's name
// carries.
func TestImportedPersonName(t *testing.T) {
	if got := personName("Ana⁣7"); got != "imported-user" {
		t.Fatalf("kept a hidden mark: %q", got)
	}
	if got := personName("Ana"); got != "Ana" {
		t.Fatalf("changed a plain name: %q", got)
	}
}
