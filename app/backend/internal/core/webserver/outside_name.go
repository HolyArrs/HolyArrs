package webserver

import (
	"context"
	"strconv"
	"strings"
)

// OutsideName is a person's name as anything outside the app sees it: a
// deleted person's stored name (their name, an invisible separator and their
// id) becomes "name (deleted)"; any other name comes back as it is.
func OutsideName(name string) string {
	i := strings.LastIndex(name, "⁣")
	if i < 0 {
		return name
	}
	if _, err := strconv.ParseInt(name[i+len("⁣"):], 10, 64); err != nil {
		return name
	}
	return strings.TrimSpace(name[:i]) + " (deleted)"
}

// NameFor is name for this request: an API key gets the outside form, while
// the app keeps the stored name it draws the deleted look from.
func NameFor(ctx context.Context, name string) string {
	if c, ok := From(ctx); ok && c.KeyID != 0 {
		return OutsideName(name)
	}
	return name
}
