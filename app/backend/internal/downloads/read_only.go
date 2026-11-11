package downloads

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ReadOnlyEnv names the env var that lists Downloaders to treat as read only.
const ReadOnlyEnv = "HOLYARRS_READONLY_DOWNLOADERS"

// ErrReadOnly is returned when a write is tried on a read only Downloader.
var ErrReadOnly = errors.New("this Downloader is read only here; HolyArrs does not change it")

var optionalInterfaces = []reflect.Type{
	reflect.TypeFor[Controller](),
	reflect.TypeFor[TorrentController](),
	reflect.TypeFor[TorrentInspector](),
	reflect.TypeFor[Retrier](),
	reflect.TypeFor[AllLister](),
	reflect.TypeFor[ActiveLister](),
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

type readOnlyMarker interface{ readOnlyInner() Engine }

type roCore struct {
	inner    Engine
	allowAdd bool
}

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

// roRTorrent is rTorrent's shape: the engine, the row controls and the
// background read of what is still downloading.
type roRTorrent struct {
	*roCore
	roController
	roActiveLister
}

// roTransmission is Transmission's shape: the engine plus the full list.
type roTransmission struct {
	*roCore
	roAllLister
}

// roAria2 is aria2's shape: the engine plus the row controls.
type roAria2 struct {
	*roCore
	roController
}

type roUsenetQueue struct{ c *roCore }

type roController struct{ c *roCore }

type roTorrentController struct{ c *roCore }

type roTorrentInspector struct{ c *roCore }

type roRetrier struct{ c *roCore }

type roAllLister struct{ c *roCore }

type roActiveLister struct{ c *roCore }

type roTorrentAdder struct{ c *roCore }

type roTorrentFolders struct{ c *roCore }

type roTorrentBulk struct{ c *roCore }

type roShareLimiter struct{ c *roCore }

type roSlowModer struct{ c *roCore }

type roTrackerEditor struct{ c *roCore }

type roMover struct{ c *roCore }

type roMainSyncer struct{ c *roCore }

type roHistoryReader struct{ c *roCore }

type roQueueReader struct{ c *roCore }

type roUsenetInspector struct{ c *roCore }

// ReadOnlyByEnv reports whether the env var marks d as read only.
func ReadOnlyByEnv(d domain.Downloader) bool {
	return readOnlyListed(os.Getenv(ReadOnlyEnv), d)
}

// ReadOnlyFor reports whether d must not be changed by HolyArrs.
func ReadOnlyFor(d domain.Downloader) bool { return readOnlyFor(d) }

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

func (r *roCore) Remove(context.Context, string, bool) error { return ErrReadOnly }

func (roUsenetQueue) MoveItem(context.Context, string, string) error { return ErrReadOnly }

func (roUsenetQueue) SetPriority(context.Context, string, string) error { return ErrReadOnly }

func (roController) PauseItem(context.Context, string) error { return ErrReadOnly }

func (roController) ResumeItem(context.Context, string) error { return ErrReadOnly }

func (roController) PauseAll(context.Context) error { return ErrReadOnly }

func (roController) ResumeAll(context.Context) error { return ErrReadOnly }

func (roController) MoveTop(context.Context, string) error { return ErrReadOnly }

func (roController) SetSpeedLimit(context.Context, int64, int64) error { return ErrReadOnly }

func (r roController) Status(ctx context.Context) (ClientStatus, error) {
	return r.c.inner.(Controller).Status(ctx)
}

func (roTorrentController) Recheck(context.Context, string) error { return ErrReadOnly }

func (roTorrentController) Reannounce(context.Context, string) error { return ErrReadOnly }

func (r roTorrentController) Trackers(ctx context.Context, id string) ([]Tracker, error) {
	return r.c.inner.(TorrentController).Trackers(ctx, id)
}

func (r roTorrentController) Peers(ctx context.Context, id string) ([]Peer, error) {
	return r.c.inner.(TorrentController).Peers(ctx, id)
}

func (r roTorrentInspector) Properties(ctx context.Context, id string) (TorrentDetail, error) {
	return r.c.inner.(TorrentInspector).Properties(ctx, id)
}

func (r roTorrentInspector) TorrentFiles(ctx context.Context, id string) ([]TorrentFile, error) {
	return r.c.inner.(TorrentInspector).TorrentFiles(ctx, id)
}

func (roTorrentInspector) SetFilePriority(context.Context, string, []int, int) error {
	return ErrReadOnly
}

func (roRetrier) Retry(context.Context, string) (string, error) { return "", ErrReadOnly }

func (r roAllLister) ListAll(ctx context.Context) ([]Job, error) {
	return r.c.inner.(AllLister).ListAll(ctx)
}

func (r roActiveLister) ListActive(ctx context.Context, ids []string) ([]Job, error) {
	return r.c.inner.(ActiveLister).ListActive(ctx, ids)
}

func (r roTorrentAdder) AddTorrents(ctx context.Context, b AddBatch) (AddOutcome, error) {
	if r.c.allowAdd {
		return r.c.inner.(TorrentAdder).AddTorrents(ctx, b)
	}
	return AddOutcome{}, ErrReadOnly
}

func (r roTorrentFolders) SaveBase(ctx context.Context) (SaveBase, error) {
	return r.c.inner.(TorrentFolders).SaveBase(ctx)
}

func (roTorrentBulk) Bulk(context.Context, string, []string) error { return ErrReadOnly }

func (roTorrentBulk) RemoveMany(context.Context, []string, bool) error { return ErrReadOnly }

func (roShareLimiter) SetShareLimits(context.Context, []string, ShareLimits) error {
	return ErrReadOnly
}

func (roSlowModer) SetSlowMode(context.Context, bool) error { return ErrReadOnly }

func (roTrackerEditor) AddTrackers(context.Context, string, []string) error { return ErrReadOnly }

func (roTrackerEditor) EditTracker(context.Context, string, string, string, *int) error {
	return ErrReadOnly
}

func (roTrackerEditor) RemoveTrackers(context.Context, string, []string) error { return ErrReadOnly }

func (roMover) SetLocation(context.Context, []string, string) error { return ErrReadOnly }

func (roMover) SetTorrentLimits(context.Context, []string, int64, int64) error { return ErrReadOnly }

func (r roMainSyncer) SyncMain(ctx context.Context, rid int64) (MainData, error) {
	return r.c.inner.(MainSyncer).SyncMain(ctx, rid)
}

func (r roHistoryReader) History(ctx context.Context, since int64) (HistoryPage, error) {
	return r.c.inner.(HistoryReader).History(ctx, since)
}

func (r roQueueReader) Queue(ctx context.Context) (QueuePage, error) {
	return r.c.inner.(QueueReader).Queue(ctx)
}

func (r roUsenetInspector) JobStages(ctx context.Context, id string) ([]JobStage, error) {
	return r.c.inner.(UsenetInspector).JobStages(ctx, id)
}

func (r roUsenetInspector) ServerStats(ctx context.Context) (ServerStats, error) {
	return r.c.inner.(UsenetInspector).ServerStats(ctx)
}

func readOnlyFor(d domain.Downloader) bool {
	return !d.Writable || ReadOnlyByEnv(d)
}

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

func readOnly(inner Engine) Engine { return readOnlyWrap(inner, false) }

func readOnlyAdds(inner Engine) Engine { return readOnlyWrap(inner, true) }

func readOnlyWrap(inner Engine, allowAdd bool) Engine {
	c := &roCore{inner: inner, allowAdd: allowAdd}
	switch shapeOf(inner) {
	case shapeOf(&Qbittorrent{}):
		return roQbit{c, roController{c}, roTorrentController{c}, roTorrentInspector{c}, roAllLister{c}, roTorrentAdder{c},
			roTorrentFolders{c}, roTorrentBulk{c}, roShareLimiter{c}, roSlowModer{c}, roTrackerEditor{c}, roMover{c}, roMainSyncer{c}}
	case shapeOf(&Sabnzbd{}):
		return roSab{c, roController{c}, roRetrier{c}, roHistoryReader{c}, roQueueReader{c}, roUsenetInspector{c}, roUsenetQueue{c}}
	case shapeOf(&RTorrent{}):
		return roRTorrent{c, roController{c}, roActiveLister{c}}
	case shapeOf(&Aria2{}):
		return roAria2{c, roController{c}}
	case shapeOf(&Transmission{}):
		return roTransmission{c, roAllLister{c}}
	}
	return c
}

func isReadOnly(e Engine) bool { _, ok := e.(readOnlyMarker); return ok }

func addAllowed(e Engine) bool {
	r, ok := e.(*roCore)
	if !ok {
		if m, isRO := e.(interface{ readOnlyCore() *roCore }); isRO {
			r, ok = m.readOnlyCore(), true
		}
	}
	return !isReadOnly(e) || ok && r.allowAdd
}

func unwrapReadOnly(e Engine) Engine {
	if r, ok := e.(readOnlyMarker); ok {
		return r.readOnlyInner()
	}
	return e
}

func (r *roCore) readOnlyInner() Engine { return r.inner }

func (r *roCore) readOnlyCore() *roCore { return r }
