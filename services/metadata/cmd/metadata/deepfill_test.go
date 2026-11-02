package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func rgID(n int) string { return fmt.Sprintf("%08d-1234-1234-1234-123456789abc", n) }

// Deep fill (plan P4): a listed artist's 10 newest studio albums go at P2,
// older ones at P4; singles, live and compilations are skipped.
func TestDeepFillSplitsNewestAndOlder(t *testing.T) {
	var groups []string
	for i := 0; i < 14; i++ {
		groups = append(groups, fmt.Sprintf(`{"id":%q,"primary-type":"Album","secondary-types":[],"first-release-date":"%d-01-01"}`, rgID(i), 1990+i))
	}
	groups = append(groups,
		fmt.Sprintf(`{"id":%q,"primary-type":"Single","first-release-date":"2030"}`, rgID(90)),
		fmt.Sprintf(`{"id":%q,"primary-type":"Album","secondary-types":["Live"],"first-release-date":"2031"}`, rgID(91)))
	data := `{"source_data":{"release-groups":[` + strings.Join(groups, ",") + `]}}`
	newest, older := deepFill([]byte(data))
	if len(newest) != 10 || newest[0] != rgID(13) || len(older) != 4 || older[0] != rgID(3) {
		t.Fatalf("newest %v older %v", newest, older)
	}
	if slices.Contains(append(newest, older...), rgID(90)) || slices.Contains(append(newest, older...), rgID(91)) {
		t.Fatal("non-studio album queued")
	}
	if n, o := deepFill([]byte(`{}`)); n != nil || o != nil {
		t.Fatal("empty answer queued albums")
	}
}
