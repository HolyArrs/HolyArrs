package shows

import (
	"context"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Tags typed when a show is added go onto it, cleaned like every other tag.
func TestAddShowTakesTypedTags(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{Tags: []string{"Toloka", " UA Dub "}}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var title mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", id, &title); err != nil {
		t.Fatal(err)
	}
	if want := []string{"toloka", "ua-dub"}; !reflect.DeepEqual(title.Tags, want) {
		t.Fatalf("tags %v, want %v", title.Tags, want)
	}
}
