// Command holyarrs-demo looks after the lab copy of HolyArrs while it is
// stopped: inspect counts what is stored, doctor runs the health checks once
// and says what each warning means, seed fills the Library with a realistic
// demo collection through the app's own services, and reset takes out only
// what seed added.
package main

import (
	"context"
	"fmt"
	"os"
)

func usage() {
	fmt.Fprintln(os.Stderr, `usage: holyarrs-demo <command> [flags]
  inspect -config DIR          counts, schema version and states (read only)
  fingerprint -config DIR      one hash per sign-in and settings row, to compare before and after
  query -config DIR "SELECT ..."  one read-only query (sign-in tables refused)
  doctor  -config DIR          run every health check once and explain the results
  seed    -config DIR ...      add the demo library (see -h)
  seed    -config DIR -reset-demo   remove only what seed added
  seed    -config DIR -init-lab     mark the folder as the lab's (once, by hand)
  showcase [-runtime DIR] [-reset]  fill the stopped redesign showcase (18728) with every Library state
  showcase -runtime DIR -init       mark a showcase runtime folder (once, by hand)`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "inspect":
		c, perr := parseCommon("inspect", os.Args[2:])
		if perr != nil {
			os.Exit(2)
		}
		err = inspect(ctx, c.ConfigDir, os.Stdout)
	case "query":
		// query -config DIR "SELECT ..." prints rows of a read-only query.
		if len(os.Args) < 5 {
			usage()
			os.Exit(2)
		}
		err = query(ctx, os.Args[3], os.Args[4], os.Stdout)
	case "fingerprint":
		c, perr := parseCommon("fingerprint", os.Args[2:])
		if perr != nil {
			os.Exit(2)
		}
		err = fingerprint(ctx, c.ConfigDir, os.Stdout)
	case "doctor":
		c, perr := parseCommon("doctor", os.Args[2:])
		if perr != nil {
			os.Exit(2)
		}
		err = doctor(ctx, c, os.Stdout)
	case "showcase":
		so, perr := parseShowcase(os.Args[2:])
		if perr != nil {
			fmt.Fprintln(os.Stderr, perr)
			os.Exit(2)
		}
		err = showcaseCommand(ctx, so, os.Stdout)
	case "seed":
		c, perr := parseCommon("seed", os.Args[2:])
		if perr != nil {
			os.Exit(2)
		}
		err = seedCommand(ctx, c, os.Stdout)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "holyarrs-demo:", err)
		os.Exit(1)
	}
}
