package parts

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}
