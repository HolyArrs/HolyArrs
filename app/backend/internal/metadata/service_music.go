package metadata

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	serviceMusicSearchLimit = 25
	serviceReleaseLimit     = 100
	serviceTrackLimit       = 10000
)

const (
	detachedArtistTimeout = 45 * time.Second
	detachedArtistLimit   = 4
)

type serviceMusic struct {
	service *Service
	direct  ArtistProvider

	mu       sync.Mutex
	inflight map[string]*artistFlight
	slots    chan struct{}
}

// artistFlight is one detached full artist lookup that every request for the
// same MBID shares.
type artistFlight struct {
	done chan struct{}
	a    Artist
	err  error
}

// detachedArtist joins the in-flight full lookup for mbid or starts one. It
// returns nil when none runs and all detached slots are taken.
func (m *serviceMusic) detachedArtist(ctx context.Context, mbid string) *artistFlight {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.inflight[mbid]; ok {
		return f
	}
	if m.slots == nil {
		m.slots = make(chan struct{}, detachedArtistLimit)
	}
	select {
	case m.slots <- struct{}{}:
	default:
		return nil
	}
	if m.inflight == nil {
		m.inflight = map[string]*artistFlight{}
	}
	f := &artistFlight{done: make(chan struct{})}
	m.inflight[mbid] = f
	bg, stop := context.WithTimeout(context.WithoutCancel(ctx), detachedArtistTimeout)
	go func() {
		defer stop()
		f.a, f.err = m.direct.Artist(bg, mbid)
		m.mu.Lock()
		delete(m.inflight, mbid)
		m.mu.Unlock()
		<-m.slots
		close(f.done)
	}()
	return f
}

// Artists uses the optional standalone service while retaining the direct
// provider for complete catalog reads and eligible fallbacks.
func (s *Service) Artists(direct ArtistProvider) ArtistProvider {
	return &serviceMusic{service: s, direct: direct}
}

type serviceArtistData struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SortName       string `json:"sort-name"`
	Type           string `json:"type"`
	Disambiguation string `json:"disambiguation"`
	Country        string `json:"country"`
	Score          int    `json:"score"`
	LifeSpan       struct {
		Ended bool `json:"ended"`
	} `json:"life-span"`
	ReleaseGroups *[]serviceAlbumData `json:"release-groups"`
	Popularity    *ArtistPopularity   `json:"popularity"`
}

type serviceAlbumData struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	PrimaryType    string   `json:"primary-type"`
	SecondaryTypes []string `json:"secondary-types"`
	FirstRelease   string   `json:"first-release-date"`
	Disambiguation string   `json:"disambiguation"`
}

func (m *serviceMusic) SearchArtists(ctx context.Context, query string) ([]Artist, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Artist{}, nil
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	directQuery := query
	serviceQuery := luceneSpecial.ReplaceAllString(query, `\$1`)
	result, err := m.service.search(ctx, "musicbrainz", "artist", serviceQuery, 1, serviceMusicSearchLimit)
	if err != nil {
		if m.direct != nil && serviceFallback(ctx, err) {
			return m.direct.SearchArtists(ctx, directQuery)
		}
		return nil, serviceMusicError(err)
	}
	out := make([]Artist, 0, len(result.Items))
	for _, item := range result.Items {
		artist, err := parseServiceArtist(item, false)
		if err != nil {
			return nil, err
		}
		out = append(out, artist)
	}
	return out, nil
}

