package accounts

import (
	"testing"
)

// route: GET /api/v1/users/invites
// route: DELETE /api/v1/users/invites/{id}
func TestInviteListAndTakeBack(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	tok := invite(t, f, own, `{"note":"for ana"}`)
	resp, out := own.do(t, f, "GET", "/api/v1/users/invites", "")
	if resp.StatusCode != 200 {
		t.Fatalf("list: %d %s", resp.StatusCode, out)
	}
	var l struct {
		Invites []InviteView `json:"invites"`
	}
	jsonOf(t, out, &l)
	if len(l.Invites) != 1 || l.Invites[0].Note != "for ana" {
		t.Fatalf("invites %+v", l.Invites)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/invites/"+l.Invites[0].ID, ""); resp.StatusCode != 204 {
		t.Fatalf("take back: %d", resp.StatusCode)
	}
	if resp, _ := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/invites/"+tok, "", nil); resp.StatusCode != 404 {
		t.Fatalf("a taken-back invite still opens: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "DELETE", "/api/v1/users/invites/"+l.Invites[0].ID, ""); resp.StatusCode != 404 {
		t.Fatalf("second take back: %d", resp.StatusCode)
	}
}

// route: POST /api/v1/auth/two-factor/backup-codes
func TestNewBackupCodes(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	secret, old := turnOnTwoFactor(t, f, own)
	if resp, _ := own.do(t, f, "POST", "/api/v1/auth/two-factor/backup-codes", `{"code":"000000"}`); resp.StatusCode != 401 {
		t.Fatalf("wrong code: %d", resp.StatusCode)
	}
	f.clock.add(31e9) // the next 30-second step, so the code is new
	resp, out := own.do(t, f, "POST", "/api/v1/auth/two-factor/backup-codes", `{"code":"`+codeNow(t, f, secret, 0)+`"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("new codes: %d %s", resp.StatusCode, out)
	}
	var b struct {
		Codes []string `json:"backup_codes"`
	}
	jsonOf(t, out, &b)
	if len(b.Codes) != backupCodeCount || b.Codes[0] == old[0] {
		t.Fatalf("codes %v (old %v)", b.Codes, old)
	}
}
