// Command musicwipe clears the music data on the metadata server: music
// tables, music pictures (kinds artist, album, wide, logo only), Deezer
// matches, chart and new-release caches. Movie/TV data is never touched.
//
//	musicwipe -data /data            dry run: prints what would go
//	musicwipe -data /data -apply     really deletes (stop the server first)
//
// The default is a dry run, which opens every file read-only.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	data := flag.String("data", "", "metadata data directory (holds music.sqlite and img/)")
	doApply := flag.Bool("apply", false, "really delete (default: dry run)")
	sample := flag.Int("sample", 5, "ids to show per table")
	flag.StringVar(&picturesDir, "pictures", "", "dry run only: count picture files here instead of <data>/img")
	flag.Parse()
	if picturesDir != "" && *doApply {
		fmt.Fprintln(os.Stderr, "musicwipe: -pictures is for dry runs only")
		os.Exit(2)
	}
	if *data == "" {
		fmt.Fprintln(os.Stderr, "musicwipe: -data is required")
		os.Exit(2)
	}
	p, err := plan(*data, *sample)
	if err != nil {
		fmt.Fprintln(os.Stderr, "musicwipe:", err)
		os.Exit(1)
	}
	p.print(os.Stdout, false)
	kp, err := keptPictures(*data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "musicwipe:", err)
		os.Exit(1)
	}
	fmt.Println("\nkept pictures by kind (rows, bytes, misses):")
	ks := make([]string, 0, len(kp))
	for k := range kp {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		fmt.Printf("  %-14s %10d %14d %10d\n", k, kp[k][0], kp[k][1], kp[k][2])
	}
	if !*doApply {
		after, err := kept(*data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "musicwipe:", err)
			os.Exit(1)
		}
		ok, diff := p.Kept.equal(after)
		fmt.Printf("\nDRY RUN: nothing was changed. kept rows before = after: %v %v\n", ok, diff)
		return
	}
	if err := apply(*data); err != nil {
		fmt.Fprintln(os.Stderr, "musicwipe:", err)
		os.Exit(1)
	}
	fmt.Println("\napplied. kept rows unchanged.")
}
