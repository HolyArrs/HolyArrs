package unrar

import (
	"strings"
	"testing"
)

// The archive package (zip, 7z, tar) returns these errors too, so their
// words fit any archive, not only RAR.
func TestSharedErrorsUseNeutralWords(t *testing.T) {
	for _, err := range []error{ErrEncrypted, ErrUnsafePath, ErrCorrupt, ErrUnrecognised, ErrTooBig} {
		if strings.Contains(err.Error(), "RAR") {
			t.Errorf("%q names RAR; it is shared with zip, 7z and tar", err)
		}
	}
}
