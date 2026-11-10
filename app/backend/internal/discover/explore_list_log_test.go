package discover

import (
	"errors"
	"testing"
	"time"
)

func TestListLogSaysEachErrorOnceInAWhile(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	refused := errors.New("the metadata server refused this install")
	if !listLogDue(refused, now) {
		t.Fatal("the first failure is logged")
	}
	if listLogDue(refused, now.Add(3*time.Second)) {
		t.Fatal("the same failure 3 s later is not logged again")
	}
	if !listLogDue(errors.New("another failure"), now.Add(3*time.Second)) {
		t.Fatal("a different failure is logged")
	}
	if !listLogDue(refused, now.Add(6*time.Minute)) {
		t.Fatal("the same failure is logged again after five minutes")
	}
}
