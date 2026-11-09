package secretsfile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// The addic7ed entry makes an Addic7ed site with the account in the vault;
// a second run with the same values changes nothing.
func TestImportAddic7ed(t *testing.T) {
	d := deps(t)
	ctx := context.Background()
	f := parse(t, "version: 1\naddic7ed:\n  username: lab-person\n  password: 'your-addic7ed-password'\n")
	ls := Apply(ctx, d, f, false)
	if r := results(ls); r["addic7ed"] != Set {
		t.Fatalf("%v", r)
	}
	noValues(t, d, f, ls)
	rows, err := subtitles.Sites(ctx, d.Store.Read())
	if err != nil || len(rows) != 1 || rows[0].Kind != "addic7ed" || rows[0].SecretRef == nil {
		t.Fatalf("%+v %v", rows, err)
	}
	raw, err := d.Vault.Get(ctx, *rows[0].SecretRef)
	var a subtitles.Account
	if err != nil || json.Unmarshal([]byte(raw), &a) != nil || a.Username != "lab-person" || a.Password != "your-addic7ed-password" {
		t.Fatalf("account %+v %v", a, err)
	}
	if rows[0].Limits.GapMS < 3000 || rows[0].Limits.WindowSeconds != 86400 {
		t.Fatalf("limits %+v", rows[0].Limits)
	}
	if r := results(Apply(ctx, d, f, false)); r["addic7ed"] != Unchanged {
		t.Fatalf("again %v", r)
	}
}
