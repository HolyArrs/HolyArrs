package downloader

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// ErrReadOnly is a change refused because the Downloader is marked read
// only for this HolyArrs (its switch is off, or HOLYARRS_READONLY_DOWNLOADERS). It never reaches
// the client.
var ErrReadOnly = errors.New("this Downloader is read only here; HolyArrs does not change it")

// ReadOnlyEnv names the setting that marks Downloaders read only: a comma
// list of Downloader ids and/or base addresses.
const ReadOnlyEnv = "HOLYARRS_READONLY_DOWNLOADERS"

// readOnlyFor says whether HolyArrs may not change d: its "HolyArrs can
// change this Downloader" switch is off, or it is listed in
// HOLYARRS_READONLY_DOWNLOADERS (a hard override).
func readOnlyFor(d domain.Downloader) bool {
	return !d.Writable || ReadOnlyByEnv(d)
}

// ReadOnlyByEnv says whether d is listed in HOLYARRS_READONLY_DOWNLOADERS, by
// id or by base address (so a recreated Downloader row stays covered).
func ReadOnlyByEnv(d domain.Downloader) bool {
	return readOnlyListed(os.Getenv(ReadOnlyEnv), d)
}

// ReadOnlyFor is readOnlyFor for callers outside this package (the
// Downloads screen hides controls on these rows).
func ReadOnlyFor(d domain.Downloader) bool { return readOnlyFor(d) }

func readOnlyListed(list string, d domain.Downloader) bool {
	base := strings.TrimRight(strings.ToLower(strings.TrimSpace(d.URL)), "/")
	for _, v := range strings.Split(list, ",") {
		v = strings.TrimRight(strings.ToLower(strings.TrimSpace(v)), "/")
		if v == "" {
			continue
		}
		if v == strconv.FormatInt(d.ID, 10) || (base != "" && v == base) {
			return true
		}
	}
	return false
}

// optionalInterfaces is the registry of every optional engine part. The
// guard test and the read-only test walk it; a new exported interface in
// this package that is not listed here (or in notOptional) fails a test.
var optionalInterfaces = []reflect.Type{
	reflect.TypeFor[Controller](),
	reflect.TypeFor[TorrentController](),
	reflect.TypeFor[TorrentInspector](),
	reflect.TypeFor[Retrier](),
	reflect.TypeFor[AllLister](),
	reflect.TypeFor[TorrentAdder](),
	reflect.TypeFor[TorrentFolders](),
	reflect.TypeFor[TorrentBulk](),
	reflect.TypeFor[ShareLimiter](),
	reflect.TypeFor[SlowModer](),
	reflect.TypeFor[TrackerEditor](),
	reflect.TypeFor[Mover](),
	reflect.TypeFor[MainSyncer](),
	reflect.TypeFor[HistoryReader](),
	reflect.TypeFor[Syncer](),
	reflect.TypeFor[QueueReader](),
	reflect.TypeFor[UsenetInspector](),
	reflect.TypeFor[UsenetQueue](),
}

func shapeOf(e any) uint64 {
	var m uint64
	t := reflect.TypeOf(e)
	for i, it := range optionalInterfaces {
		if t != nil && t.Implements(it) {
			m |= 1 << i
		}
	}
	return m
}

// readOnly wraps an inner engine (inside the guard) so reads pass and every
// change answers ErrReadOnly without contacting the client. The wrapper has
// exactly the inner engine's optional parts, so feature checks by type
// assertion see the same engine; an engine of an unknown shape keeps only
// the five Engine methods (fewer controls, never a write).
func readOnly(inner Engine) Engine { return readOnlyWrap(inner, false) }

// readOnlyAdds is readOnly that still lets new downloads through (Add and
// AddTorrents): a Downloader whose "HolyArrs can change this Downloader"
// switch is off takes grabs, but nothing already in it is changed.
func readOnlyAdds(inner Engine) Engine { return readOnlyWrap(inner, true) }

