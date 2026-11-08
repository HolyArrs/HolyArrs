package health

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Test folders live in the test machine's temp dir, which inside a CI
	// container is never mapped; the unmapped check has its own tests.
	unmappedConfig = func(string) string { return "" }
	os.Exit(m.Run())
}
