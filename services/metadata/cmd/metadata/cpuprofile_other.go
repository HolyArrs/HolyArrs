//go:build !unix

package main

import "os"

func profileSignals() []os.Signal { return nil }
