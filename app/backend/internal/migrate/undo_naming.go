package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// undoNaming puts back the naming keys one item moved, unless they were
// changed since.
func undoNaming(ctx context.Context, st *database.Store, before, after string, by domain.Actor) (string, string) {
	var moved map[string]string
	old, oldErr := mediafiles.NamingFromJSON([]byte(before))
	if after == "" || json.Unmarshal([]byte(after), &moved) != nil || oldErr != nil {
		return ResKept, keptUnknown
	}
	cur, err := mediafiles.ReadNaming(ctx, st.Read())
	if err != nil {
		return ResFailed, err.Error()
	}
	same := cur
	for k, v := range moved {
		setNaming(&same, k, v)
	}
	if !reflect.DeepEqual(same, cur) {
		return ResKept, keptChanged
	}
	// Only the moved keys go back: two naming items of one move each undo
	// their own keys, so a later change to another key stays.
	var was map[string]any
	raw, _ := json.Marshal(old)
	_ = json.Unmarshal(raw, &was)
	next := cur
	for k := range moved {
		if v, ok := was[k]; ok {
			setNaming(&next, k, fmt.Sprint(v))
		}
	}
	if err := mediafiles.PutNamingSettings(ctx, st, next, by); err != nil {
		return ResFailed, err.Error()
	}
	return ResRemoved, "naming is back as it was"
}
