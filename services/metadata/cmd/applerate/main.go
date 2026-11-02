// Command applerate measures how fast Apple Music's catalog answers from
// this address: stepped call rates, counting answers by status. It prints
// only counts, never the token or key.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

func main() {
	steps := flag.String("steps", "5,10,20,40", "calls a second per step")
	each := flag.Duration("each", 30*time.Second, "time per step")
	flag.Parse()
	am, err := imgstore.LoadAppleMusic(os.Getenv, os.ReadFile)
	if err != nil {
		log.Fatal("apple key: not loaded")
	}
	tok, err := am.Token()
	if err != nil {
		log.Fatal("apple token: failed")
	}
	terms := []string{"radiohead", "beatles", "adele", "drake", "taylor swift", "bad bunny", "coldplay", "rihanna", "eminem", "metallica", "kendrick lamar", "dua lipa", "the weeknd", "billie eilish", "queen", "abba", "nirvana", "madonna", "u2", "bjork"}
	cl := &http.Client{Timeout: 15 * time.Second}
	var rates []int
	for _, s := range splitInts(*steps) {
		rates = append(rates, s)
	}
	var n int64
	for _, rate := range rates {
		var mu sync.Mutex
		codes := map[int]int{}
		var lat time.Duration
		var wg sync.WaitGroup
		tick := time.NewTicker(time.Second / time.Duration(rate))
		end := time.Now().Add(*each)
		t0 := time.Now()
		for time.Now().Before(end) {
			<-tick.C
			i := atomic.AddInt64(&n, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				q := url.Values{"term": {terms[i%int64(len(terms))]}, "types": {"artists"}, "limit": {"1"}, "offset": {fmt.Sprint(i % 3)}}
				req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://api.music.apple.com/v1/catalog/us/search?"+q.Encode(), nil)
				req.Header.Set("Authorization", "Bearer "+tok)
				st := time.Now()
				code := 0
				if resp, err := cl.Do(req); err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					code = resp.StatusCode
				}
				mu.Lock()
				codes[code]++
				lat += time.Since(st)
				mu.Unlock()
			}()
		}
		tick.Stop()
		wg.Wait()
		tot := 0
		for _, c := range codes {
			tot += c
		}
		fmt.Printf("target %d/s: %d calls in %.1f s = %.1f/s, codes %v, mean latency %d ms\n", rate, tot, time.Since(t0).Seconds(), float64(tot)/time.Since(t0).Seconds(), codes, (lat / time.Duration(max(tot, 1))).Milliseconds())
	}
}

func splitInts(s string) []int {
	var out []int
	cur := 0
	for _, r := range s + "," {
		if r == ',' {
			out = append(out, cur)
			cur = 0
			continue
		}
		cur = cur*10 + int(r-'0')
	}
	return out
}
