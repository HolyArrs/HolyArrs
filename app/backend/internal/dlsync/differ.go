package dlsync

import (
	"reflect"
	"sort"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// Changes is what moved between two reads of one client.
type Changes struct {
	Added       []string
	Removed     []string
	Changed     []FieldChange
	Transitions []Transition
}

// FieldChange names the fields of one item that changed.
type FieldChange struct {
	ID     string
	Fields []string
}

// Transition is an item whose state word moved.
type Transition struct {
	ID, From, To string
}

// Empty says nothing moved.
func (c Changes) Empty() bool {
	return len(c.Added) == 0 && len(c.Removed) == 0 && len(c.Changed) == 0
}

var jobFields = func() []reflect.StructField {
	t := reflect.TypeFor[downloader.Job]()
	out := make([]reflect.StructField, 0, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if f.IsExported() && f.Name != "ID" {
			out = append(out, f)
		}
	}
	return out
}()

// Diff compares two reads, item by item and field by field. It is pure:
// same input, same output, sorted by id.
func Diff(prev, cur map[string]downloader.Job) Changes {
	var c Changes
	for id, j := range cur {
		p, ok := prev[id]
		if !ok {
			c.Added = append(c.Added, id)
			continue
		}
		if fields := changedFields(p, j); len(fields) > 0 {
			c.Changed = append(c.Changed, FieldChange{ID: id, Fields: fields})
			if p.State != j.State {
				c.Transitions = append(c.Transitions, Transition{ID: id, From: p.State, To: j.State})
			}
		}
	}
	for id := range prev {
		if _, ok := cur[id]; !ok {
			c.Removed = append(c.Removed, id)
		}
	}
	sort.Strings(c.Added)
	sort.Strings(c.Removed)
	sort.Slice(c.Changed, func(i, k int) bool { return c.Changed[i].ID < c.Changed[k].ID })
	sort.Slice(c.Transitions, func(i, k int) bool { return c.Transitions[i].ID < c.Transitions[k].ID })
	return c
}

func changedFields(a, b downloader.Job) []string {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	var out []string
	for _, f := range jobFields {
		x, y := va.FieldByIndex(f.Index), vb.FieldByIndex(f.Index)
		switch f.Type.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
		default:
			if !x.Equal(y) {
				out = append(out, f.Name)
			}
			continue
		}
		if !reflect.DeepEqual(x.Interface(), y.Interface()) {
			out = append(out, f.Name)
		}
	}
	return out
}
