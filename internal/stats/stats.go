// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package stats

import (
	"sync"
	"time"
)

type Stats struct {
	// Handler
	HandlerStats HandlerStatsSnapshot `json:"handler_stats"`

	// Hub
	HubStats HubStats `json:"hub_stats"`

	// Performance & System
	AverageLatency   time.Duration `json:"average_latency"`
	ActiveGoroutines int           `json:"active_goroutines"`
	MemoryUsage      uint64        `json:"memory_usage"`
	Uptime           time.Duration `json:"uptime"`

	// Meta
	Timestamp time.Time `json:"timestamp"`
}

type HubStats struct {
	ActiveConnections    int                 `json:"active_connections"` // = TotalClients
	TotalRooms           int                 `json:"total_rooms"`
	RoomStats            map[string]RoomStat `json:"rooms"`
	DroppedMessages      uint64
	MessagesDroppedTotal int `json:"messages_dropped_total"`
	MessagesSentTotal    int `json:"messages_sent_total"`
	MessagesFailedTotal  int `json:"messages_failed_total"`
}

type RoomStat struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ClientCount int       `json:"client_count"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

type HandlerStatsSnapshot struct {
	StartTime           time.Time      `json:"start_time"`
	TotalConnections    uint64         `json:"total_connections"`
	ConnectionsPerIP    map[string]int `json:"connections_per_ip,omitempty"`
	ConnectionsPerSec   float64        `json:"connections_per_sec"`
	RejectedConnections uint64         `json:"rejected_connections"`
	MessagesSent        uint64         `json:"messages_sent"`
	MessagesReceived    uint64         `json:"messages_received"`
	MessagesPerSec      float64        `json:"messages_per_sec"`
	BytesSent           uint64         `json:"bytes_sent"`
	BytesReceived       uint64         `json:"bytes_received"`

	// Errors & Health
	RateLimitViolations uint64 `json:"rate_limit_violations"`
	AuthErrors          uint64 `json:"auth_errors"`
	ErrorCount          uint64 `json:"error_count"`
	LastError           error  `json:"last_error"`

	DisconnectedClients uint64        `json:"disconnected_clients"`
	LastStatsTime       time.Time     `json:"last_stats_time"`
	LastTotalConns      uint64        `json:"last_total_conns"`
	LastTotalMsgs       uint64        `json:"last_total_msgs"`
	LatencySum          time.Duration `json:"latency_sum"`
	LatencyCount        int64         `json:"latency_count"`
}

type HandlerStats struct {
	StartTime           time.Time
	TotalConnections    uint64
	ConnectionsPerIP    map[string]int
	ConnectionsPerSec   float64
	RejectedConnections uint64
	MessagesSent        uint64
	MessagesReceived    uint64
	MessagesPerSec      float64
	BytesSent           uint64
	BytesReceived       uint64

	// Errors & Health
	RateLimitViolations uint64
	AuthErrors          uint64
	ErrorCount          uint64
	LastError           error

	DisconnectedClients uint64
	LastStatsTime       time.Time
	LastTotalConns      uint64
	LastTotalMsgs       uint64
	LatencySum          time.Duration
	LatencyCount        int64

	mu sync.RWMutex
}

func (h *HandlerStats) IncrementTotalConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.TotalConnections++
}

func (h *HandlerStats) IncrementRejectedConnections() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.RejectedConnections++
}

func (h *HandlerStats) IncrementMessagesSent(bytes uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.MessagesSent++
	h.BytesSent += bytes
}

func (h *HandlerStats) IncrementMessagesReceived(bytes uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.MessagesReceived++
	h.BytesReceived += bytes
}

func (h *HandlerStats) IncrementErrors(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ErrorCount++
	h.LastError = err
}

func (h *HandlerStats) IncrementDisconnectedClients() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DisconnectedClients++
}

func (h *HandlerStats) IncrementAuthFailures() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.AuthErrors++
}

func (h *HandlerStats) CalculateConnectionsPerSec() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	if h.LastStatsTime.IsZero() {
		h.LastStatsTime = now
		h.LastTotalConns = h.TotalConnections
		return 0
	}

	elapsed := now.Sub(h.LastStatsTime).Seconds()
	if elapsed < 1 {
		return 0
	}

	rate := float64(h.TotalConnections-h.LastTotalConns) / elapsed

	// update to next calculation
	h.LastStatsTime = now
	h.LastTotalConns = h.TotalConnections

	return rate
}

func (h *HandlerStats) CalculateMessagesPerSec() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	if h.LastStatsTime.IsZero() {
		h.LastTotalMsgs = h.MessagesSent + h.MessagesReceived
		return 0
	}

	elapsed := now.Sub(h.LastStatsTime).Seconds()
	if elapsed < 1 {
		return 0
	}

	currentTotalMsgs := h.MessagesSent + h.MessagesReceived
	rate := float64(currentTotalMsgs-h.LastTotalMsgs) / elapsed
	h.LastTotalMsgs = currentTotalMsgs

	return rate

}

func (h *HandlerStats) CalculateAverageLatency() time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.LatencyCount == 0 {
		return 0
	}
	return h.LatencySum / time.Duration(h.LatencyCount)
}

func (h *HandlerStats) UpdateLatencyStats(latency time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.LatencySum += latency
	h.LatencyCount++
}

func (h *HandlerStats) Snapshot(
	connectionsPerIP map[string]int,
	rateLimitViolations uint64,
) HandlerStatsSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return HandlerStatsSnapshot{
		StartTime:           h.StartTime,
		TotalConnections:    h.TotalConnections,
		ConnectionsPerIP:    connectionsPerIP,
		ConnectionsPerSec:   h.CalculateConnectionsPerSec(),
		RejectedConnections: h.RejectedConnections,

		MessagesSent:     h.MessagesSent,
		MessagesReceived: h.MessagesReceived,
		MessagesPerSec:   h.CalculateMessagesPerSec(),
		BytesSent:        h.BytesSent,
		BytesReceived:    h.BytesReceived,

		RateLimitViolations: rateLimitViolations,
		AuthErrors:          h.AuthErrors,
		ErrorCount:          h.ErrorCount,
		LastError:           h.LastError,

		DisconnectedClients: h.DisconnectedClients,
		LastStatsTime:       h.LastStatsTime,
		LastTotalConns:      h.LastTotalConns,
		LastTotalMsgs:       h.LastTotalMsgs,
		LatencySum:          h.LatencySum,
		LatencyCount:        h.LatencyCount,
	}
}
