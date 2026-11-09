// Command holyarrs-fakesubs serves the lab's fake Subtitle site (made-up
// subtitles for the lab's open movies, and made-up ones for any other
// title). Add it in HolyArrs as a "Lab subtitle site" with its address and
// May reach the local network on.
//
// It listens on this machine only (127.0.0.1) unless -addr names another
// address: the site is for the lab and must not be reachable from the
// network. It holds no secrets and never reaches out.
//
//	holyarrs-fakesubs -addr 127.0.0.1:18727
//
// With -accounts-addr it also serves fakes shaped like the two sites that
// need an account: OpenSubtitles.com under /opensubtitles/api/v1, subdl
// under /subdl/api/v1 and Addic7ed's pages under /addic7ed, all with the
// made-up lab key, name and password below (never a real account; add them
// in HolyArrs with these values).
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18727", "where to listen (this machine only by default)")
	accounts := flag.String("accounts-addr", "", "also serve the account-site fakes here (off when empty)")
	flag.Parse()
	if *accounts != "" {
		go serveAccounts(*accounts)
	}
	srv := &http.Server{Addr: *addr, Handler: fakesite.Lab(), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("lab subtitle site on %s", *addr)
	log.Fatal(srv.ListenAndServe())
}

// The lab's made-up account details for the account-site fakes.
const (
	labKey  = "lab-made-up-key"
	labUser = "lab"
	labPass = "lab-made-up-word"
)

func serveAccounts(addr string) {
	subs := []fakesite.Sub{
		{ID: "9001", Kind: "movie", Language: "en", Title: "Big Buck Bunny", Year: 2008, Release: "Big.Buck.Bunny.2008.1080p.BluRay.x264-LAB", ByID: true, Trusted: true, Text: fakesite.Cue("A big rabbit wakes up.", 1000)},
	}
	mux := http.NewServeMux()
	mux.Handle("/opensubtitles/", http.StripPrefix("/opensubtitles", &fakesite.OpenSubs{Key: labKey, User: labUser, Pass: labPass, Quota: 20, Subs: subs}))
	mux.Handle("/subdl/", http.StripPrefix("/subdl", &fakesite.Subdl{Key: labKey, Quota: 20, Subs: []fakesite.Sub{
		{ID: "9001-1", Kind: "movie", Language: "en", Title: "Big Buck Bunny", Year: 2008, Release: "Big.Buck.Bunny.2008.1080p.BluRay.x264-LAB", Text: fakesite.Cue("A big rabbit wakes up.", 1000)},
	}}))
	mux.Handle("/addic7ed/", http.StripPrefix("/addic7ed", &fakesite.Addic7ed{User: labUser, Pass: labPass, Base: "/addic7ed",
		Shows: map[int]string{1: "Sintel Stories"}, Subs: []fakesite.Sub{
			{ID: "7001", Title: "Sintel Stories", Season: 1, Episode: 1, Language: "English", Release: "WEB.LAB", Text: fakesite.Cue("A girl looks for her dragon.", 1000)},
		}}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("lab account-site fakes on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
