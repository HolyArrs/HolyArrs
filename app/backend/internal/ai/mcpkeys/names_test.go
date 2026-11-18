package mcpkeys

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// accounts stores and checks the tick boxes by name; they must be these
// groups, and the same default.
func TestAccountsKnowTheGroups(t *testing.T) {
	names := func(s Set) []string {
		out := []string{}
		for _, g := range s {
			out = append(out, string(g))
		}
		return out
	}
	if !slices.Equal(accounts.MCPGroups, names(All)) || !slices.Equal(accounts.MCPDefault, names(Default)) {
		t.Fatalf("accounts %v / %v, mcpkeys %v / %v", accounts.MCPGroups, accounts.MCPDefault, All, Default)
	}
}
