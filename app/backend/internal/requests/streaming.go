package requests

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// maxMyServices caps the owner's streaming services.
const maxMyServices = 30

// ErrStreamingRegion and ErrStreamingServices answer odd My streaming
// services settings.
var (
	ErrStreamingRegion   = errors.New("pick the country your streaming services are in")
	ErrStreamingServices = errors.New("pick up to 30 streaming services")
	ErrNoMyServices      = errors.New("pick at least one streaming service under My streaming services, then save")
)

// watchOf reads where one title streams, per country.
func (s *Service) watchOf(ctx context.Context, kind, tmdbID string) (map[string][]metadata.WatchService, error) {
	if s.Watch != nil {
		return s.Watch(ctx, kind, tmdbID)
	}
	l, ok := metadata.WatchListerFor(ctx, s.Metadata)
	if !ok {
		return nil, metadata.ErrNoAccess
	}
	return l.WatchProviders(ctx, kind, tmdbID)
}

// OnMyServices finds the first of services (in the owner's order) that
// streams the title in region, from one watch providers answer.
func OnMyServices(got map[string][]metadata.WatchService, region string, services []int64) (metadata.WatchService, bool) {
	list := got[strings.ToUpper(region)]
	for _, want := range services {
		for _, sv := range list {
			if sv.ID == want {
				return sv, true
			}
		}
	}
	return metadata.WatchService{}, false
}

// streamingOn says which of the owner's services a movie or show ask's
// title is on, when the skip setting is on; empty when none, or when the
// answer could not be read (the download then goes ahead).
func (s *Service) streamingOn(ctx context.Context, a domain.Ask, set Settings) string {
	if !set.SkipOnServices || set.StreamingRegion == "" || len(set.StreamingServices) == 0 || (a.Kind != "movie" && a.Kind != "show") || a.TMDBID == "" {
		return ""
	}
	got, err := s.watchOf(ctx, a.Kind, a.TMDBID)
	if err != nil {
		log.Printf("asks: streaming services of ask %d: %v", a.ID, err)
		return ""
	}
	sv, ok := OnMyServices(got, set.StreamingRegion, set.StreamingServices)
	if !ok {
		return ""
	}
	return sv.Name
}

// skipNote is the note on an ask turned down because it streams on one
// of the owner's services.
func skipNote(service string) string {
	return "It's on " + service + ", one of the streaming services set in Requests settings, so it was not downloaded. Approve it again to download it anyway."
}

// SkippedOnServices is said to the approver when the ask was turned down
// because it streams on one of the owner's services.
func SkippedOnServices(service string) string {
	return "Not downloaded: it's on " + service + ". Approve it again to download it anyway."
}

// parseServices reads service ids joined by |, keeping order, dropping
// odd ones and repeats.
func parseServices(v string) []int64 {
	out := []int64{}
	seen := map[int64]bool{}
	for _, p := range strings.Split(v, "|") {
		n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || n <= 0 || seen[n] || len(out) == maxMyServices {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// storeServices joins service ids with |.
func storeServices(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, "|")
}

// checkStreaming checks and tidies the My streaming services settings.
func checkStreaming(s *Settings) error {
	s.StreamingRegion = strings.ToUpper(strings.TrimSpace(s.StreamingRegion))
	if r := s.StreamingRegion; r != "" && (len(r) != 2 || r[0] < 'A' || r[0] > 'Z' || r[1] < 'A' || r[1] > 'Z') {
		return ErrStreamingRegion
	}
	if len(s.StreamingServices) > maxMyServices {
		return ErrStreamingServices
	}
	for _, id := range s.StreamingServices {
		if id <= 0 {
			return ErrStreamingServices
		}
	}
	s.StreamingServices = parseServices(storeServices(s.StreamingServices))
	return nil
}
