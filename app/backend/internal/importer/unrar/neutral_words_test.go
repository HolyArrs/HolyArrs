package unrar

import (
	"strings"
	"testing"
)

// TestErrors_NeutralWords checks the shared errors read well for any archive type.
func TestErrors_NeutralWords(t *testing.T) {
	for _, err := range []error{ErrEncrypted, ErrUnsafePath, ErrCorrupt, ErrUnrecognised, ErrTooBig} {
		if strings.Contains(err.Error(), "RAR") {
			t.Errorf("%q names RAR; it is shared with zip, 7z and tar", err)
		}
	}
}
