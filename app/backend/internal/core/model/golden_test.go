package model

import (
	"reflect"
	"strings"
	"testing"
)

// fieldLayout lists a record's fields in order, with their tags.
func fieldLayout(v any) string {
	t := reflect.TypeOf(v)
	var b strings.Builder
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		typ := strings.NewReplacer("model.", "", "domain.", "", "database.", "", "jobs.", "").Replace(f.Type.String())
		b.WriteString(f.Name + " " + typ + " `" + string(f.Tag) + "`\n")
	}
	return b.String()
}

func TestSharedRecordLayoutIsFrozen(t *testing.T) {
	for name, want := range goldenLayouts {
		var v any
		switch name {
		case "Actor":
			v = Actor{}
		case "Meta":
			v = Meta{}
		case "Subject":
			v = Subject{}
		case "Event":
			v = Event{}
		case "Job":
			v = Job{}
		case "JobError":
			v = JobError{}
		case "Schedule":
			v = Schedule{}
		}
		if got := fieldLayout(v); got != want {
			t.Errorf("%s layout changed:\n%s\nwant:\n%s", name, got, want)
		}
	}
}
