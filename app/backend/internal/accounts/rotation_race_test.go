package accounts

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// Requests that arrive together with a session due for rotation must each
// leave the browser with a cookie that still works after the grace minute.
func TestConcurrentRotationKeepsSession(t *testing.T) {
	const parallel = 8
	failures := 0
	for round := 0; round < 20; round++ {
		f := setup(t, Options{})
		c := client(t)
		setupOwner(t, f, c)
		u := mustURL(f.srv.URL)
		old := c.Jar.Cookies(u)[0].Value
		f.clock.add(RotateEvery + time.Minute)

		var wg sync.WaitGroup
		start := make(chan struct{})
		got := make([]string, parallel)
		for i := 0; i < parallel; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				p := client(t)
				p.Jar.SetCookies(u, []*http.Cookie{{Name: CookieName, Value: old}})
				<-start
				resp, _ := call(t, p, "GET", f.srv.URL+"/api/v1/titles", "", nil)
				if resp.StatusCode != 200 {
					t.Errorf("request %d: %d", i, resp.StatusCode)
				}
				for _, ck := range resp.Cookies() {
					if strings.HasPrefix(ck.Name, CookieName) && ck.Value != "" {
						got[i] = ck.Value
					}
				}
			}(i)
		}
		close(start)
		wg.Wait()

		f.clock.add(rotateGrace + time.Second)
		for i, v := range got {
			if v == "" {
				continue // no new cookie: the browser keeps the one it had
			}
			p := client(t)
			p.Jar.SetCookies(u, []*http.Cookie{{Name: CookieName, Value: v}})
			if resp, _ := call(t, p, "GET", f.srv.URL+"/api/v1/titles", "", nil); resp.StatusCode != 200 {
				failures++
				t.Logf("round %d: cookie from request %d dead after the grace minute (%d)", round, i, resp.StatusCode)
			}
		}
	}
	if failures > 0 {
		t.Fatalf("%d rotated cookies expired after the grace minute", failures)
	}
}
