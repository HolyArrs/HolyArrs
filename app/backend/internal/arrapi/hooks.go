package arrapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Webhook records. A request app such as Pulsarr asks a side to keep one,
// so HolyArrs tells it when a file was imported. Each side keeps its own
// records. The address, the extra headers and the sign-in are sealed in
// the vault; the rest of the record is kept as sent and given back as it
// came, with header values and the password masked. A record is saved
// only after a test call to it worked, as the old apps do.

// hookTestTimeout bounds one test call to a hook, inside Pulsarr's own
// ten-second wait for the whole check.
var hookTestTimeout = 8 * time.Second

// maxHooks caps the records one side keeps, so one import never fans out
// to an unbounded list.
const maxHooks = 25

// hookSecret is the sealed part of a hook.
type hookSecret struct {
	URL      string       `json:"url"`
	Headers  []hookHeader `json:"headers"`
	Username string       `json:"username,omitempty"`
	Password string       `json:"password,omitempty"`
}

type hookHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// hookIn is a hook body as read. maskedHeaders and maskedPassword mark the
// values that came back masked, which take the stored value.
type hookIn struct {
	id             int64
	hk             hook
	sec            hookSecret
	maskedHeaders  []bool
	maskedPassword bool
}

// framingHeaders are header names a record may not set: they change how
// the call is sent, or which server it reaches.
var framingHeaders = map[string]bool{"host": true, "content-length": true, "content-type": true, "transfer-encoding": true, "connection": true,
	"upgrade": true, "te": true, "trailer": true, "keep-alive": true, "expect": true, "proxy-connection": true, "proxy-authorization": true,
	strings.ToLower(outbound.SelfCallHeader): true}

// checkHookURL accepts an http or https address with a host and no
// sign-in in it. Where it may connect is checked on every call, after DNS.
func checkHookURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 2048 && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Hostname() != "" && u.User == nil
}

// tokenChar reports whether c may stand in a header name.
func tokenChar(c byte) bool {
	return c > ' ' && c < 0x7f && !strings.ContainsRune(`()<>@,;:\"/[]?={}`, rune(c))
}

func validHeader(h hookHeader) bool {
	if h.Key == "" || len(h.Key) > 100 || len(h.Value) > 4096 || framingHeaders[strings.ToLower(h.Key)] {
		return false
	}
	for i := 0; i < len(h.Key); i++ {
		if !tokenChar(h.Key[i]) {
			return false
		}
	}
	for i := 0; i < len(h.Value); i++ {
		if c := h.Value[i]; (c < ' ' && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

func isMasked(s string) bool { return s == maskedKey || s == secrets.Masked }

// readHeaders reads the headers setting: a list of key and value pairs, or
// one object of names and values. It returns false for anything else.
func readHeaders(v any) ([]hookHeader, bool) {
	var out []hookHeader
	switch x := v.(type) {
	case nil:
	case []any:
		for _, e := range x {
			m, ok := e.(map[string]any)
			if !ok {
				return nil, false
			}
			k := valueString(m, "key")
			if k == "" {
				k = valueString(m, "name")
			}
			out = append(out, hookHeader{Key: strings.TrimSpace(k), Value: valueString(m, "value")})
		}
	case map[string]any:
		for k, e := range x {
			s, _ := e.(string)
			out = append(out, hookHeader{Key: strings.TrimSpace(k), Value: s})
		}
	default:
		return nil, false
	}
	return out, len(out) <= 20
}

// readHook reads a hook body. It returns false after sending a 400.
func (h *handler) readHook(w http.ResponseWriter, r *http.Request) (hookIn, bool) {
	var in hookIn
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyMax))
	var m map[string]any
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil || m == nil {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return in, false
	}
	if kind := valueString(m, "implementation"); !strings.EqualFold(kind, "webhook") {
		refuse(w, failure{PropertyName: "Implementation", ErrorMessage: "HolyArrs keeps webhook records only; it tells Plex, Jellyfin and Emby about new files itself, under Media servers", AttemptedValue: kind})
		return in, false
	}
	in.hk = hook{Side: h.side, Name: strings.TrimSpace(valueString(m, "name")), Method: http.MethodPost}
	if in.hk.Name == "" || len(in.hk.Name) > 200 {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "give the record a name of 1 to 200 characters", AttemptedValue: in.hk.Name})
		return in, false
	}
	raw, _ := fieldValue(m, "url")
	in.sec.URL, _ = raw.(string)
	in.sec.URL = strings.TrimSpace(in.sec.URL)
	if !checkHookURL(in.sec.URL) {
		refuse(w, failure{PropertyName: "Url", ErrorMessage: "give an address that starts with http:// or https:// and names a host; a sign-in goes in the username and password settings"})
		return in, false
	}
	if reachedOn(r, in.sec.URL) {
		refuse(w, failure{PropertyName: "Url", ErrorMessage: "this address points back at HolyArrs itself; give the address of the app that should get the calls", AttemptedValue: in.sec.URL})
		return in, false
	}
	// The old apps number the methods: 1 is POST, 2 is PUT.
	if v, _ := fieldValue(m, "method"); v != nil {
		s, _ := v.(string)
		if n, ok := toInt(v); (ok && n == 2) || strings.EqualFold(strings.TrimSpace(s), "put") {
			in.hk.Method = http.MethodPut
		}
	}
	hv, _ := fieldValue(m, "headers")
	headers, ok := readHeaders(hv)
	if !ok {
		refuse(w, failure{PropertyName: "Headers", ErrorMessage: "headers are a list of at most 20 key and value pairs"})
		return in, false
	}
	for _, hd := range headers {
		masked := isMasked(hd.Value)
		if !validHeader(hd) {
			refuse(w, failure{PropertyName: "Headers", ErrorMessage: "a header needs a plain name that does not change how the call is sent, and a value on one line", AttemptedValue: hd.Key})
			return in, false
		}
		in.sec.Headers = append(in.sec.Headers, hd)
		in.maskedHeaders = append(in.maskedHeaders, masked)
	}
	user, _ := fieldValue(m, "username")
	pass, _ := fieldValue(m, "password")
	in.sec.Username, _ = user.(string)
	in.sec.Password, _ = pass.(string)
	in.maskedPassword = isMasked(in.sec.Password)
	on := func(k string) bool { v, _ := valueOf(m, k); return toBool(v, false) }
	in.hk.OnImport, in.hk.OnUpgrade = on("onDownload"), on("onUpgrade")
	if v, ok := valueOf(m, "tags"); ok {
		l, _ := v.([]any)
		for _, x := range l {
			if n, ok := toInt(x); ok && n > 0 {
				in.hk.Tags = append(in.hk.Tags, int64(n))
			}
		}
	}
	// Keep the record as sent, without its id and the sealed values.
	for k, v := range m {
		if strings.EqualFold(k, "id") {
			n, _ := toInt(v)
			in.id = int64(n)
			delete(m, k)
		}
	}
	for _, f := range fieldRows(m) {
		switch strings.ToLower(fieldName(f)) {
		case "url", "username", "password":
			f["value"] = ""
		case "headers":
			f["value"] = []any{}
		}
	}
	kept, _ := json.Marshal(m)
	if len(kept) > bodyKeep {
		message(w, http.StatusBadRequest, "the record is too big to keep")
		return in, false
	}
	in.hk.Body = string(kept)
	return in, true
}