func readOnlyWrap(inner Engine, allowAdd bool) Engine {
	c := &roCore{inner: inner, allowAdd: allowAdd}
	switch shapeOf(inner) {
	case shapeOf(&Qbittorrent{}):
		return roQbit{c, roController{c}, roTorrentController{c}, roTorrentInspector{c}, roAllLister{c}, roTorrentAdder{c},
			roTorrentFolders{c}, roTorrentBulk{c}, roShareLimiter{c}, roSlowModer{c}, roTrackerEditor{c}, roMover{c}, roMainSyncer{c}}
	case shapeOf(&Sabnzbd{}):
		return roSab{c, roController{c}, roRetrier{c}, roHistoryReader{c}, roQueueReader{c}, roUsenetInspector{c}, roUsenetQueue{c}}
	}
	return c
}

type readOnlyMarker interface{ readOnlyInner() Engine }

func isReadOnly(e Engine) bool { _, ok := e.(readOnlyMarker); return ok }

// addAllowed says whether e takes new downloads: not read only, or read
// only by its switch (which still lets grabs through).
func addAllowed(e Engine) bool {
	r, ok := e.(*roCore)
	if !ok {
		if m, isRO := e.(interface{ readOnlyCore() *roCore }); isRO {
			r, ok = m.readOnlyCore(), true
		}
	}
	return !isReadOnly(e) || ok && r.allowAdd
}

// unwrapReadOnly gives the engine under a read-only wrapper (for reads that
// need the concrete adapter, such as SABnzbd's category check).
func unwrapReadOnly(e Engine) Engine {
	if r, ok := e.(readOnlyMarker); ok {
		return r.readOnlyInner()
	}
	return e
}

type roCore struct {
	inner    Engine
	allowAdd bool
}

func (r *roCore) readOnlyInner() Engine                      { return r.inner }
func (r *roCore) readOnlyCore() *roCore                      { return r }
func (r *roCore) Health(ctx context.Context) (Health, error) { return r.inner.Health(ctx) }
func (r *roCore) Add(ctx context.Context, req AddRequest) (string, error) {
	if r.allowAdd {
		return r.inner.Add(ctx, req)
	}
	return "", ErrReadOnly
}
func (r *roCore) List(ctx context.Context, category string) ([]Job, error) {
	return r.inner.List(ctx, category)
}
func (r *roCore) Get(ctx context.Context, id string) (Job, error) { return r.inner.Get(ctx, id) }
func (r *roCore) Remove(context.Context, string, bool) error      { return ErrReadOnly }

type roQbit struct {
	*roCore
	roController
	roTorrentController
	roTorrentInspector
	roAllLister
	roTorrentAdder
	roTorrentFolders
	roTorrentBulk
	roShareLimiter
	roSlowModer
	roTrackerEditor
	roMover
	roMainSyncer
}

type roSab struct {
	*roCore
	roController
	roRetrier
	roHistoryReader
	roQueueReader
	roUsenetInspector
	roUsenetQueue
}

type roUsenetQueue struct{ c *roCore }

func (roUsenetQueue) MoveItem(context.Context, string, string) error    { return ErrReadOnly }
func (roUsenetQueue) SetPriority(context.Context, string, string) error { return ErrReadOnly }

type roController struct{ c *roCore }

func (roController) PauseItem(context.Context, string) error           { return ErrReadOnly }
func (roController) ResumeItem(context.Context, string) error          { return ErrReadOnly }
func (roController) PauseAll(context.Context) error                    { return ErrReadOnly }
func (roController) ResumeAll(context.Context) error                   { return ErrReadOnly }
func (roController) MoveTop(context.Context, string) error             { return ErrReadOnly }
func (roController) SetSpeedLimit(context.Context, int64, int64) error { return ErrReadOnly }
func (r roController) Status(ctx context.Context) (ClientStatus, error) {
	return r.c.inner.(Controller).Status(ctx)
}

type roTorrentController struct{ c *roCore }

