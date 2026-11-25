package mediafiles

// SlotKept says whether a Slot is wanted: its own choice when one was made
// for this Version, else whether its Item (with its season or album) and
// its Version are both kept up to date. A dubbed Version that is not kept
// up to date can so want only the episodes asked for in it, and a Version
// can leave out an episode the others want.
func SlotKept(sl Slot, itemKept, versionKept bool) bool {
	if sl.KeepUpToDate != nil {
		return *sl.KeepUpToDate
	}
	return itemKept && versionKept
}

// SlotKeptSQL is the SQL of SlotKept for the Slot s, the Item i and the
// Version v, with itemKept the SQL saying the Item is kept up to date.
func SlotKeptSQL(itemKept string) string {
	return "COALESCE(s.keep_up_to_date, (" + itemKept + ") AND v.keep_up_to_date = 1) = 1"
}
