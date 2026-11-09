package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// A write that panics answers with the panic's type only; its text may
// hold a secret.
func TestWritePanicKeepsItsText(t *testing.T) {
	s, _ := openTest(t, Options{})
	err := s.Write(context.Background(), func(*sql.Tx) error { panic("private detail z4k") })
	if err == nil || strings.Contains(err.Error(), "z4k") || !strings.Contains(err.Error(), "string") {
		t.Fatalf("err %v", err)
	}
}
