package main

import (
	"io"
	"testing"
)

func TestParseFlagsDefaults(t *testing.T) {
	c, err := parseFlags(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.ConfigDir != "/config" || c.Addr != ":18730" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestParseFlagsSet(t *testing.T) {
	c, err := parseFlags([]string{"-config", "/data", "-addr", ":9000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.ConfigDir != "/data" || c.Addr != ":9000" {
		t.Fatalf("flags not applied: %+v", c)
	}
}

func TestParseFlagsRejectsExtra(t *testing.T) {
	if _, err := parseFlags([]string{"stray"}, io.Discard); err == nil {
		t.Fatal("expected an error for stray arguments")
	}
}
