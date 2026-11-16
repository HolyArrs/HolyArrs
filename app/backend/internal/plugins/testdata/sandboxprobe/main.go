// Command sandboxprobe tries one file action and exits 0 only if it worked.
// Usage: sandboxprobe read|write <path>
package main

import (
	"fmt"
	"os"
)

func main() {
	var err error
	switch os.Args[1] {
	case "read":
		_, err = os.ReadFile(os.Args[2])
	case "write":
		err = os.WriteFile(os.Args[2], []byte("x"), 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
