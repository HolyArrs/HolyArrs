package ai

import "testing"

// The name from initialize wins over a User-Agent seen later; the
// User-Agent fills the name when initialize was skipped.
func TestRememberClientKeepsInitName(t *testing.T) {
	h := &Handlers{}
	h.rememberClient(1, "Claude Desktop")
	h.rememberClient(1, clientFromAgent("claude-code/2.1 node"))
	if got := h.clientOf(1); got != "Claude Desktop" {
		t.Fatalf("known name overwritten: %q", got)
	}
	h.rememberClient(2, clientFromAgent("claude-code/2.1 node"))
	if got := h.clientOf(2); got != "claude-code/2.1" {
		t.Fatalf("agent fallback: %q", got)
	}
	if got := h.clientOf(3); got != "an AI app" {
		t.Fatalf("unknown: %q", got)
	}
}
