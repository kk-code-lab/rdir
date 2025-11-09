package search

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// IndexTelemetry carries progress information for the async index builder.
type IndexTelemetry struct {
	RootPath        string
	Building        bool
	Ready           bool
	Disabled        bool
	UseIndex        bool
	FilesIndexed    int
	Threshold       int
	MaxIndexResults int
	StartedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     time.Time
	Duration        time.Duration
	LastError       string
}

var (
	progressDebugEnabled = os.Getenv("RDIR_DEBUG_PROGRESS") == "1"
	progressDebugVerbose = os.Getenv("RDIR_DEBUG_PROGRESS_VERBOSE") == "1"
	progressDebugMu      sync.Mutex
)

func progressDebugf(format string, args ...interface{}) {
	if !progressDebugEnabled {
		return
	}
	progressDebugMu.Lock()
	defer progressDebugMu.Unlock()

	f, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	timestamp := time.Now().Format(time.RFC3339Nano)
	_, _ = fmt.Fprintf(f, "%s "+format+"\n", append([]interface{}{timestamp}, args...)...)
	_ = f.Close()
}

type progressTracker struct {
	emit         func(func(*IndexTelemetry))
	start        time.Time
	interval     time.Duration
	lastEmit     time.Time
	lastReported int
	clock        func() time.Time
}

func newProgressTracker(start time.Time, interval time.Duration, emit func(func(*IndexTelemetry))) *progressTracker {
	if interval <= 0 {
		interval = indexProgressInterval
	}
	return &progressTracker{
		emit:         emit,
		start:        start,
		interval:     interval,
		lastEmit:     start.Add(-interval),
		lastReported: 0,
		clock:        time.Now,
	}
}

func (pt *progressTracker) withClock(clock func() time.Time) *progressTracker {
	pt.clock = clock
	return pt
}

func (pt *progressTracker) update(count int) {
	if count <= pt.lastReported {
		return
	}

	now := pt.clock()
	if count <= 2048 || pt.lastReported == 0 || now.Sub(pt.lastEmit) >= pt.interval || count-pt.lastReported >= 1024 {
		pt.emitEmission(count, now)
		pt.lastEmit = now
		pt.lastReported = count
	}
}

func (pt *progressTracker) flush(count int) {
	if count <= pt.lastReported {
		progressDebugf("progress flush skip count=%d lastReported=%d", count, pt.lastReported)
		return
	}
	now := pt.clock()
	progressDebugf("progress flush emit count=%d lastReported=%d", count, pt.lastReported)
	pt.emitEmission(count, now)
	pt.lastEmit = now
	pt.lastReported = count
}

func (pt *progressTracker) emitEmission(count int, now time.Time) {
	progressDebugf("progress emit count=%d", count)
	pt.emit(func(p *IndexTelemetry) {
		if p.StartedAt.IsZero() {
			p.StartedAt = pt.start
		}
		p.Building = true
		p.Ready = false
		p.Disabled = false
		p.FilesIndexed = count
		p.UpdatedAt = now
	})
}
