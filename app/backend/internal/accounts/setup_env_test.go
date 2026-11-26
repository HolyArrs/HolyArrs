package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// envFile writes a value file and returns its path.
func envFile(t *testing.T, v string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "value")
	if err := os.WriteFile(p, []byte(v), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// clearEnvSetup unsets every env setup variable for the test.
func clearEnvSetup(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvOwnerName, EnvOwnerPassword, EnvAPIKey, EnvAPIKeyScopes} {
		t.Setenv(k, "")
	}
}

// applyEnv reads the variables as they are now and applies them.
func applyEnv(t *testing.T, a *Auth) error {
	t.Helper()
	e, err := ReadEnvSetup()
	if err != nil {
		return err
	}
	return a.ApplyEnvSetup(context.Background(), e)
}

func ownerRow(t *testing.T, f *fixture) (name, hash string) {
	t.Helper()
	err := f.s.Read().QueryRow("SELECT name, password_hash FROM users WHERE role = 'owner'").Scan(&name, &hash)
	if err != nil {
		t.Fatal(err)
	}
	return name, hash
}

func envKeyValue() string { return keyPrefix + token(32) }

// envKeyCount counts the keys HolyArrs made from the file.
func envKeyCount(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	by, _ := json.Marshal(domain.System)
	if err := f.s.Read().QueryRow("SELECT count(*) FROM api_keys WHERE label = ? AND created_by = ?", envKeyLabel, string(by)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEnvSetupMakesTheOwner(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	pw := "a long enough phrase"
	t.Setenv(EnvOwnerName, " river ")
	t.Setenv(EnvOwnerPassword, envFile(t, pw+"\n"))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	name, hash := ownerRow(t, f)
	if name != "river" || !CheckPassword(hash, pw) {
		t.Fatalf("owner %q, password matches %v", name, CheckPassword(hash, pw))
	}
	var action string
	if err := f.s.ReadEvents().QueryRow("SELECT json_extract(data, '$.action') FROM events WHERE kind = ?", EventAccountChanged).Scan(&action); err != nil || action != "owner_created_from_env" {
		t.Fatalf("owner event: %q %v", action, err)
	}
	// The wizard is closed: setup now answers as for any made owner.
	c := client(t)
	if resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"late","password":"another long phrase"}`, nil); resp.StatusCode != 409 {
		t.Fatalf("setup after env owner: %d %s", resp.StatusCode, body)
	}
	if resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"river","password":"`+pw+`"}`, nil); resp.StatusCode != 200 {
		t.Fatalf("sign-in as the env owner: %d %s", resp.StatusCode, body)
	}
}

