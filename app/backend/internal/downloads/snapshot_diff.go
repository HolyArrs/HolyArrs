package downloads

import (
	"reflect"
	"sort"
)

// jobFields lists the Job fields compared by Diff, found once by reflection
// so a new Job field is compared without touching this file.
var jobFields = func() []reflect.StructField {
	t := reflect.TypeFor[Job]()
	out := make([]reflect.StructField, 0, t.NumField())
	for i := range t.NumField() {
		f := t.Field(i)
		if f.IsExported() && f.Name != "ID" {
			out = append(out, f)
		}
	}
	return out
}()

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

// Empty reports whether nothing was added, removed or changed.
func (c Changes) Empty() bool {
	return len(c.Added) == 0 && len(c.Removed) == 0 && len(c.Changed) == 0
}

// Diff compares two reads item by item and field by field. The result is
// sorted by id, so the same input always gives the same output.
func Diff(prev, cur map[string]Job) Changes {
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

// changedFields returns the names of the fields that differ between two
// versions of one job. Pointers, slices and maps need a deep compare.
func changedFields(a, b Job) []string {
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
