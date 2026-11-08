package library

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCopyChoicesKeepEditionAndAudioIndependent(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	main := versionsOf(t, s, id)[0]
	if main.Edition != "" || len(main.AudioLanguages) != 0 {
		t.Fatalf("old copy was relabeled: %+v", main)
	}
	for _, in := range []VersionInput{
		{Label: "Theatrical HD", Edition: "Theatrical", AudioLanguages: []string{"eng"}},
		{Label: "Theatrical 4K", Edition: "Theatrical", PickRulesID: fourKRules(t, s), AudioLanguages: []string{"en", "fra", "en"}},
		{Label: "Directors cut", Edition: "Director's Cut", AudioLanguages: []string{"ja"}},
	} {
		v, err := AddVersion(ctx, s, id, in, nil, now, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if v.Edition != in.Edition || len(v.AudioLanguages) == 0 {
			t.Fatalf("choices lost: %+v", v)
		}
	}
	vs := versionsOf(t, s, id)
	if len(vs) != 4 || vs[1].AudioLanguages[0] != "en" || len(vs[2].AudioLanguages) != 2 || vs[2].AudioLanguages[1] != "fr" {
		t.Fatalf("normalization: %+v", vs)
	}
	seen := map[string]bool{}
	for _, v := range vs {
		if seen[v.FolderName] {
			t.Fatalf("shared destination: %s", v.FolderName)
		}
		seen[v.FolderName] = true
	}
	bad := []VersionInput{{Label: "bad", Edition: "../Cut"}, {Label: "bad", Edition: "{edition-hack}"}, {Label: "bad", AudioLanguages: []string{"und"}}, {Label: "bad", AudioLanguages: []string{"not-a-language"}}}
	for _, in := range bad {
		if _, err := AddVersion(ctx, s, id, in, nil, now, domain.System); err == nil {
			t.Fatalf("accepted %+v", in)
		}
	}
}