// A second start, or an owner made in the wizard: nothing changes, least of
// all the password.
func TestEnvSetupLeavesAnOwnerAlone(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	c := client(t)
	if resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"first","password":"the first phrase"}`, nil); resp.StatusCode != 201 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	_, before := ownerRow(t, f)
	t.Setenv(EnvOwnerName, "second")
	t.Setenv(EnvOwnerPassword, envFile(t, "the second phrase"))
	for range 2 {
		if err := applyEnv(t, f.a); err != nil {
			t.Fatal(err)
		}
	}
	// A password that would be refused does not matter once the owner exists.
	t.Setenv(EnvOwnerPassword, envFile(t, "short"))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatalf("unused short password: %v", err)
	}
	name, after := ownerRow(t, f)
	if name != "first" || after != before || !CheckPassword(after, "the first phrase") {
		t.Fatalf("owner %q changed (hash same %v)", name, after == before)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM users").Scan(&n)
	if n != 1 {
		t.Fatalf("%d people, want only the owner", n)
	}
}

func TestEnvSetupNeedsBothOwnerVariables(t *testing.T) {
	for _, set := range []string{EnvOwnerName, EnvOwnerPassword} {
		clearEnvSetup(t)
		if set == EnvOwnerName {
			t.Setenv(EnvOwnerName, "river")
		} else {
			t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
		}
		_, err := ReadEnvSetup()
		if err == nil || !strings.Contains(err.Error(), EnvOwnerName) || !strings.Contains(err.Error(), EnvOwnerPassword) {
			t.Fatalf("only %s: %v", set, err)
		}
	}
}

// A refused name or password stops the start, names the variable and never
// shows the password.
func TestEnvSetupRefusesABadOwner(t *testing.T) {
	cases := []struct{ name, pw, want string }{
		{name: "river", pw: "short", want: EnvOwnerPassword + ": the password needs at least 8 characters"},
		{name: "river", pw: strings.Repeat("x", MaxPassword+1), want: EnvOwnerPassword + ": the password can have at most"},
		{name: "ri​ver", pw: "a long enough phrase", want: EnvOwnerName + ": names can't have hidden characters"},
		{name: strings.Repeat("n", 65), pw: "a long enough phrase", want: EnvOwnerName + ": pick a name"},
	}
	for _, c := range cases {
		clearEnvSetup(t)
		f := setup(t, Options{})
		t.Setenv(EnvOwnerName, c.name)
		t.Setenv(EnvOwnerPassword, envFile(t, c.pw))
		err := applyEnv(t, f.a)
		if err == nil || !strings.HasPrefix(err.Error(), c.want) || strings.Contains(err.Error(), c.pw) {
			t.Fatalf("%q: %v", c.want, err)
		}
		if _, _, ok := f.a.owner(context.Background()); ok {
			t.Fatalf("%q: an owner was made", c.want)
		}
	}
}

// Value files follow the FILE__ rule: one trailing newline goes, nothing else.
func TestEnvSetupValueFiles(t *testing.T) {
	for in, want := range map[string]string{"pw phrase\n": "pw phrase", "pw phrase\r\n": "pw phrase", " pw phrase \n\n": " pw phrase \n", "pw phrase": "pw phrase"} {
		got, err := readValueFile(EnvOwnerPassword, envFile(t, in))
		if err != nil || got != want {
			t.Fatalf("%q: %q %v", in, got, err)
		}
	}
	secret := "secret-ish phrase"
	for _, c := range []struct{ path, want string }{
		{envFile(t, "\n"), "is empty"},
		{envFile(t, strings.Repeat(secret, 300)), "is over 4096 bytes"},
		{filepath.Join(t.TempDir(), "missing"), "cannot read the file"},
		{t.TempDir(), "is not a regular file"},
	} {
		_, err := readValueFile(EnvOwnerPassword, c.path)
		if err == nil || !strings.HasPrefix(err.Error(), EnvOwnerPassword+":") || !strings.Contains(err.Error(), c.want) || strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: %v", c.want, err)
		}
	}
	// The key file follows the same rule, so `echo hak_... > file` works.
	clearEnvSetup(t)
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k+"\r\n"))
	e, err := ReadEnvSetup()
	if err != nil || e.key != k {
		t.Fatalf("key with CR LF: %v", err)
	}
}

func TestEnvSetupMakesTheKey(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k+"\n"))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	who, ok := f.a.checkKey(ctx, k)
	if !ok || who.Role != webserver.RoleOwner || who.KeyLabel != envKeyLabel || !slices.Equal(who.Scopes, []string{webserver.ScopeAll}) {
		t.Fatalf("checkKey: %v %+v", ok, who)
	}
	// The key works on the API like one made in Settings.
	resp, body := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/keys", "", map[string]string{APIKeyHeader: k})
	if resp.StatusCode != 200 || !strings.Contains(body, envKeyLabel) || !strings.Contains(body, k[len(k)-4:]) || strings.Contains(body, k) {
		t.Fatalf("keys list: %d %s", resp.StatusCode, body)
	}

	// Same file on the next start: the same key, untouched.
	var id int64
	_ = f.s.Read().QueryRow("SELECT id FROM api_keys WHERE hash = ?", sum(k)).Scan(&id)
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	var again int64
	_ = f.s.Read().QueryRow("SELECT id FROM api_keys WHERE hash = ?", sum(k)).Scan(&again)
	if again != id || envKeyCount(t, f) != 1 {
		t.Fatalf("restart: id %d then %d, %d env keys", id, again, envKeyCount(t, f))
	}

	// New scopes: the same key, scopes brought in line.
	t.Setenv(EnvAPIKeyScopes, "write, read")
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	who, ok = f.a.checkKey(ctx, k)
	if !ok || who.KeyID != id || !slices.Equal(who.Scopes, []string{"read", "write"}) {
		t.Fatalf("rescoped: %v %+v", ok, who)
	}

	// A new key in the file: the old one stops working.
	k2 := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k2))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, k); ok {
		t.Fatal("the replaced key still works")
	}
	if _, ok := f.a.checkKey(ctx, k2); !ok || envKeyCount(t, f) != 1 {
		t.Fatalf("new key works %v, %d env keys", ok, envKeyCount(t, f))
	}
	var kinds []string
	rows, _ := f.s.ReadEvents().Query("SELECT kind FROM events WHERE kind LIKE 'api_key_%' ORDER BY id")
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		kinds = append(kinds, s)
	}
	rows.Close()
	if strings.Join(kinds, " ") != "api_key_created api_key_changed api_key_deleted api_key_created" {
		t.Fatalf("events %v", kinds)
	}

	// The variable gone: the key stays until someone deletes it.
	t.Setenv(EnvAPIKey, "")
	t.Setenv(EnvAPIKeyScopes, "")
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, k2); !ok {
		t.Fatal("the key went when the variable did")
	}
}

// Keys people made are never taken for the file's key, even with its label.
func TestEnvSetupLeavesPeoplesKeys(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, _ := firstOwnerID(ctx, f.s.Read())
	by, _ := json.Marshal(domain.Actor{Kind: "user", ID: id})
	mine, taken := envKeyValue(), envKeyValue()
	now := database.FormatTime(f.clock.now())
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := insertKey(ctx, tx, now, string(by), id, envKeyLabel, `["all"]`, "", sum(mine), "mine"); err != nil {
			return err
		}
		_, err := insertKey(ctx, tx, now, string(by), id, "made in Settings", `["all"]`, "", sum(taken), "take")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAPIKey, envFile(t, envKeyValue()))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, mine); !ok {
		t.Fatal("a person's key with the same label went")
	}
	// A key made in Settings, copied into the file, is not adopted.
	t.Setenv(EnvAPIKey, envFile(t, taken))
	err := applyEnv(t, f.a)
	if !errors.Is(err, errKeyTaken) || !strings.Contains(err.Error(), keyRecipe) {
		t.Fatalf("taken key: %v", err)
	}
}

// Ten thousand keys made the way HolyArrs makes them all pass, so no sanity
// rule ever refuses a real key.
func TestEnvSetupTakesMadeKeys(t *testing.T) {
	for range 10000 {
		if k := envKeyValue(); envKeyProblem(k) != "" {
			t.Fatalf("%q refused: %s", k, envKeyProblem(k))
		}
	}
}

func TestEnvSetupRefusesWeakKeys(t *testing.T) {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	good := envKeyValue()
	bad := []string{
		token(32),                           // no prefix
		"HAK_" + token(32),                  // wrong prefix
		keyPrefix + token(31),               // 31 bytes
		keyPrefix + token(33),               // 33 bytes
		keyPrefix + hex.EncodeToString(raw), // hex, not the made shape
		keyPrefix + base64.URLEncoding.EncodeToString(raw), // padding
		good[:10] + "+" + good[11:],                        // + is not URL-safe
		good[:len(good)-1] + "B",                           // not a canonical last character
		good[:len(good)-1] + " ",                           // a space
		keyPrefix + strings.Repeat("1", 80),
		// Placeholders.
		"hak_your-api-key-goes-here-please-change-it-now",
		"hak_password1password1password1password1password1",
		"hak_abcdefghabcdefghabcdefghabcdefghabcdefghabcdefgh",
		"hak_" + strings.Repeat("12345678", 10),
		// A 32-byte phrase in base64: the made shape, but no random bytes.
		keyPrefix + base64.RawURLEncoding.EncodeToString([]byte("my-super-secret-holyarrs-api-key")),
		keyPrefix + base64.RawURLEncoding.EncodeToString([]byte("changeme-please-0123456789abcdef")),
		keyPrefix + base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		"",
	}
	for _, k := range bad {
		msg := envKeyProblem(k)
		if msg == "" || !strings.Contains(msg, keyRecipe) {
			t.Fatalf("%q accepted (%q)", k, msg)
		}
	}
	clearEnvSetup(t)
	t.Setenv(EnvAPIKey, envFile(t, keyPrefix+"changeme"))
	if _, err := ReadEnvSetup(); err == nil || !strings.HasPrefix(err.Error(), EnvAPIKey+": ") || strings.Contains(err.Error(), "changeme") {
		t.Fatalf("weak key: %v", err)
	}
}

// Keys with the made shape, enough characters and both cases that only
// repeat a pattern: the repetition rule alone refuses them.
func TestEnvSetupRefusesRepeatingKeys(t *testing.T) {
	for _, pat := range []string{"QwAeRtYuIoPaSdFgHjKl", "EaBcDdFfGgHhIiJjKkLmN", "Zq4Xw8Cv1Bn5Mk7Lp0Jh"} {
		body := strings.Repeat(pat, 3)[:43]
		b, err := base64.RawURLEncoding.Strict().DecodeString(body)
		if err != nil || len(b) != keyBytes || distinct(body) < minKeyChars ||
			!strings.ContainsFunc(body, unicode.IsUpper) || !strings.ContainsFunc(body, unicode.IsLower) {
			t.Fatalf("%q is not refused by repetition alone (%v, %d characters)", body, err, distinct(body))
		}
		if envKeyProblem(keyPrefix+body) == "" {
			t.Fatalf("%q accepted", body)
		}
	}
}

// The recipe the error gives must make a key that is accepted.
func TestEnvSetupKeyRecipeWorks(t *testing.T) {
	for _, tool := range []string{"sh", "openssl", "tr"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool + " here")
		}
	}
	for range 20 {
		out, err := exec.Command("sh", "-c", keyRecipe).Output()
		if err != nil {
			t.Fatal(err)
		}
		k := strings.TrimSuffix(string(out), "\n")
		if msg := envKeyProblem(k); msg != "" || len(k) != len(envKeyValue()) {
			t.Fatalf("recipe made %q: %s", k, msg)
		}
	}
}

func TestEnvSetupScopes(t *testing.T) {
	ok := map[string][]string{"": {"all"}, " , ": {"all"}, "read": {"read"}, "write,read,read": {"read", "write"}, "mcp": {"mcp"}, "control, admin": {"admin", "control"}}
	for in, want := range ok {
		got, err := parseEnvScopes(in)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%q: %v %v", in, got, err)
		}
	}
	bad := map[string]string{"everything": "scopes are", "compat": "make arr API keys", "mcp,read": errMixedMCP, "All": "scopes are"}
	for in, want := range bad {
		_, err := parseEnvScopes(in)
		if err == nil || !strings.HasPrefix(err.Error(), EnvAPIKeyScopes+": ") || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", in, err)
		}
	}
	clearEnvSetup(t)
	t.Setenv(EnvAPIKeyScopes, "read")
	if _, err := ReadEnvSetup(); err == nil || !strings.Contains(err.Error(), EnvAPIKey) {
		t.Fatalf("scopes without a key: %v", err)
	}
	t.Setenv(EnvAPIKey, envFile(t, envKeyValue()))
	t.Setenv(EnvAPIKeyScopes, "compat")
	if _, err := ReadEnvSetup(); err == nil {
		t.Fatal("compat scope accepted")
	}
}

// A key with no owner to hold it stops the start, and leaves the install
// with no owner and no key.
func TestEnvSetupKeyNeedsAnOwner(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	t.Setenv(EnvAPIKey, envFile(t, envKeyValue()))
	err := applyEnv(t, f.a)
	if !errors.Is(err, errNoOwnerForKey) || !strings.Contains(err.Error(), "finish setup first") {
		t.Fatalf("no owner: %v", err)
	}
	var n int
	_ = f.s.Read().QueryRow("SELECT (SELECT count(*) FROM users) + (SELECT count(*) FROM api_keys)").Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows left behind", n)
	}
}

// reset-owner removes every API key; the next start makes the file's key
// again for the reset owner.
func TestEnvSetupKeyAfterResetOwner(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := ResetOwner(ctx, f.s, "lake", "the reset phrase"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, k); ok {
		t.Fatal("the key outlived reset-owner")
	}
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	who, ok := f.a.checkKey(ctx, k)
	name, hash := ownerRow(t, f)
	if !ok || who.Name != "lake" || name != "lake" || !CheckPassword(hash, "the reset phrase") {
		t.Fatalf("after reset: key %v for %q, owner %q", ok, who.Name, name)
	}
}

// A key a person deleted in Settings never comes back on its own: the next
// start stops until the file holds a new key, and the old value stays refused.
func TestEnvSetupKeyDeletedInSettingsStaysDeleted(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	ctx := context.Background()
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	// A file change is not a revoke.
	old := k
	k = envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAPIKey, envFile(t, old))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatalf("a key the file replaced: %v", err)
	}
	// A person's own key, deleted, is not listed either.
	resp, body := call(t, c, "POST", f.srv.URL+"/api/v1/auth/keys", `{"label":"script"}`, map[string]string{CSRFHeader: csrf})
	var mine struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &mine)
	if resp.StatusCode != 201 {
		t.Fatalf("make key: %d %s", resp.StatusCode, body)
	}
	if resp, _ := call(t, c, "DELETE", f.srv.URL+"/api/v1/auth/keys/"+itoa(mine.ID), "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 204 {
		t.Fatalf("delete own key: %d", resp.StatusCode)
	}
	if list, err := revokedKeys(ctx, f.s.Read()); err != nil || len(list) != 0 {
		t.Fatalf("%d revoked hashes before the file's key was deleted (%v)", len(list), err)
	}

	who, ok := f.a.checkKey(ctx, old)
	if !ok {
		t.Fatal("the file's key does not work")
	}
	if resp, _ := call(t, c, "DELETE", f.srv.URL+"/api/v1/auth/keys/"+itoa(who.KeyID), "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	err := applyEnv(t, f.a)
	if !errors.Is(err, errKeyRevoked) || !strings.Contains(err.Error(), "was removed") || strings.Contains(err.Error(), old) {
		t.Fatalf("revoked key: %v", err)
	}
	if _, ok := f.a.checkKey(ctx, old); ok || envKeyCount(t, f) != 0 {
		t.Fatalf("the deleted key came back (%d env keys)", envKeyCount(t, f))
	}
	fresh := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, fresh))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, fresh); !ok {
		t.Fatal("the new key does not work")
	}
	t.Setenv(EnvAPIKey, envFile(t, old))
	if err := applyEnv(t, f.a); !errors.Is(err, errKeyRevoked) {
		t.Fatalf("the revoked key, later: %v", err)
	}
	if _, ok := f.a.checkKey(ctx, fresh); !ok {
		t.Fatal("refusing the revoked key took the new one away")
	}
}

// The owner and the key are one write: a refused key leaves no owner.
func TestEnvSetupIsOneWrite(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	ctx := context.Background()
	k := envKeyValue()
	v, _ := json.Marshal([]string{sum(k)})
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, keyRevokedSetting, string(v), domain.System, f.clock.now())
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); !errors.Is(err, errKeyRevoked) {
		t.Fatalf("revoked key: %v", err)
	}
	if n, _ := countOwners(ctx, f.s.Read()); n != 0 {
		t.Fatal("the owner was made although the key was refused")
	}
}

// A key of the made shape, both cases and no pattern, but drawn from ten
// characters: the character count alone refuses it.
func TestEnvSetupRefusesFewCharacters(t *testing.T) {
	const from = "AaBbCcEeQw"
	b := make([]byte, 43)
	for i := range b {
		b[i] = from[(i*7+i*i/3)%len(from)]
	}
	b[42] = 'A' // a canonical last character
	body := string(b)
	d, err := base64.RawURLEncoding.Strict().DecodeString(body)
	if err != nil || len(d) != keyBytes || repeats(body) || distinct(body) >= minKeyChars {
		t.Fatalf("%q is not refused by the character count alone (%v)", body, err)
	}
	if envKeyProblem(keyPrefix+body) == "" {
		t.Fatalf("%q accepted", body)
	}
}

// A password change removes the owner's keys because someone who knew the old
// password may hold them: the file's key goes too, and the next start does
// not make it again.
func TestEnvSetupKeyGoesWithAPasswordChange(t *testing.T) {
	clearEnvSetup(t)
	f := gated(t, Options{})
	own := ownerOf(t, f)
	ctx := context.Background()
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if resp, body := own.do(t, f, "POST", "/api/v1/auth/password", `{"current":"`+pw+`","new":"another long one"}`); resp.StatusCode != 204 {
		t.Fatalf("change: %d %s", resp.StatusCode, body)
	}
	if _, ok := f.a.checkKey(ctx, k); ok {
		t.Fatal("the file's key outlived the password change")
	}
	err := applyEnv(t, f.a)
	if !errors.Is(err, errKeyRevoked) || !strings.Contains(err.Error(), "password changed") {
		t.Fatalf("restart after the change: %v", err)
	}
	if _, ok := f.a.checkKey(ctx, k); ok || envKeyCount(t, f) != 0 {
		t.Fatalf("the key came back (%d env keys)", envKeyCount(t, f))
	}
	fresh := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, fresh))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.a.checkKey(ctx, fresh); !ok {
		t.Fatal("the new key does not work")
	}
}

// Once the owner exists the password file is never read, so it may be
// removed or emptied after the first start.
func TestEnvSetupPasswordFileOnlyForANewOwner(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	t.Setenv(EnvOwnerName, "river")
	for _, p := range []string{filepath.Join(t.TempDir(), "gone"), envFile(t, "")} {
		t.Setenv(EnvOwnerPassword, p)
		if err := applyEnv(t, f.a); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	// With no owner, a missing file still stops the start.
	clearEnvSetup(t)
	g := setup(t, Options{})
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, filepath.Join(t.TempDir(), "gone"))
	if err := applyEnv(t, g.a); err == nil || !strings.HasPrefix(err.Error(), EnvOwnerPassword+": cannot read") {
		t.Fatalf("missing file with no owner: %v", err)
	}
}

// A list of removed keys that cannot be read stops the start, and deleting a
// key does not write over it.
func TestEnvSetupRevokedListFailsClosed(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	c := client(t)
	csrf := setupOwner(t, f, c)
	ctx := context.Background()
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, keyRevokedSetting, "not a list", domain.System, f.clock.now())
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyEnv(t, f.a); !errors.Is(err, errRevokedUnreadable) || !strings.Contains(err.Error(), keyRevokedSetting) {
		t.Fatalf("corrupt list: %v", err)
	}
	who, _ := f.a.checkKey(ctx, k)
	if resp, _ := call(t, c, "DELETE", f.srv.URL+"/api/v1/auth/keys/"+itoa(who.KeyID), "", map[string]string{CSRFHeader: csrf}); resp.StatusCode != 500 {
		t.Fatalf("delete with a corrupt list: %d", resp.StatusCode)
	}
	if v := database.SettingOr(ctx, f.s.Read(), settingsGroup, keyRevokedSetting, ""); v != "not a list" {
		t.Fatalf("the list was written over: %q", v)
	}
	if _, ok := f.a.checkKey(ctx, k); !ok {
		t.Fatal("the key went although its removal could not be recorded")
	}
}

// The message says what shape is wanted, so a hand-made value can be fixed.
func TestEnvSetupKeyMessageSaysTheShape(t *testing.T) {
	msg := envKeyProblem(keyPrefix + strings.Repeat("x", 43))
	for _, want := range []string{"exactly 43 characters", "A-Z, a-z, 0-9, - and _", "no padding", keyRecipe} {
		if !strings.Contains(msg, want) {
			t.Fatalf("%q lacks %q", msg, want)
		}
	}
}

// A password file saved by a Windows editor starts with a byte order mark;
// it is dropped, as reset-owner drops it.
func TestEnvSetupPasswordDropsAByteOrderMark(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "\ufeffa long enough phrase\r\n"))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if _, hash := ownerRow(t, f); !CheckPassword(hash, "a long enough phrase") {
		t.Fatal("the byte order mark stayed in the password")
	}
}

// The list of removed keys keeps the newest maxRevoked hashes.
func TestEnvSetupRevokedListIsCapped(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	var all []string
	for i := range maxRevoked + 5 {
		h := sum(itoa(int64(i)))
		all = append(all, h)
		if err := f.s.Write(ctx, func(tx *sql.Tx) error {
			return noteRevoked(ctx, tx, h, domain.System, f.clock.now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := revokedKeys(ctx, f.s.Read())
	if err != nil || len(list) != maxRevoked || slices.Contains(list, all[4]) || list[0] != all[5] || list[len(list)-1] != all[len(all)-1] {
		t.Fatalf("%d kept (%v), oldest kept is the first %v, newest kept %v", len(list), err, list[0] == all[0], slices.Contains(list, all[len(all)-1]))
	}
}

// A refusal a person fixes is an *EnvSetupError whose page text leaves the
// file path out; the log line keeps it. A database problem is not one.
func TestEnvSetupRefusalsAreForAPerson(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	p := filepath.Join(t.TempDir(), "owner-password")
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, p)
	err := applyEnv(t, f.a)
	var es *EnvSetupError
	if !errors.As(err, &es) || !strings.Contains(err.Error(), strconv.Quote(p)) || strings.Contains(es.Public, p) || !strings.Contains(es.Public, EnvOwnerPassword) {
		t.Fatalf("missing file: %v / %+v", err, es)
	}
	// Whatever went wrong with the file, the page text never shows its path,
	// even from an os error that names it bare.
	for _, p := range []string{filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		assertNoPath(t, p, EnvOwnerPassword, p)
		assertNoPath(t, p, EnvAPIKey, p)
	}
	raw := refused(&fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}, p)
	if !errors.As(raw, &es) || strings.Contains(es.Public, p) {
		t.Fatalf("bare path: %+v", es)
	}
	clearEnvSetup(t)
	t.Setenv(EnvAPIKey, envFile(t, keyPrefix+"short"))
	if _, err := ReadEnvSetup(); !errors.As(err, &es) {
		t.Fatalf("weak key: %v", err)
	}
	if refused(nil) != nil {
		t.Fatal("refused(nil) is not nil")
	}
}

// reset-owner brings the file's key back, but never one removed before.
func TestEnvSetupResetOwnerKeepsRemovedKeysOut(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	ctx := context.Background()
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, envFile(t, "a long enough phrase"))
	k := envKeyValue()
	t.Setenv(EnvAPIKey, envFile(t, k))
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error { return noteRevoked(ctx, tx, sum(k), domain.System, f.clock.now()) }); err != nil {
		t.Fatal(err)
	}
	if err := ResetOwner(ctx, f.s, "lake", "the reset phrase"); err != nil {
		t.Fatal(err)
	}
	if err := applyEnv(t, f.a); !errors.Is(err, errKeyRevoked) {
		t.Fatalf("removed key after reset-owner: %v", err)
	}
}

// assertNoPath reads path as the variable's file through ReadEnvSetup or the
// owner step and checks the refusal's page text leaves the path out.
func assertNoPath(t *testing.T, path, variable, label string) {
	t.Helper()
	clearEnvSetup(t)
	var err error
	if variable == EnvAPIKey {
		t.Setenv(EnvAPIKey, path)
		_, err = ReadEnvSetup()
	} else {
		f := setup(t, Options{})
		t.Setenv(EnvOwnerName, "river")
		t.Setenv(EnvOwnerPassword, path)
		err = applyEnv(t, f.a)
	}
	var es *EnvSetupError
	if !errors.As(err, &es) || !strings.Contains(err.Error(), strconv.Quote(path)) || strings.Contains(es.Public, path) || !strings.Contains(es.Public, variable) {
		t.Fatalf("%s %s: %v / page %+v", variable, label, err, es)
	}
}

// An owner made from the files still meets the first-run wizard: the files
// only take the place of the account step.
func TestEnvOwnerStillGetsTheWizard(t *testing.T) {
	clearEnvSetup(t)
	f := setup(t, Options{})
	p := filepath.Join(t.TempDir(), "owner-password")
	if err := os.WriteFile(p, []byte("a long enough password 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvOwnerName, "river")
	t.Setenv(EnvOwnerPassword, p)
	if err := applyEnv(t, f.a); err != nil {
		t.Fatal(err)
	}
	if name, _ := ownerRow(t, f); name != "river" {
		t.Fatalf("owner %q", name)
	}
	if FirstRunDone(context.Background(), f.s.Read()) {
		t.Fatal("the wizard counts as finished for an owner made from the files")
	}
}