func (roTorrentController) Recheck(context.Context, string) error    { return ErrReadOnly }
func (roTorrentController) Reannounce(context.Context, string) error { return ErrReadOnly }
func (r roTorrentController) Trackers(ctx context.Context, id string) ([]Tracker, error) {
	return r.c.inner.(TorrentController).Trackers(ctx, id)
}
func (r roTorrentController) Peers(ctx context.Context, id string) ([]Peer, error) {
	return r.c.inner.(TorrentController).Peers(ctx, id)
}

type roTorrentInspector struct{ c *roCore }

func (r roTorrentInspector) Properties(ctx context.Context, id string) (TorrentDetail, error) {
	return r.c.inner.(TorrentInspector).Properties(ctx, id)
}
func (r roTorrentInspector) TorrentFiles(ctx context.Context, id string) ([]TorrentFile, error) {
	return r.c.inner.(TorrentInspector).TorrentFiles(ctx, id)
}
func (roTorrentInspector) SetFilePriority(context.Context, string, []int, int) error {
	return ErrReadOnly
}

type roRetrier struct{ c *roCore }

func (roRetrier) Retry(context.Context, string) (string, error) { return "", ErrReadOnly }

type roAllLister struct{ c *roCore }

func (r roAllLister) ListAll(ctx context.Context) ([]Job, error) {
	return r.c.inner.(AllLister).ListAll(ctx)
}

type roTorrentAdder struct{ c *roCore }

func (r roTorrentAdder) AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error) {
	if r.c.allowAdd {
		return r.c.inner.(TorrentAdder).AddTorrents(ctx, b)
	}
	return AddOutcome{}, ErrReadOnly
}

type roTorrentFolders struct{ c *roCore }

func (r roTorrentFolders) SaveBase(ctx context.Context) (SaveBase, error) {
	return r.c.inner.(TorrentFolders).SaveBase(ctx)
}

type roTorrentBulk struct{ c *roCore }

func (roTorrentBulk) Bulk(context.Context, string, []string) error     { return ErrReadOnly }
func (roTorrentBulk) RemoveMany(context.Context, []string, bool) error { return ErrReadOnly }

type roShareLimiter struct{ c *roCore }

func (roShareLimiter) SetShareLimits(context.Context, []string, ShareLimits) error {
	return ErrReadOnly
}

type roSlowModer struct{ c *roCore }

func (roSlowModer) SetSlowMode(context.Context, bool) error { return ErrReadOnly }

type roTrackerEditor struct{ c *roCore }

func (roTrackerEditor) AddTrackers(context.Context, string, []string) error { return ErrReadOnly }
func (roTrackerEditor) EditTracker(context.Context, string, string, string, *int) error {
	return ErrReadOnly
}
func (roTrackerEditor) RemoveTrackers(context.Context, string, []string) error { return ErrReadOnly }

type roMover struct{ c *roCore }

func (roMover) SetLocation(context.Context, []string, string) error            { return ErrReadOnly }
func (roMover) SetTorrentLimits(context.Context, []string, int64, int64) error { return ErrReadOnly }

type roMainSyncer struct{ c *roCore }

func (r roMainSyncer) SyncMain(ctx context.Context, rid int64) (MainData, error) {
	return r.c.inner.(MainSyncer).SyncMain(ctx, rid)
}

type roHistoryReader struct{ c *roCore }

func (r roHistoryReader) History(ctx context.Context, since int64) (HistoryPage, error) {
	return r.c.inner.(HistoryReader).History(ctx, since)
}

type roQueueReader struct{ c *roCore }

func (r roQueueReader) Queue(ctx context.Context) (QueuePage, error) {
	return r.c.inner.(QueueReader).Queue(ctx)
}

type roUsenetInspector struct{ c *roCore }

func (r roUsenetInspector) JobStages(ctx context.Context, id string) ([]JobStage, error) {
	return r.c.inner.(UsenetInspector).JobStages(ctx, id)
}
func (r roUsenetInspector) ServerStats(ctx context.Context) (ServerStats, error) {
	return r.c.inner.(UsenetInspector).ServerStats(ctx)
}