// fillMasked puts the stored values in place of masked ones. A stored
// value only goes to the server it was typed for. It returns false after
// sending a 400.
func fillMasked(w http.ResponseWriter, in *hookIn, stored *hookSecret) bool {
	masked := in.maskedPassword
	for _, m := range in.maskedHeaders {
		masked = masked || m
	}
	if !masked {
		return true
	}
	if stored == nil || !strings.EqualFold(hostOnly(stored.URL), hostOnly(in.sec.URL)) {
		refuse(w, failure{PropertyName: "Headers", ErrorMessage: "type the header values and password again: HolyArrs sends a stored one only to the server it was typed for"})
		return false
	}
	for i, m := range in.maskedHeaders {
		if !m {
			continue
		}
		found := false
		for _, old := range stored.Headers {
			if strings.EqualFold(old.Key, in.sec.Headers[i].Key) {
				in.sec.Headers[i].Value, found = old.Value, true
				break
			}
		}
		if !found {
			refuse(w, failure{PropertyName: "Headers", ErrorMessage: "type this header's value: HolyArrs has none stored for it", AttemptedValue: in.sec.Headers[i].Key})
			return false
		}
	}
	if in.maskedPassword {
		in.sec.Password = stored.Password
	}
	return true
}

// mayWriteHooks reports whether the caller may write or test hooks. They
// make HolyArrs call out to an address, so the key must act as someone who
// may change settings. It returns false after sending the answer.
func mayWriteHooks(w http.ResponseWriter, r *http.Request, v *secrets.Vault) bool {
	if who, ok := webserver.From(r.Context()); !ok || !who.Has(webserver.PermSettings) {
		message(w, http.StatusForbidden, "the key acts as the person who made it, who may not change settings")
		return false
	}
	if v == nil {
		message(w, http.StatusServiceUnavailable, "HolyArrs cannot keep a record's secrets right now")
		return false
	}
	return true
}

// hookSecretOf reads a hook's sealed part.
func hookSecretOf(ctx context.Context, v *secrets.Vault, hk hook) (hookSecret, error) {
	var sec hookSecret
	if hk.SecretID == nil || v == nil {
		return sec, errors.New("hook has no sealed part")
	}
	plain, err := v.Get(ctx, *hk.SecretID)
	if err != nil {
		return sec, err
	}
	return sec, json.Unmarshal([]byte(plain), &sec)
}

// hookOf builds a record as the list gives it back: as sent, with the
// address in full and header values and the password masked.
func hookOf(hk hook, sec hookSecret) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal([]byte(hk.Body), &m)
	m["id"], m["name"], m["tags"] = hk.ID, hk.Name, hk.Tags
	if hk.Tags == nil {
		m["tags"] = []int64{}
	}
	for _, f := range fieldRows(m) {
		switch strings.ToLower(fieldName(f)) {
		case "url":
			f["value"] = sec.URL
		case "username":
			f["value"] = sec.Username
		case "password":
			f["value"] = secrets.Mask(sec.Password)
		case "headers":
			hs := []map[string]string{}
			for _, hd := range sec.Headers {
				hs = append(hs, map[string]string{"key": hd.Key, "value": maskedKey})
			}
			f["value"] = hs
		}
	}
	return m
}

