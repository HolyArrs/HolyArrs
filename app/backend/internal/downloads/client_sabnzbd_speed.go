package downloads

import (
	"math"
	"sync"
	"time"
)

// SABnzbd reports one speed for the whole queue and calls every unpaused job
// Downloading, the waiting ones too. It fetches its queue in order, and near
// the end of a job it starts on the next one while the first one finishes.
// The only sign of which jobs the speed belongs to is how fast each job's
// bytes left go down between reads, so the client measures that.

const (
	// sabRateGap is the shortest time that a job's rate is measured over.
	// A read sooner than that after the last measure keeps the last rate.
	sabRateGap = time.Second
	// sabRateForget drops a job that no read has seen for this long.
	sabRateForget = 10 * time.Minute
)

// sabMeter keeps the bytes left of each Downloading job from earlier reads.
type sabMeter struct {
	mu   sync.Mutex
	seen map[string]sabSample
}

type sabSample struct {
	left float64   // bytes left at the last measure
	at   time.Time // time of the last measure
	rate float64   // bytes per second up to the last measure; -1 before the first
}

// rates records the bytes left of each Downloading job in jobs, read at now,
// and returns the measured rate of each one in bytes per second. A job that
// is read for the first time has no rate yet.
func (m *sabMeter) rates(jobs []Job, now time.Time) map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen == nil {
		m.seen = map[string]sabSample{}
	}
	out := map[string]float64{}
	for _, j := range jobs {
		if j.State != StateDownloading {
			delete(m.seen, j.ID)
			continue
		}
		left := float64(j.Size) * (1 - j.Progress)
		s, ok := m.seen[j.ID]
		switch {
		case !ok:
			s = sabSample{left: left, at: now, rate: -1}
		case now.Sub(s.at) >= sabRateGap:
			// Bytes left can go up when SABnzbd fetches repair blocks; that
			// counts as no progress, not as negative speed.
			s.rate = max(s.left-left, 0) / now.Sub(s.at).Seconds()
			s.left, s.at = left, now
		}
		m.seen[j.ID] = s
		if s.rate >= 0 {
			out[j.ID] = s.rate
		}
	}
	for id, s := range m.seen {
		if now.Sub(s.at) > sabRateForget {
			delete(m.seen, id)
		}
	}
	return out
}

// sabShare sets the speed of each Downloading job from SABnzbd's total speed
// and the rates measured for the jobs. When whole is true, jobs is the whole
// queue, so the total is split in proportion to the rates. A filtered list
// may lack the jobs that take the rest of the total, so each of its jobs gets
// its own measured rate instead, up to the total. When nothing has been
// measured yet, the whole queue's first Downloading job gets the total,
// because SABnzbd fetches in queue order.
func sabShare(jobs []Job, rates map[string]float64, total int64, whole bool) {
	if total <= 0 {
		return
	}
	var sum float64
	for _, j := range jobs {
		if j.State == StateDownloading {
			sum += rates[j.ID]
		}
	}
	for i := range jobs {
		j := &jobs[i]
		if j.State != StateDownloading {
			continue
		}
		switch {
		case sum <= 0 && whole:
			j.Speed = total
			return
		case sum <= 0:
			return
		case whole:
			j.Speed = int64(math.Round(float64(total) * rates[j.ID] / sum))
		default:
			j.Speed = min(int64(math.Round(rates[j.ID])), total)
		}
	}
}

func (s *Sabnzbd) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}