func (m *serviceMusic) Artist(ctx context.Context, mbid string) (Artist, error) {
	if !ValidMBID(mbid) {
		return Artist{}, ErrArtistNotFound
	}
	mbid = strings.ToLower(mbid)
	// A package kept on disk answers at once and in full; an artist no
	// package covers is asked of the server below and wanted there.
	if l := m.service.localMusic(); l != nil {
		// The Library (not a page) wants the whole list: a partial package
		// is left to the server's walk there.
		// A page that needs an album the partial list lacks asks for the
		// whole list (WholeArtist).
		if a, ok := l.LocalArtist(ctx, mbid); ok && (!a.Partial || IsMemberLookup(ctx) && ctx.Value(wholeArtistKey{}) == nil) {
			return a, nil
		} else if !ok {
			l.Uncovered(mbid)
		}
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	item, err := m.service.detail(ctx, "musicbrainz", "artist", mbid)
	if err != nil && !(errors.Is(err, ErrServicePending) && item.Source != "") {
		if m.direct != nil && serviceFallback(ctx, err) {
			return m.directArtist(ctx, mbid)
		}
		return Artist{}, serviceMusicError(err)
	}
	// The server's figures are kept whichever album list answers: a long
	// discography falls back to MusicBrainz's own, which has none.
	pop := servicePopularity(item)
	artist, err := parseServiceArtist(item, true)
	if err != nil {
		if m.direct != nil && serviceFallback(ctx, err) {
			a, err := m.directArtist(ctx, mbid)
			if err == nil && a.Popularity == nil {
				a.Popularity = pop
			}
			return a, err
		}
		return Artist{}, err
	}
	// A complete stored list is the whole discography: no MusicBrainz walk.
	artist.Partial = !item.ReleaseGroupsComplete
	if m.direct == nil || !artist.Partial {
		return artist, nil
	}
	if wait, ok := ctx.Value(quickArtistKey{}).(time.Duration); ok {
		// A page view: MusicBrainz pages through release groups at one
		// request a second, so wait only a while. The full lookup goes on
		// and lands in the lookup cache for the next view.
		f := m.detachedArtist(ctx, mbid)
		if f == nil {
			return artist, nil
		}
		done := f.done
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-done:
			if f.err == nil && !f.a.Partial && validDirectArtist(f.a, mbid) {
				a := f.a
				a.Popularity = pop
				return a, nil
			}
		case <-t.C:
		case <-ctx.Done():
			return Artist{}, ctx.Err()
		}
		return artist, nil
	}
	direct, directErr := m.direct.Artist(ctx, mbid)
	if ctx.Err() != nil {
		return Artist{}, ctx.Err()
	}
	if directErr == nil && !direct.Partial && validDirectArtist(direct, mbid) {
		direct.Popularity = pop
		return direct, nil
	}
	return artist, nil
}

// servicePopularity reads an artist answer's figures, inside its data or
// beside it; nil when there are none.
func servicePopularity(item serviceItem) *ArtistPopularity {
	var data struct {
		Popularity *ArtistPopularity `json:"popularity"`
	}
	if item.decode(&data) == nil && data.Popularity.orNil() != nil {
		return data.Popularity.orNil()
	}
	return item.Popularity.orNil()
}

type quickArtistKey struct{}

type wholeArtistKey struct{}

type tracklistOnlyKey struct{}

// TracklistOnly marks a page view's ctx so Pressings asks the metadata
// server's one-call album answer only, never the per-edition fan-out: a
// warm-up ask that makes the server fetch the tracklist ahead of a click.
func TracklistOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, tracklistOnlyKey{}, true)
}

// IsTracklistOnly says ctx was marked by TracklistOnly.
func IsTracklistOnly(ctx context.Context) bool { return ctx.Value(tracklistOnlyKey{}) != nil }

// WholeArtist marks ctx so Artist never answers with a partial package
// kept on disk (an album page whose album the package does not list).
func WholeArtist(ctx context.Context) context.Context {
	return context.WithValue(ctx, wholeArtistKey{}, true)
}

// QuickArtist marks ctx so Artist waits at most wait for the full
// MusicBrainz album list before it answers with the metadata server's.
func QuickArtist(ctx context.Context, wait time.Duration) context.Context {
	return context.WithValue(ctx, quickArtistKey{}, wait)
}

func (m *serviceMusic) directArtist(ctx context.Context, mbid string) (Artist, error) {
	artist, err := m.direct.Artist(ctx, mbid)
	if err != nil {
		return Artist{}, err
	}
	if !validDirectArtist(artist, mbid) {
		return Artist{}, ErrServiceInvalid
	}
	return artist, nil
}

