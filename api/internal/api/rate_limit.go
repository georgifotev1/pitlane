package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimiters is the in-process per-IP token bucket store. Single-instance
// (ADR decision 18). Multi-instance upgrade path: Redis-backed limiter.
type rateLimiters struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	rate     rate.Limit
	burst    int
	ttl      time.Duration
}

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newRateLimiters(r rate.Limit, burst int) *rateLimiters {
	rl := &rateLimiters{
		limiters: make(map[string]*ipLimiter),
		rate:     r,
		burst:    burst,
		ttl:      10 * time.Minute,
	}
	go rl.gc()
	return rl
}

func (rl *rateLimiters) get(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	il, ok := rl.limiters[ip]
	if !ok {
		il = &ipLimiter{limiter: rate.NewLimiter(rl.rate, rl.burst)}
		rl.limiters[ip] = il
	}
	il.lastSeen = time.Now()
	return il.limiter
}

func (rl *rateLimiters) gc() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.ttl)
		for ip, il := range rl.limiters {
			if il.lastSeen.Before(cutoff) {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimit returns a middleware that applies the global per-IP rate limit
// (lenient, sized for SPA page-mount parallel queries — ADR decision 18).
func (s *Server) rateLimit(next http.Handler) http.Handler {
	rl := newRateLimiters(rate.Limit(20), 50)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, s.cfg.TrustedProxies)
		if !rl.get(ip).Allow() {
			s.rateLimitResponse(w, r, 1*time.Second)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// strictRateLimit applies a tighter limit to mutating auth endpoints to
// throttle credential stuffing (ADR decision 18).
func (s *Server) strictRateLimit(next http.Handler) http.Handler {
	rl := newRateLimiters(rate.Limit(0.083), 5) // 5/min, burst 5
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r, s.cfg.TrustedProxies)
		if !rl.get(ip).Allow() {
			s.rateLimitResponse(w, r, 60*time.Second)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimitResponse(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	w.Header().Set("Retry-After", retryAfter.String())
	renderProblem(w, r, http.StatusTooManyRequests, CodeRateLimited, "rate limit exceeded")
}

// clientIP extracts the client IP, honoring X-Forwarded-For only when the
// immediate hop is in the trusted-proxy list. When no trusted proxy is
// configured, we fall back to RemoteAddr — safe default.
func clientIP(r *http.Request, trustedProxies []string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(trustedProxies) == 0 {
		return host
	}
	if !isTrustedProxy(host, trustedProxies) {
		return host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	return host
}

func isTrustedProxy(host string, cidrs []string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		if strings.Contains(c, "/") {
			_, n, err := net.ParseCIDR(c)
			if err == nil && n.Contains(ip) {
				return true
			}
			continue
		}
		if c == host {
			return true
		}
	}
	return false
}
