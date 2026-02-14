// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package transport

import (
	"net"
	"sync"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/utils"
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

type RateLimiterConfig struct {
	// PerClientRate is requests/msgs per second allowed per client.
	PerClientRate float64
	// PerClientBurst is burst size for client limiter.
	PerClientBurst int

	// PerIPRate is requests/msgs per second allowed per IP (handshake / connection attempts / overall).
	PerIPRate float64
	// PerIPBurst is burst size for IP limiter.
	PerIPBurst int

	// CleanupInterval controls how often internal stale entries are purged.
	CleanupInterval time.Duration
	// EntryTTL is how long an unused entry stays before eligible for cleanup.
	EntryTTL time.Duration

	// MaxRateLimitViolations is the maximum number of times a client can be rate limited before being disconnected.
	MaxRateLimitViolations int
}

func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		PerClientRate:          10, // 10 messages/s per client
		PerClientBurst:         100,
		PerIPRate:              20, // 20 reqs/s per IP (handshake/connection attempts)
		PerIPBurst:             40,
		CleanupInterval:        30 * time.Second,
		EntryTTL:               5 * time.Minute,
		MaxRateLimitViolations: 5,
	}
}

type RateLimiterManager struct {
	config *RateLimiterConfig

	clientsMu sync.Mutex
	clients   map[string]*limiterEntry

	ipsMu sync.Mutex
	ips   map[string]*limiterEntry

	violationsMu      sync.Mutex
	clientsViolations uint64
	ipsViolations     uint64

	quit chan struct{}
}

func NewRateLimiterManager(config *RateLimiterConfig) *RateLimiterManager {
	if config == nil {
		config = DefaultRateLimiterConfig()
	}

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

	if !lim.Allow() {
		r.violationsMu.Lock()
		r.clientsViolations += 1
		r.violationsMu.Unlock()
		return false
	}

	return true
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

	if !lim.Allow() {
		r.violationsMu.Lock()
		r.ipsViolations += 1
		r.violationsMu.Unlock()
		return false
	}

	return true
}

// AllowNetAddr is helper: extracts IP from net.Addr (e.g. ws conn.RemoteAddr())
func (r *RateLimiterManager) AllowNetAddr(addr net.Addr) bool {
	return r.AllowIP(utils.ExtractIP(addr))
}

func (r *RateLimiterManager) Stop() {
	close(r.quit)
}

func (r *RateLimiterManager) GetViolations() uint64 {
	r.violationsMu.Lock()
	defer r.violationsMu.Unlock()
	return r.clientsViolations + r.ipsViolations
}

func (r *RateLimiterManager) MaxRateLimitViolations() int {
	return r.config.MaxRateLimitViolations
}

func (r *RateLimiterManager) Config() *RateLimiterConfig {
	return r.config
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