func parseServiceArtist(item serviceItem, albums bool) (Artist, error) {
	if item.Source != "musicbrainz" || item.Kind != "artist" || !ValidMBID(item.SourceID) {
		return Artist{}, ErrServiceInvalid
	}
	var data serviceArtistData
	if err := item.decode(&data); err != nil || !ValidMBID(data.ID) || strings.TrimSpace(data.Name) == "" || data.Score < 0 || data.Score > 100 {
		return Artist{}, ErrServiceInvalid
	}
	out := Artist{MBID: strings.ToLower(data.ID), Name: data.Name, SortName: data.SortName, Type: strings.ToLower(data.Type),
		Disambiguation: data.Disambiguation, Country: data.Country, Score: data.Score, Status: "running", Popularity: data.Popularity.orNil()}
	if out.Popularity == nil {
		out.Popularity = item.Popularity.orNil()
	}
	if data.LifeSpan.Ended {
		out.Status = "ended"
	}
	if !albums {
		return out, nil
	}
	if data.ReleaseGroups == nil {
		return Artist{}, ErrServiceIncomplete
	}
	if len(*data.ReleaseGroups) > serviceReleaseLimit && !item.ReleaseGroupsComplete {
		return Artist{}, ErrServiceIncomplete
	}
	seen := make(map[string]bool, len(*data.ReleaseGroups))
	out.Albums = make([]Album, 0, len(*data.ReleaseGroups))
	for _, group := range *data.ReleaseGroups {
		id := strings.ToLower(group.ID)
		if !ValidMBID(id) || seen[id] || strings.TrimSpace(group.Title) == "" {
			return Artist{}, ErrServiceInvalid
		}
		seen[id] = true
		album := Album{MBID: id, Name: group.Title, PrimaryType: strings.ToLower(group.PrimaryType), ReleaseDate: group.FirstRelease,
			Disambiguation: group.Disambiguation, SecondaryTypes: make([]string, 0, len(group.SecondaryTypes))}
		for _, secondary := range group.SecondaryTypes {
			album.SecondaryTypes = append(album.SecondaryTypes, strings.ToLower(secondary))
		}
		out.Albums = append(out.Albums, album)
	}
	return out, nil
}

func validDirectArtist(artist Artist, mbid string) bool {
	if !strings.EqualFold(artist.MBID, mbid) || strings.TrimSpace(artist.Name) == "" {
		return false
	}
	seen := make(map[string]bool, len(artist.Albums))
	for _, album := range artist.Albums {
		id := strings.ToLower(album.MBID)
		if !ValidMBID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

type serviceReleaseGroupData struct {
	ID       string                `json:"id"`
	Releases *[]serviceReleaseLink `json:"releases"`
}

type serviceReleaseLink struct {
	ID string `json:"id"`
}

type serviceReleaseData struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Country        string `json:"country"`
	Date           string `json:"date"`
	Disambiguation string `json:"disambiguation"`
	ReleaseGroup   struct {
		ID string `json:"id"`
	} `json:"release-group"`
	Media *[]serviceMediumData `json:"media"`
}

type serviceMediumData struct {
	Position   int                 `json:"position"`
	Format     string              `json:"format"`
	TrackCount *int                `json:"track-count"`
	Tracks     *[]serviceTrackData `json:"tracks"`
}

type serviceTrackData struct {
	ID        string `json:"id"`
	Position  int    `json:"position"`
	Title     string `json:"title"`
	Length    *int   `json:"length"`
	Recording struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Length *int   `json:"length"`
	} `json:"recording"`
}

type serviceRecordingData struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Length *int   `json:"length"`
}

// AlbumArtist reads the artist credit the metadata server keeps for the
// album, else asks MusicBrainz directly.
func (m *serviceMusic) AlbumArtist(ctx context.Context, albumMBID string) (string, error) {
	if !ValidMBID(albumMBID) {
		return "", ErrArtistNotFound
	}
	if l := m.service.localMusic(); l != nil {
		if id, ok := l.LocalAlbumArtist(ctx, strings.ToLower(albumMBID)); ok {
			return id, nil
		}
	}
	sctx, cancel := serviceContext(ctx)
	item, err := m.service.detail(sctx, "musicbrainz", "release-group", strings.ToLower(albumMBID))
	cancel()
	if err == nil {
		if id, err := firstCreditID(item.SourceData); err == nil {
			return id, nil
		}
	}
	if f, ok := m.direct.(AlbumArtistFinder); ok {
		id, err := f.AlbumArtist(ctx, albumMBID)
		if errors.Is(err, ErrSourceOff) {
			// MusicBrainz is not asked in this mode: the same as no direct source.
			return "", ErrArtistNotFound
		}
		return id, err
	}
	return "", ErrArtistNotFound
}

// servicePressingFetches is how many releases of one album are asked at once.
const servicePressingFetches = 8