// testHookCall sends the test event to a hook, once, and answers false
// after refusing on the address field when it did not work.
func (h *handler) testHookCall(w http.ResponseWriter, r *http.Request, in hookIn) bool {
	ctx, cancel := context.WithTimeout(r.Context(), hookTestTimeout)
	defer cancel()
	if err := callHook(ctx, in.hk.Method, in.sec, testEvent()); err != nil {
		refuse(w, failure{PropertyName: "Url", ErrorMessage: "HolyArrs could not reach this address: " + err.Error()})
		return false
	}
	return true
}

// listHooks answers this side's records.
func (h *handler) listHooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hs, err := listHookRows(ctx, h.Store.Read(), h.side)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(hs))
	for _, hk := range hs {
		sec, err := hookSecretOf(ctx, h.Secrets, hk)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, hookOf(hk, sec))
	}
	writeJSON(w, http.StatusOK, out)
}

// saveHookFrom checks the name, tests the hook and stores it. stored is
// the record it replaces, nil for a new one.
func (h *handler) saveHookFrom(w http.ResponseWriter, r *http.Request, in hookIn, stored *hook) {
	ctx := r.Context()
	var old *hookSecret
	if stored != nil {
		sec, err := hookSecretOf(ctx, h.Secrets, *stored)
		if err != nil {
			internalError(w, err)
			return
		}
		old = &sec
		in.hk.ID = stored.ID
	}
	if !fillMasked(w, &in, old) {
		return
	}
	if taken, err := hookNameTaken(ctx, h.Store.Read(), h.side, in.hk.Name, in.hk.ID); err != nil {
		internalError(w, err)
		return
	} else if taken {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "another record on this side has that name", AttemptedValue: in.hk.Name})
		return
	}
	if stored == nil {
		if n, err := countHookRows(ctx, h.Store.Read(), h.side); err != nil {
			internalError(w, err)
			return
		} else if n >= maxHooks {
			refuse(w, failure{PropertyName: "Name", ErrorMessage: "this side keeps at most 25 records; remove one first"})
			return
		}
	}
	if !h.testHookCall(w, r, in) {
		return
	}
	sealed, _ := json.Marshal(in.sec)
	id, err := saveHook(ctx, h.Store, h.Secrets, in.hk, string(sealed), actor(r))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		// Another call saved the same name since the check above.
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "another record on this side has that name", AttemptedValue: in.hk.Name})
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	hk, err := hookRow(ctx, h.Store.Read(), h.side, id)
	if err != nil {
		internalError(w, err)
		return
	}
	code := http.StatusCreated
	if stored != nil {
		code = http.StatusAccepted
	}
	writeJSON(w, code, hookOf(hk, in.sec))
}

// addHook keeps a new record after a test call to it worked.
func (h *handler) addHook(w http.ResponseWriter, r *http.Request) {
	if !mayWriteHooks(w, r, h.Secrets) {
		return
	}
	in, ok := h.readHook(w, r)
	if !ok {
		return
	}
	h.saveHookFrom(w, r, in, nil)
}

// putHook replaces a record in place, after the same test, so a refused
// change never drops the working one. The id comes from the path or the
// body.
func (h *handler) putHook(w http.ResponseWriter, r *http.Request) {
	if !mayWriteHooks(w, r, h.Secrets) {
		return
	}
	in, ok := h.readHook(w, r)
	if !ok {
		return
	}
	if v := r.PathValue("id"); v != "" {
		in.id = atoi64(v)
	}
	stored, err := hookRow(r.Context(), h.Store.Read(), h.side, in.id)
	if err != nil {
		internalError(w, err)
		return
	}
	h.saveHookFrom(w, r, in, &stored)
}

// deleteHook removes a record. A gone id is 404.
func (h *handler) deleteHook(w http.ResponseWriter, r *http.Request) {
	if !mayWriteHooks(w, r, h.Secrets) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	found, err := deleteHookRow(r.Context(), h.Store, h.side, id)
	switch {
	case err != nil:
		internalError(w, err)
	case !found:
		notFound(w)
	default:
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// testHook sends the test event to a record that is not saved. With the id
// of a stored record, masked values take that record's stored ones.
func (h *handler) testHook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !mayWriteHooks(w, r, h.Secrets) {
		return
	}
	in, ok := h.readHook(w, r)
	if !ok {
		return
	}
	var old *hookSecret
	if in.id > 0 {
		stored, err := hookRow(ctx, h.Store.Read(), h.side, in.id)
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			internalError(w, err)
			return
		}
		if err == nil {
			sec, err := hookSecretOf(ctx, h.Secrets, stored)
			if err != nil {
				internalError(w, err)
				return
			}
			old = &sec
		}
	}
	if !fillMasked(w, &in, old) || !h.testHookCall(w, r, in) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
