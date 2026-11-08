package accounts

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

func pngOf(w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func (p person) put(t *testing.T, f *fixture, path string, body []byte, csrf string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest("PUT", f.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	resp, err := p.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

// route: PUT /api/v1/account/avatar
// route: DELETE /api/v1/account/avatar
// route: GET /api/v1/users/{id}/avatar
func TestAvatarRoundTrip(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)

	if resp, _ := own.do(t, f, "GET", "/api/v1/users/1/avatar", ""); resp.StatusCode != 404 {
		t.Fatalf("no picture yet: %d", resp.StatusCode)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"avatar":null`) {
		t.Fatalf("me before: %s", out)
	}

	// A wide picture: the middle square is kept, so the red middle shows.
	wide := image.NewRGBA(image.Rect(0, 0, 600, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 600; x++ {
			if x >= 150 && x < 450 {
				wide.Set(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				wide.Set(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	var src bytes.Buffer
	_ = png.Encode(&src, wide)
	resp, out := own.put(t, f, "/api/v1/account/avatar", src.Bytes(), own.csrf)
	if resp.StatusCode != 200 || !strings.Contains(string(out), `/api/v1/users/1/avatar?v=`) {
		t.Fatalf("put: %d %s", resp.StatusCode, out)
	}

	resp, body := call(t, own.c, "GET", f.srv.URL+"/api/v1/users/1/avatar", "", nil)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cache-Control") == "" || resp.Header.Get("ETag") == "" {
		t.Fatalf("get: %d %v", resp.StatusCode, resp.Header)
	}
	img, err := jpeg.Decode(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("size %v", b)
	}
	if r, _, bl, _ := img.At(2, 128).RGBA(); r>>8 < 200 || bl>>8 > 60 {
		t.Fatalf("the edge is not red: %d %d", r>>8, bl>>8)
	}
	resp2, _ := call(t, own.c, "GET", f.srv.URL+"/api/v1/users/1/avatar", "", map[string]string{"If-None-Match": resp.Header.Get("ETag")})
	if resp2.StatusCode != 304 {
		t.Fatalf("etag: %d", resp2.StatusCode)
	}

	if _, out := own.do(t, f, "GET", "/api/v1/auth/me", ""); !strings.Contains(out, `"avatar":"/api/v1/users/1/avatar?v=`) {
		t.Fatalf("me after: %s", out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/users", ""); !strings.Contains(out, `"avatar":"/api/v1/users/1/avatar?v=`) {
		t.Fatalf("users: %s", out)
	}

	if resp, _ := own.do(t, f, "DELETE", "/api/v1/account/avatar", ""); resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/users/1/avatar", ""); resp.StatusCode != 404 {
		t.Fatalf("after delete: %d", resp.StatusCode)
	}
}

func TestAvatarRules(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	good := pngOf(20, 20, color.RGBA{1, 2, 3, 255})

	if resp, _ := own.put(t, f, "/api/v1/account/avatar", good, ""); resp.StatusCode != 403 {
		t.Fatalf("no CSRF token: %d", resp.StatusCode)
	}
	if resp, _ := own.put(t, f, "/api/v1/account/avatar", []byte("not a picture"), own.csrf); resp.StatusCode != 415 {
		t.Fatalf("not a picture: %d", resp.StatusCode)
	}
	if resp, _ := own.put(t, f, "/api/v1/account/avatar", nil, own.csrf); resp.StatusCode != 415 {
		t.Fatalf("empty: %d", resp.StatusCode)
	}
	if resp, _ := own.put(t, f, "/api/v1/account/avatar", make([]byte, avatarMaxUpload+1), own.csrf); resp.StatusCode != 413 {
		t.Fatalf("too big: %d", resp.StatusCode)
	}
	if resp, _ := own.put(t, f, "/api/v1/account/avatar", good, own.csrf); resp.StatusCode != 200 {
		t.Fatalf("small png: %d", resp.StatusCode)
	}

	// A signed-out visitor sees nothing, and cannot change anything.
	anon := client(t)
	if resp, _ := call(t, anon, "GET", f.srv.URL+"/api/v1/users/1/avatar", "", nil); resp.StatusCode != 401 {
		t.Fatalf("signed out get: %d", resp.StatusCode)
	}
	if resp, _ := call(t, anon, "DELETE", f.srv.URL+"/api/v1/account/avatar", "", nil); resp.StatusCode != 401 {
		t.Fatalf("signed out delete: %d", resp.StatusCode)
	}
}

// A member changes only their own picture.
func TestAvatarIsMine(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	mem := join(t, f, invite(t, f, own, `{"role":"member"}`), "sam")
	img := pngOf(10, 10, color.RGBA{9, 9, 9, 255})
	if resp, _ := mem.put(t, f, "/api/v1/account/avatar", img, mem.csrf); resp.StatusCode != 200 {
		t.Fatalf("member put: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/users/1/avatar", ""); resp.StatusCode != 404 {
		t.Fatalf("the owner's picture was touched: %d", resp.StatusCode)
	}
	if resp, _ := own.do(t, f, "GET", "/api/v1/users/2/avatar", ""); resp.StatusCode != 200 {
		t.Fatalf("owner sees the member's picture: %d", resp.StatusCode)
	}
	if resp, _ := mem.do(t, f, "GET", "/api/v1/users/2/avatar", ""); resp.StatusCode != 200 {
		t.Fatalf("member sees own picture: %d", resp.StatusCode)
	}
}
