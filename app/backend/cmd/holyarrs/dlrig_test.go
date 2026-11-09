package main

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func dlServer(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Secrets: v}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}
