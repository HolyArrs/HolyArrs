package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestNestedWriteFailsInsteadOfDeadlocking(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var inner error
	err := s.Write(ctx, func(*sql.Tx) error {
		inner = s.Write(ctx, func(*sql.Tx) error { return nil })
		return nil
	})
	if err != nil || !errors.Is(inner, ErrNestedWrite) {
		t.Fatalf("outer %v, inner %v", err, inner)
	}
}
