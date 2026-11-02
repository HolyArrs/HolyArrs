package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	upnet "github.com/CoderLuii/holyarrs-metadata/internal/upstream"
)

// Security M3: the committed egress example parses and names no address;
// the real egress.env is not in the source tree.
func TestEgressExampleHasNoAddresses(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "deploy", "metadata", "live")
	f, err := os.Open(filepath.Join(dir, "egress.env.example"))
	if err != nil {
		t.Skip("deploy folder not beside the source (build copy)")
	}
	defer f.Close()
	dotted := regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	sc := bufio.NewScanner(f)
	val := ""
	for sc.Scan() {
		line := sc.Text()
		if dotted.MatchString(line) {
			t.Fatalf("address in the example: %q", line)
		}
		if v, ok := strings.CutPrefix(line, upnet.EgressEnv+"="); ok {
			val = v
		}
	}
	if _, err := upnet.ParseEgress(val); err != nil || val == "" {
		t.Fatalf("example does not parse: %v", err)
	}
}
