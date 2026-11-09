package migrate

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// The UI relies on these codes for its "try again" and "too big" text.
func TestFailStatusCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		code int
	}{
		{ErrBackupBusy, 409}, {ErrHeldKey, 500}, {ErrSnapshotTooBig, 413}, {ErrTooBig, 413},
		{fmt.Errorf("x: %w", ErrUnreachable), 502}, {context.DeadlineExceeded, 504},
		{ErrWrongApp, 422}, {errors.New("other"), 400},
	} {
		w := httptest.NewRecorder()
		fail(w, c.err)
		if w.Code != c.code {
			t.Errorf("%v: %d, want %d", c.err, w.Code, c.code)
		}
		if errors.Is(c.err, ErrSnapshotTooBig) && !strings.Contains(w.Body.String(), ErrTooBig.Error()) {
			t.Errorf("too big body %s", w.Body.String())
		}
	}
}
