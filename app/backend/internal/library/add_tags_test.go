package library

import (
	"context"
	"reflect"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Tags typed when a movie is added go onto it next to its Library folder's
// tags for new titles, cleaned as one list.
func TestAddMovieTakesTypedTags(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Cartoons", NewTitleTags: []string{"kids"}}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	id, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: lf.ID, Tags: []string{" UA Dub ", "Kids", "toloka"}}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tagsOfTitle(t, s, id), []string{"kids", "toloka", "ua-dub"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tags %v, want %v", got, want)
	}

	// With none typed the movie has the folder's tags, as before.
	other, err := AddMovie(ctx, s, future, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsOfTitle(t, s, other); !reflect.DeepEqual(got, []string{"kids"}) {
		t.Fatalf("tags %v, want the folder's", got)
	}
}
