package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// query reads only the Library, search, download and History tables:
// never settings, sign-in tables, Sources or Downloaders (their addresses
// may carry keys), nor the schema.
func TestQueryReadsOnlyListedTables(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	ctx := context.Background()
	var out bytes.Buffer
	if err := query(ctx, dir, "SELECT count(*) FROM titles t JOIN versions v ON v.title_id = t.id", &out); err != nil {
		t.Fatalf("listed tables refused: %v", err)
	}
	for _, q := range []string{
		"SELECT * FROM settings",
		"SELECT * FROM Users",
		`SELECT * FROM "api_keys"`,
		"SELECT * FROM [sessions]",
		"SELECT * FROM sources",
		"SELECT url FROM downloaders",
		"SELECT name FROM sqlite_master",
		"SELECT * FROM pragma_table_info('secrets')",
		"SELECT * FROM titles WHERE id IN (SELECT id FROM user_links)",
	} {
		if err := query(ctx, dir, q, &out); err == nil || !strings.Contains(err.Error(), "not read here") {
			t.Errorf("%s: %v", q, err)
		}
	}
}