func (m *serviceMusic) Pressings(ctx context.Context, albumMBID string) ([]Pressing, error) {
	if !ValidMBID(albumMBID) {
		return nil, ErrArtistNotFound
	}
	albumMBID = strings.ToLower(albumMBID)
	// A page view takes the stored edition kept on disk when a package has it.
	if l := m.service.localMusic(); l != nil && IsMemberLookup(ctx) {
		if ps, ok, err := l.LocalPressings(ctx, albumMBID); ok {
			return ps, err
		}
	}
	ctx, cancel := serviceContext(ctx)
	defer cancel()
	// A page view takes the chosen edition's tracks in one call; the Library
	// (which keeps track ids) reads every edition below.
	if IsMemberLookup(ctx) {
		p, err := m.service.MusicAlbum(ctx, albumMBID)
		if err == nil {
			if ps, ok := p.pressings(); ok {
				if p.EditionCount <= 1 && p.EditionsComplete {
					return ps, nil
				}
				return ps, ErrListCut
			}
		}
		// A refusing or unreachable server is asked again later, never
		// through the per-edition fan-out (many requests into a refusal);
		// a warm-up ask (TracklistOnly) stops here too.
		if errors.Is(err, ErrServiceBusy) || errors.Is(err, ErrServiceUnavailable) || ctx.Err() != nil || IsTracklistOnly(ctx) {
			return nil, err
		}
	}
	item, err := m.service.detail(ctx, "musicbrainz", "release-group", albumMBID)
	if err != nil {
		return m.fallbackPressings(ctx, albumMBID, nil, err)
	}
	releaseIDs, err := parseServiceReleaseIDs(item)
	if err != nil {
		return m.fallbackPressings(ctx, albumMBID, nil, err)
	}
	if len(releaseIDs) > serviceReleaseLimit {
		return m.fallbackPressings(ctx, albumMBID, nil, ErrServiceIncomplete)
	}
	// Ask for the releases a few at a time (an album can have a hundred) and
	// read them in order as they land, so the result and its first error are
	// the same as asking one by one. At most servicePressingFetches answers
	// are held unread; the first error or the track cap stops the rest.
	type fetched struct {
		item serviceItem
		err  error
	}
	fctx, stop := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { stop(); wg.Wait() }()
	got := make([]chan fetched, len(releaseIDs))
	launch := func(i int) {
		if i >= len(releaseIDs) {
			return
		}
		got[i] = make(chan fetched, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			var f fetched
			f.item, f.err = m.service.detail(fctx, "musicbrainz", "release", releaseIDs[i])
			got[i] <- f
		}()
	}
	for i := 0; i < servicePressingFetches; i++ {
		launch(i)
	}
	out := make([]Pressing, 0, len(releaseIDs))
	totalTracks := 0
	for i := range releaseIDs {
		f := <-got[i]
		got[i] = nil
		release, err := f.item, f.err
		if err == nil {
			launch(i + servicePressingFetches)
		} else {
			stop()
		}
		if err != nil {
			return m.fallbackPressings(ctx, albumMBID, out, err)
		}
		pressing, err := parseServicePressingItem(ctx, m.service, albumMBID, release, &totalTracks)
		if err != nil {
			return m.fallbackPressings(ctx, albumMBID, out, err)
		}
		out = append(out, pressing)
	}
	if m.direct != nil {
		direct, directErr := m.direct.Pressings(ctx, albumMBID)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if directErr == nil && validDirectPressings(direct) {
			return direct, nil
		}
	}
	return out, ErrListCut
}

func (m *serviceMusic) fallbackPressings(ctx context.Context, albumMBID string, partial []Pressing, err error) ([]Pressing, error) {
	if m.direct == nil || !serviceFallback(ctx, err) {
		return partial, serviceMusicError(err)
	}
	direct, directErr := m.direct.Pressings(ctx, albumMBID)
	if directErr != nil {
		return direct, directErr
	}
	if !validDirectPressings(direct) {
		return nil, ErrServiceInvalid
	}
	return direct, nil
}

func serviceMusicError(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrArtistNotFound
	}
	return err
}

