package web

import (
	"context"
	"sync"
	"time"

	"github.com/yornik/go-practice-web/internal/runnerapi"
)

const (
	// runnerInfoTTL is how long an answer of GET /v1/info is used. A new
	// runner pod can start on another node, where the race detector may
	// work when it did not before, or the other way round.
	runnerInfoTTL = 5 * time.Minute
	// runnerNoRaceTTL is how long an answer without a race detector is
	// used. The 2 stale answers cost differently: a stale "available"
	// costs nothing, because the runner drops -race itself and its done
	// event corrects the answer at once; a stale "unavailable" sends runs
	// without -race to a new runner pod that has a race detector, and such
	// a run can pass the exercise.
	runnerNoRaceTTL = 30 * time.Second
	// runnerInfoRetry is how long after a failed fetch the next run asks
	// again. Meanwhile the last answer stands, or none.
	runnerInfoRetry = 10 * time.Second
)

// runnerInfoCache holds the runner's last answer to GET /v1/info.
type runnerInfoCache struct {
	mu        sync.Mutex
	info      runnerapi.Info
	have      bool
	fetchedAt time.Time
	failedAt  time.Time
}

// runnerInfo returns the runner's info, fetched at most once per
// runnerInfoTTL (runnerNoRaceTTL while the race detector does not work).
// ok is false while the site never got an answer. The lock is held during
// the fetch, so concurrent runs ask the runner once.
func (s *Server) runnerInfo(ctx context.Context) (info runnerapi.Info, ok bool) {
	c := &s.info
	c.mu.Lock()
	defer c.mu.Unlock()
	now := s.now()
	ttl := runnerInfoTTL
	if !c.info.RaceAvailable {
		ttl = runnerNoRaceTTL
	}
	if c.have && now.Sub(c.fetchedAt) < ttl {
		return c.info, true
	}
	if !c.failedAt.IsZero() && now.Sub(c.failedAt) < runnerInfoRetry {
		return c.info, c.have
	}
	// Detached from the request: a browser that leaves must not leave a
	// failure behind for everyone's next run.
	got, err := s.cfg.Runner.Info(context.WithoutCancel(ctx))
	if err != nil {
		s.log.Warn("runner info", "err", err)
		c.failedAt = now
		return c.info, c.have
	}
	c.info, c.have, c.fetchedAt, c.failedAt = got, true, now, time.Time{}
	return got, true
}

// raceUnavailable reports whether the runner said its race detector does
// not work. Without an answer the site assumes it works: the runner then
// drops -race itself and says so in its done event.
func (s *Server) raceUnavailable(ctx context.Context) bool {
	info, ok := s.runnerInfo(ctx)
	return ok && !info.RaceAvailable
}

// noteRaceUnavailable records a done event in which the runner ran
// without -race although the site asked for it: the cached answer is out
// of date.
func (s *Server) noteRaceUnavailable() {
	c := &s.info
	c.mu.Lock()
	defer c.mu.Unlock()
	c.info.RaceAvailable, c.have, c.fetchedAt, c.failedAt = false, true, s.now(), time.Time{}
}

// forgetRunnerInfo drops the cached answer after a failed runner call: the
// runner may be restarting, maybe on another node.
func (s *Server) forgetRunnerInfo() {
	c := &s.info
	c.mu.Lock()
	defer c.mu.Unlock()
	c.have, c.fetchedAt = false, time.Time{}
}
