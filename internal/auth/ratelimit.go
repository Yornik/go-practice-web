package auth

import (
	"math"
	"strconv"
	"sync"
	"time"
)

// Limiter is an in-memory token bucket per key: n events per period, with
// a burst of n. The site runs as 1 replica, so memory is the right place;
// a restart forgets the counts, which only makes the limits kinder.
type Limiter struct {
	mu        sync.Mutex
	n         float64
	per       time.Duration
	now       func() time.Time
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

// sweepEvery bounds how long full buckets linger: an attacker who rotates
// keys would otherwise grow the map without end.
const sweepEvery = time.Minute

// NewLimiter allows n events per period for each key. now is the clock;
// nil means time.Now.
func NewLimiter(n int, per time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{n: float64(n), per: per, now: now, buckets: map[string]*bucket{}}
}

// Rate returns the limit: n events per period.
func (l *Limiter) Rate() (n int, per time.Duration) { return int(l.n), l.per }

// WaitText says a wait as a person reads it, rounded up as Retry-After
// rounds it: whole seconds under a minute ("1 second", "20 seconds"),
// else whole minutes ("1 minute", "6 minutes").
func WaitText(d time.Duration) string {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	if secs < 60 {
		return plural(secs, "second")
	}
	return plural((secs+59)/60, "minute")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

// Allow takes 1 token for key. When the bucket is empty it returns false
// and how long until the next token.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= sweepEvery {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.n, at: now}
		l.buckets[key] = b
	}
	b.tokens = l.refill(b, now)
	b.at = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	perToken := float64(l.per) / l.n
	wait := time.Duration(math.Ceil((1 - b.tokens) * perToken))
	return false, wait
}

func (l *Limiter) refill(b *bucket, now time.Time) float64 {
	elapsed := now.Sub(b.at)
	if elapsed <= 0 {
		return b.tokens
	}
	return math.Min(l.n, b.tokens+l.n*float64(elapsed)/float64(l.per))
}

// sweep drops buckets that are full again: they hold no information.
func (l *Limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if l.refill(b, now) >= l.n {
			delete(l.buckets, k)
		}
	}
	l.lastSweep = now
}
