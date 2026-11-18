package requests

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A line has the owner's own name beside the name people see. It is stored
// trimmed, kept to 30 characters, and never reaches the request sheet.
func TestLineOwnerLabel(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	everyday := rulesID(t, r, mediafiles.GoodEverydayName)
	saveQualities(t, r, func(q *QualityLists) {
		q.Movie = append([]QualityLine{{ID: "own-small", Name: "Small", Label: " Everyday 720p ", PickRulesID: everyday}}, q.Movie...)
		q.Movie[1].Label = "Remux"
	})
	back, _ := ReadSettings(ctx, r.st.Read())
	if back.Qualities.Movie[0].Label != "Everyday 720p" || back.Qualities.Movie[0].Name != "Small" || back.Qualities.Movie[1].Label != "Remux" {
		t.Fatalf("saved %+v", back.Qualities.Movie)
	}
	for _, c := range choicesOf(back.Qualities.Movie) {
		if strings.Contains(c.Name, "Everyday") || strings.Contains(c.Name, "Remux") {
			t.Fatalf("owner name reached the sheet: %+v", c)
		}
	}
	long := back
	long.Qualities = defaultQualities()
	long.Qualities.Show[0].Label = strings.Repeat("x", 31)
	if err := SaveSettings(ctx, r.st, long, domain.System); !errors.Is(err, ErrQualityName) {
		t.Fatalf("long label: %v", err)
	}
	// Lines stored before the owner's name existed read with none.
	if got := parseQualities(`{"movie":[{"id":"standard","name":"HD"}]}`); got.Movie[0].Label != "" || got.Movie[0].Name != "HD" {
		t.Fatalf("old line %+v", got.Movie[0])
	}
}
