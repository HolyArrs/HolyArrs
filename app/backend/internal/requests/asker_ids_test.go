package requests

// askerIDs reads the asked_by_ids an Event saved, as numbers.
func askerIDs(v any) []int64 {
	var out []int64
	switch l := v.(type) {
	case []any:
		for _, x := range l {
			if f, ok := x.(float64); ok {
				out = append(out, int64(f))
			}
		}
	case []int64:
		out = append(out, l...)
	}
	return out
}
