package migrate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// A backup keeps each format condition as {"type": kind, "body": {...}}
// with the field values flat in the body; the API answers the same
// condition with "implementation" and a "fields" list. Both must read the same.
func TestFormatConditionsBackupMatchAPI(t *testing.T) {
	cases := []struct {
		app, version, api, backup string
	}{
		{"sonarr", "4", `{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":true,"fields":[{"name":"value","value":"\bx265\b"}]}`,
			`{"type":"ReleaseTitleSpecification","body":{"order":1,"implementationName":"Release Title","infoLink":"https://x","value":"\bx265\b","name":"x265","negate":false,"required":true}}`},
		{"sonarr", "4", `{"name":"French","implementation":"LanguageSpecification","negate":true,"required":false,"fields":[{"name":"value","value":2}]}`,
			`{"type":"LanguageSpecification","body":{"value":2,"name":"French","negate":true,"required":false}}`},
		{"sonarr", "5", `{"name":"Not English","implementation":"LanguageSpecification","negate":false,"required":false,"fields":[{"name":"value","value":1},{"name":"exceptLanguage","value":true}]}`,
			`{"type":"LanguageSpecification","body":{"value":1,"exceptLanguage":true,"name":"Not English","negate":false,"required":false}}`},
		{"sonarr", "4", `{"name":"Nzb","implementation":"IndexerFlagSpecification","negate":false,"required":false,"fields":[{"name":"value","value":4}]}`,
			`{"type":"IndexerFlagSpecification","body":{"value":4,"name":"Nzb","negate":false,"required":false}}`},
		{"sonarr", "4", `{"name":"Web","implementation":"SourceSpecification","negate":false,"required":false,"fields":[{"name":"value","value":3}]}`,
			`{"type":"SourceSpecification","body":{"value":3,"name":"Web","negate":false,"required":false}}`},
		{"sonarr", "4", `{"name":"2160p","implementation":"ResolutionSpecification","negate":false,"required":true,"fields":[{"name":"value","value":2160}]}`,
			`{"type":"ResolutionSpecification","body":{"value":2160,"name":"2160p","negate":false,"required":true}}`},
		{"sonarr", "4", `{"name":"Big","implementation":"SizeSpecification","negate":false,"required":false,"fields":[{"name":"min","value":1.5},{"name":"max","value":40}]}`,
			`{"type":"SizeSpecification","body":{"min":1.5,"max":40,"name":"Big","negate":false,"required":false}}`},
		{"sonarr", "4", `{"name":"Group","implementation":"ReleaseGroupSpecification","negate":false,"required":false,"fields":[{"name":"value","value":"^(NTb)$"}]}`,
			`{"type":"ReleaseGroupSpecification","body":{"value":"^(NTb)$","name":"Group","negate":false,"required":false}}`},
		{"sonarr", "4", `{"name":"Pack","implementation":"ReleaseTypeSpecification","negate":false,"required":false,"fields":[{"name":"value","value":3}]}`,
			`{"type":"ReleaseTypeSpecification","body":{"value":3,"name":"Pack","negate":false,"required":false}}`},
		{"radarr", "6", `{"name":"IMAX","implementation":"EditionSpecification","negate":false,"required":false,"fields":[{"name":"value","value":"imax"}]}`,
			`{"type":"EditionSpecification","body":{"value":"imax","name":"IMAX","negate":false,"required":false}}`},
		{"radarr", "6", `{"name":"Remux","implementation":"QualityModifierSpecification","negate":false,"required":false,"fields":[{"name":"value","value":5}]}`,
			`{"type":"QualityModifierSpecification","body":{"value":5,"name":"Remux","negate":false,"required":false}}`},
		{"radarr", "6", `{"name":"Old","implementation":"YearSpecification","negate":true,"required":false,"fields":[{"name":"min","value":1900},{"name":"max","value":1999}]}`,
			`{"type":"YearSpecification","body":{"min":1900,"max":1999,"name":"Old","negate":true,"required":false}}`},
		{"radarr", "6", `{"name":"x265","implementation":"ReleaseTitleSpecification","negate":false,"required":false,"fields":[{"name":"value","value":"x265"}]}`,
			`{"type":"ReleaseTitleSpecification","body":{"value":"x265","name":"x265","negate":false,"required":false}}`},
		{"radarr", "3", `{"name":"Bluray","implementation":"SourceSpecification","negate":false,"required":true,"fields":[{"name":"value","value":9}]}`,
			`{"Type":"SourceSpecification","Body":{"Value":9,"Name":"Bluray","Negate":false,"Required":true}}`},
		{"radarr", "3", `{"name":"Small","implementation":"SizeSpecification","negate":false,"required":false,"fields":[{"name":"min","value":0},{"name":"max","value":2}]}`,
			`{"Type":"SizeSpecification","Body":{"Min":0,"Max":2,"Name":"Small","Negate":false,"Required":false}}`},
	}
	for _, c := range cases {
		api, backup := list("["+c.api+"]"), list("["+c.backup+"]")
		if len(api) != 1 || len(backup) != 1 {
			t.Fatalf("%s %s: bad json", c.app, c.version)
		}
		want, got := specItem(api[0]), specItem(backup[0])
		if len(want) <= 4 {
			t.Fatalf("%s %s: api item has no values %v", c.app, c.version, want)
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s %s %s:\n api    %v\n backup %v", c.app, c.version, want["kind"], want, got)
		}
	}
}

// The fixtures are one format rule with every condition kind, made in the
// lab apps (Sonarr 4.0.20, Radarr 6.4.4): the API answer and the same rule's
// column from a real backup made with the Backup command.
func TestFormatConditionsRealBackupFixtures(t *testing.T) {
	for _, app := range []string{"sonarr", "radarr"} {
		api, err1 := os.ReadFile(filepath.Join("testdata", "cf_api_"+app+".json"))
		backup, err2 := os.ReadFile(filepath.Join("testdata", "cf_backup_"+app+".json"))
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		a, b := list(string(api)), list(string(backup))
		if len(a) < 8 || len(a) != len(b) {
			t.Fatalf("%s: %d api, %d backup conditions", app, len(a), len(b))
		}
		for i := range a {
			want, got := specItem(a[i]), specItem(b[i])
			if len(want) <= 4 || !reflect.DeepEqual(want, got) {
				t.Errorf("%s %v:\n api    %v\n backup %v", app, want["kind"], want, got)
			}
		}
	}
}
