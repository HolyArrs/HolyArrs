package mediafiles

import (
	"reflect"
	"testing"
)

func TestClean(t *testing.T) {
	got := CleanTags([]string{" Kids ", "kids", "4K  Only", "", ",", "anime,"})
	if want := []string{"4k-only", "anime", "kids"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := CleanTags(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil in: %#v", got)
	}
	long := CleanTags([]string{"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"})
	if len([]rune(long[0])) != MaxLen {
		t.Fatalf("not cut: %q", long[0])
	}
}

type dl struct {
	name string
	tags []string
}

func names(ds []dl) []string {
	out := []string{}
	for _, d := range ds {
		out = append(out, d.name)
	}
	return out
}

func TestDownloadersFollowTheOldAppsRule(t *testing.T) {
	all := []dl{{"plain", nil}, {"kids", []string{"kids"}}, {"4k", []string{"4k"}}, {"both", []string{"kids", "4k"}}}
	of := func(d dl) []string { return d.tags }
	for _, c := range []struct {
		title []string
		want  []string
	}{
		{nil, []string{"plain"}},                     // no tags: untagged Downloaders only
		{[]string{"kids"}, []string{"kids", "both"}}, // a shared tag wins
		{[]string{"anime"}, []string{"plain"}},       // a tag no Downloader has: untagged ones
		{[]string{"4k", "kids"}, []string{"kids", "4k", "both"}},
	} {
		if got := names(Downloaders(all, c.title, of)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("title %v: %v, want %v", c.title, got, c.want)
		}
	}
	if got := Downloaders([]dl{{"kids", []string{"kids"}}}, []string{"anime"}, of); len(got) != 0 {
		t.Fatalf("only tagged Downloaders and no shared tag: %v", names(got))
	}
}

func TestSourceServes(t *testing.T) {
	if !SourceServes(nil, nil) || !SourceServes(nil, []string{"kids"}) || !SourceServes([]string{"kids"}, []string{"kids", "4k"}) {
		t.Fatal("should serve")
	}
	if SourceServes([]string{"kids"}, nil) || SourceServes([]string{"kids"}, []string{"anime"}) {
		t.Fatal("should not serve")
	}
}
