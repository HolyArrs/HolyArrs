package mediafiles

import (
	"strings"
	"unicode/utf8"
)

// maxBlockedGroups caps the blocked groups one set of Pick rules holds.
const maxBlockedGroups = 500

// GroupBlocked says whether group is one of the blocked groups: the whole
// name in any case, never a part of it. An empty group never matches.
func GroupBlocked(blocked []string, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return false
	}
	for _, b := range blocked {
		if strings.EqualFold(strings.TrimSpace(b), group) {
			return true
		}
	}
	return false
}

// AddBlockedGroup adds group to the list unless it is empty or already there
// in any case, and says whether it was added.
func AddBlockedGroup(blocked []string, group string) ([]string, bool) {
	group = strings.TrimSpace(group)
	if group == "" || GroupBlocked(blocked, group) {
		return blocked, false
	}
	return append(blocked, group), true
}

func checkBlockedGroups(list []string) error {
	if len(list) > maxBlockedGroups {
		return fieldErr("body.blocked_groups", "List at most 500 blocked groups.")
	}
	for i, g := range list {
		g = strings.TrimSpace(g)
		if g == "" || !utf8.ValidString(g) || utf8.RuneCountInString(g) > pickMaxText {
			return fieldErr("body.blocked_groups", "Each blocked group holds 1 to 200 characters.")
		}
		if GroupBlocked(list[:i], g) {
			return fieldErr("body.blocked_groups", "The group %q is listed twice.", g)
		}
	}
	return nil
}
