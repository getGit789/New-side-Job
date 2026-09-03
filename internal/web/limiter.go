package web

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiter is a per-IP token bucket. ponytail: in-memory, so limits are per process; fine for one binary.
type ipLimiter struct {
	mu    sync.Mutex
	rate  rate.Limit
	burst int
	seen  map[string]*entry
}

type entry struct {
	lim  *rate.Limiter
	last time.Time
}

func newIPLimiter(r rate.Limit, burst int) *ipLimiter {
	l := &ipLimiter{rate: r, burst: burst, seen: map[string]*entry{}}
	go func() {
		for range time.Tick(5 * time.Minute) {
			l.mu.Lock()
			for ip, e := range l.seen {
				if time.Since(e.last) > 10*time.Minute {
					delete(l.seen, ip)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.seen[ip]
	if !ok {
		e = &entry{lim: rate.NewLimiter(l.rate, l.burst)}
		l.seen[ip] = e
	}
	e.last = time.Now()
	return e.lim.Allow()
}
