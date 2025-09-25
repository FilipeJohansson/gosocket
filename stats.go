package gosocket

import (
	"sync"
	"time"
)

type RoomStat struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	ClientCount int       `json:"client_count"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

type HandlerStats struct {
	startTime           time.Time
	totalConnections    uint64
	messagesSent        uint64
	messagesReceived    uint64
	bytesSent           uint64
	bytesReceived       uint64
	errorCount          uint64
	lastError           error
	disconnectedClients uint64
	lastStatsTime       time.Time
	lastTotalConns      uint64
	lastTotalMsgs       uint64
	latencySum          time.Duration
	latencyCount        int64
	mu                  sync.RWMutex
}

type Stats struct {
	// Connections
	ActiveConnections int            `json:"active_connections"` // = TotalClients
	TotalConnections  uint64         `json:"total_connections"`  // history
	ConnectionsPerIP  map[string]int `json:"connections_per_ip,omitempty"`
	ConnectionsPerSec float64        `json:"connections_per_sec"`

	// Messages
	MessagesSent     uint64  `json:"messages_sent"`
	MessagesReceived uint64  `json:"messages_received"`
	MessagesPerSec   float64 `json:"messages_per_sec"`
	BytesSent        uint64  `json:"bytes_sent"`
	BytesReceived    uint64  `json:"bytes_received"`

	// Rooms
	TotalRooms int                 `json:"total_rooms"`
	RoomStats  map[string]RoomStat `json:"rooms"`

	// Performance & System
	AverageLatency   time.Duration `json:"average_latency"`
	ActiveGoroutines int           `json:"active_goroutines"`
	MemoryUsage      uint64        `json:"memory_usage"`
	Uptime           time.Duration `json:"uptime"`

	// Errors & Health
	ErrorCount          uint64 `json:"error_count"`
	LastError           error  `json:"last_error,omitempty"`
	DisconnectedClients uint64 `json:"disconnected_clients"`
	RateLimitViolations uint64 `json:"rate_limit_violations"`

	// Meta
	Timestamp time.Time `json:"timestamp"`
}