func parseServiceReleaseIDs(item serviceItem) ([]string, error) {
	if item.Source != "musicbrainz" || item.Kind != "release-group" || !ValidMBID(item.SourceID) {
		return nil, ErrServiceInvalid
	}
	var data serviceReleaseGroupData
	if err := item.decode(&data); err != nil {
		return nil, err
	}
	if data.Releases == nil {
		return nil, ErrServiceIncomplete
	}
	if len(*data.Releases) > serviceReleaseLimit {
		return nil, ErrServiceIncomplete
	}
	seen := make(map[string]bool, len(*data.Releases))
	out := make([]string, 0, len(*data.Releases))
	for _, release := range *data.Releases {
		id := strings.ToLower(release.ID)
		if !ValidMBID(id) || seen[id] {
			return nil, ErrServiceInvalid
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func parseServicePressingItem(ctx context.Context, service *Service, albumMBID string, item serviceItem, totalTracks *int) (Pressing, error) {
	if item.Source != "musicbrainz" || item.Kind != "release" || !ValidMBID(item.SourceID) {
		return Pressing{}, ErrServiceInvalid
	}
	var data serviceReleaseData
	if err := item.decode(&data); err != nil {
		return Pressing{}, err
	}
	if !strings.EqualFold(data.ReleaseGroup.ID, albumMBID) || data.Media == nil || strings.TrimSpace(data.Title) == "" {
		return Pressing{}, ErrServiceInvalid
	}
	out := Pressing{MBID: strings.ToLower(data.ID), Name: data.Title, Status: strings.ToLower(data.Status), Country: data.Country,
		Date: data.Date, Disambiguation: data.Disambiguation, Formats: make([]string, 0, len(*data.Media)), Tracks: []Track{}}
	position := 0
	seenDiscs := make(map[int]bool, len(*data.Media))
	for mediumIndex, medium := range *data.Media {
		if medium.Tracks == nil {
			return Pressing{}, ErrServiceIncomplete
		}
		if medium.TrackCount != nil && (*medium.TrackCount < 0 || *medium.TrackCount != len(*medium.Tracks)) {
			return Pressing{}, ErrServiceIncomplete
		}
		disc := medium.Position
		if disc <= 0 {
			disc = mediumIndex + 1
		}
		if disc > len(*data.Media) || seenDiscs[disc] {
			return Pressing{}, ErrServiceInvalid
		}
		seenDiscs[disc] = true
		out.Formats = append(out.Formats, medium.Format)
		seenTrackPositions := make(map[int]bool, len(*medium.Tracks))
		for trackIndex, raw := range *medium.Tracks {
			(*totalTracks)++
			if *totalTracks > serviceTrackLimit {
				return Pressing{}, ErrServiceIncomplete
			}
			if !ValidMBID(raw.ID) || !ValidMBID(raw.Recording.ID) {
				return Pressing{}, ErrServiceInvalid
			}
			position++
			trackPosition := raw.Position
			if trackPosition <= 0 {
				trackPosition = trackIndex + 1
			}
			if trackPosition > len(*medium.Tracks) || seenTrackPositions[trackPosition] {
				return Pressing{}, ErrServiceInvalid
			}
			seenTrackPositions[trackPosition] = true
			track := Track{ID: strings.ToLower(raw.ID), RecordingID: strings.ToLower(raw.Recording.ID), Disc: disc,
				Number: trackPosition, Position: position, Name: raw.Title}
			if track.Name == "" {
				track.Name = raw.Recording.Title
			}
			if raw.Length != nil {
				track.LengthMS = *raw.Length
			} else if raw.Recording.Length != nil {
				track.LengthMS = *raw.Recording.Length
			}
			if track.LengthMS < 0 {
				return Pressing{}, ErrServiceInvalid
			}
			if track.Name == "" || track.LengthMS == 0 {
				if service == nil {
					return Pressing{}, ErrServiceIncomplete
				}
				recordingItem, err := service.detail(ctx, "musicbrainz", "recording", track.RecordingID)
				if err != nil {
					return Pressing{}, err
				}
				var recording serviceRecordingData
				if recordingItem.Kind != "recording" || recordingItem.decode(&recording) != nil || !strings.EqualFold(recording.ID, track.RecordingID) {
					return Pressing{}, ErrServiceInvalid
				}
				if track.Name == "" {
					track.Name = recording.Title
				}
				if track.LengthMS == 0 && recording.Length != nil {
					track.LengthMS = *recording.Length
				}
			}
			if strings.TrimSpace(track.Name) == "" || track.LengthMS < 0 {
				return Pressing{}, ErrServiceIncomplete
			}
			out.Tracks = append(out.Tracks, track)
		}
	}
	return out, nil
}

func validDirectPressings(pressings []Pressing) bool {
	seen := make(map[string]bool, len(pressings))
	for _, pressing := range pressings {
		id := strings.ToLower(pressing.MBID)
		if !ValidMBID(id) || seen[id] {
			return false
		}
		seen[id] = true
		for _, track := range pressing.Tracks {
			if !ValidMBID(track.ID) || !ValidMBID(track.RecordingID) {
				return false
			}
		}
	}
	return true
}

var _ ArtistProvider = (*serviceMusic)(nil)
