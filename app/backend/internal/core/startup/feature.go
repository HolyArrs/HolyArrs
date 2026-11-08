package startup

import (
	"os"
	"strings"
)

var heldFeatures []string

// RegisterFeature adds a held-back feature to the list. A held-back feature
// is off unless its name is listed in HOLYARRS_FLAGS. Registering a name
// twice keeps one copy.
func RegisterFeature(name string) {
	for _, n := range heldFeatures {
		if n == name {
			return
		}
	}
	heldFeatures = append(heldFeatures, name)
}

// HeldFeatures lists the registered held-back features in the order they
// were registered.
func HeldFeatures() []string {
	return append([]string(nil), heldFeatures...)
}

// FeatureOn reports whether a feature switch is listed in HOLYARRS_FLAGS.
func FeatureOn(name string) bool {
	for _, f := range strings.Split(os.Getenv("HOLYARRS_FLAGS"), ",") {
		if strings.TrimSpace(f) == name {
			return true
		}
	}
	return false
}

// FeaturesOff lists the held-back features that are switched off right now.
// The web app gets the list with the signed-in person's details.
func FeaturesOff() []string {
	out := []string{}
	for _, n := range heldFeatures {
		if !FeatureOn(n) {
			out = append(out, n)
		}
	}
	return out
}
