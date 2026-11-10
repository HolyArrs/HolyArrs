package library

import (
	"encoding/json"
	"testing"
)

// TestTrashSizeSpec: a size test syncs as a size trait test in GB, either
// end open; one with no usable ends leaves the format out.
func TestTrashSizeSpec(t *testing.T) {
	cases := []struct {
		fields string
		want   string
		bad    bool
	}{
		{`{"min":1,"max":9}`, "1-9", false},
		{`{"min":60,"max":0}`, "60-", false},
		{`{"min":0,"max":8.5}`, "-8.5", false},
		{`{"min":0,"max":0}`, "", true},
		{`{"min":9,"max":2}`, "", true},
	}
	for _, c := range cases {
		var s TrashSpec
		raw := `{"name":"Size","implementation":"SizeSpecification","negate":false,"required":true,"fields":` + c.fields + `}`
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			t.Fatal(err)
		}
		out, kinds := trashConvert("radarr", []TrashSpec{s})
		if c.bad {
			if len(kinds) != 1 || kinds[0] != "size" || len(out) != 0 {
				t.Errorf("%s: got %+v %v, want left out", c.fields, out, kinds)
			}
			continue
		}
		if len(kinds) != 0 || len(out) != 1 || out[0].Kind != "size" || out[0].Value != c.want || !out[0].Required {
			t.Errorf("%s: got %+v %v, want size %s", c.fields, out, kinds, c.want)
		}
	}
}
