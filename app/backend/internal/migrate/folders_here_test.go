package migrate

import (
	"reflect"
	"testing"
)

func TestHereFoldersAddsOneLevelInside(t *testing.T) {
	sub := map[string][]string{"/data": {"/data/Movies", "/data/TV Shows"}, "/data/Movies": {"/data/Movies/A"}}
	got := hereFolders([]string{"/data", "/data/Movies"}, func(p string) []string { return sub[p] })
	want := []string{"/data", "/data/Movies", "/data/Movies/A", "/data/TV Shows"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
