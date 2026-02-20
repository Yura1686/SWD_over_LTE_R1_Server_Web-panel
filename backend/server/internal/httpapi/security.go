package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateBucket struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

type ipRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]*rateBucket
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &ipRateLimiter{
		limit:   limit,
		window:  window,
		entries: make(map[string]*rateBucket),
	}
}

func (l *ipRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok {
		l.entries[key] = &rateBucket{
			windowStart: now,
			count:       1,
			lastSeen:    now,
		}
		l.cleanupLocked(now)
		return true
	}

	if now.Sub(entry.windowStart) >= l.window {
		entry.windowStart = now
		entry.count = 0
	}
	entry.lastSeen = now

	if entry.count >= l.limit {
		l.cleanupLocked(now)
		return false
	}

	entry.count++
	l.cleanupLocked(now)
	return true
}

func (l *ipRateLimiter) cleanupLocked(now time.Time) {
	if len(l.entries) <= 128 {
		return
	}
	for key, entry := range l.entries {
		if now.Sub(entry.lastSeen) > (l.window * 3) {
			delete(l.entries, key)
		}
	}
}

type loginGuardRecord struct {
	consecutive int
	blockedTill time.Time
	lastSeen    time.Time
}

type loginGuard struct {
	mu          sync.Mutex
	burst       int
	blockFor    time.Duration
	perIPRate   *ipRateLimiter
	perIPStatus map[string]*loginGuardRecord
}

func newLoginGuard(ratePerMinute int, burst int) *loginGuard {
	if burst <= 0 {
		burst = 5
	}
	return &loginGuard{
		burst:       burst,
		blockFor:    60 * time.Second,
		perIPRate:   newIPRateLimiter(ratePerMinute, time.Minute),
		perIPStatus: make(map[string]*loginGuardRecord),
	}
}

func (g *loginGuard) allow(ip string, now time.Time) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()

	status, ok := g.perIPStatus[ip]
	if ok && now.Before(status.blockedTill) {
		return false, status.blockedTill.Sub(now)
	}
	if !g.perIPRate.allow(ip, now) {
		return false, time.Minute
	}

	g.cleanupLocked(now)
	return true, 0
}

func (g *loginGuard) onFailure(ip string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()

	status, ok := g.perIPStatus[ip]
	if !ok {
		status = &loginGuardRecord{}
		g.perIPStatus[ip] = status
	}

	status.lastSeen = now
	status.consecutive++
	if status.consecutive >= g.burst {
		status.blockedTill = now.Add(g.blockFor)
		status.consecutive = 0
	}

	g.cleanupLocked(now)
}

func (g *loginGuard) onSuccess(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	status, ok := g.perIPStatus[ip]
	if !ok {
		return
	}
	status.consecutive = 0
}

func (g *loginGuard) cleanupLocked(now time.Time) {
	if len(g.perIPStatus) <= 128 {
		return
	}
	for key, value := range g.perIPStatus {
		if now.Sub(value.lastSeen) > (2 * time.Hour) {
			delete(g.perIPStatus, key)
		}
	}
}

func requestIP(r *http.Request, trustProxyHeaders bool) string {
	if trustProxyHeaders {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if first != "" {
				return first
			}
		}

		xri := strings.TrimSpace(r.Header.Get("X-Real-IP"))
		if xri != "" {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
