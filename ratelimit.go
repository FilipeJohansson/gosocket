package gosocket

import (
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter interface {
	AllowClient(clientId string) bool

	AllowIP(ip string) bool

	AllowNetAddr(addr net.Addr) bool

	Stop()
}

type RateLimiterManager struct {
	config RateLimiterConfig

	clientsMu sync.Mutex
	clients   map[string]*limiterEntry

	ipsMu sync.Mutex
	ips   map[string]*limiterEntry

	quit chan struct{}
}

func NewRateLimiterManager(config RateLimiterConfig) *RateLimiterManager {
	rl := &RateLimiterManager{
		config:  config,
		clients: make(map[string]*limiterEntry),
		ips:     make(map[string]*limiterEntry),
		quit:    make(chan struct{}),
	}

	go rl.cleanupLoop()

	return rl
}

// AllowClient returns true if client with given id is allowed (token available).
// Should be called for each incoming message from the client.
func (r *RateLimiterManager) AllowClient(clientID string) bool {
	if clientID == "" {
		// treat empty as limited: create ephemeral key per-empty
		clientID = "__empty__"
	}

	r.clientsMu.Lock()
	entry, ok := r.clients[clientID]
	if !ok {
		entry = &limiterEntry{
			limiter:  rate.NewLimiter(rate.Limit(r.config.PerClientRate), r.config.PerClientBurst),
			lastSeen: time.Now(),
		}
		r.clients[clientID] = entry
	}
	entry.lastSeen = time.Now()
	lim := entry.limiter
	r.clientsMu.Unlock()

	return lim.Allow()
}

// AllowIP returns true if IP is allowed (token available).
// ip should be a canonical string (e.g., net.IP.String()).
func (r *RateLimiterManager) AllowIP(ip string) bool {
	if ip == "" {
		ip = "__unknown_ip__"
	}

	r.ipsMu.Lock()
	entry, ok := r.ips[ip]
	if !ok {
		entry = &limiterEntry{
			limiter:  rate.NewLimiter(rate.Limit(r.config.PerIPRate), r.config.PerIPBurst),
			lastSeen: time.Now(),
		}
		r.ips[ip] = entry
	}
	entry.lastSeen = time.Now()
	lim := entry.limiter
	r.ipsMu.Unlock()

	return lim.Allow()
}

// AllowNetAddr is helper: extracts IP from net.Addr (e.g. ws conn.RemoteAddr())
func (r *RateLimiterManager) AllowNetAddr(addr net.Addr) bool {
	if addr == nil {
		return r.AllowIP("")
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		// fallback to raw string
		return r.AllowIP(addr.String())
	}
	return r.AllowIP(host)
}

func (r *RateLimiterManager) Stop() {
	close(r.quit)
}

func (r *RateLimiterManager) cleanupLoop() {
	t := time.NewTicker(r.config.CleanupInterval)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			r.cleanup()
		case <-r.quit:
			return
		}
	}
}

func (r *RateLimiterManager) cleanup() {
	now := time.Now()
	threshold := now.Add(-r.config.EntryTTL)

	r.clientsMu.Lock()
	for k, v := range r.clients {
		if v.lastSeen.Before(threshold) {
			delete(r.clients, k)
		}
	}
	r.clientsMu.Unlock()

	r.ipsMu.Lock()
	for k, v := range r.ips {
		if v.lastSeen.Before(threshold) {
			delete(r.ips, k)
		}
	}
	r.ipsMu.Unlock()
}
