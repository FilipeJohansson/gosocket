// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"time"
)

type LoggerConfig struct {
	Logger Logger
	Level  map[LogType]LogLevel
}

func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Logger: &NullLogger{},
		Level:  make(map[LogType]LogLevel),
	}
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

func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		PerClientRate:          10, // 10 messages/s per client
		PerClientBurst:         100,
		PerIPRate:              20, // 20 reqs/s per IP (handshake/connection attempts)
		PerIPBurst:             40,
		CleanupInterval:        30 * time.Second,
		EntryTTL:               5 * time.Minute,
		MaxRateLimitViolations: 5,
	}
}

type SerializationConfig struct {
	MaxDepth        int
	MaxKeys         int
	MaxElements     int
	MaxStringLength int
	MaxBinarySize   int64
	DisallowedTypes []string
	EnableStrict    bool
}

func DefaultSerializerConfig() SerializationConfig {
	return SerializationConfig{
		MaxDepth:        10,
		MaxKeys:         100,
		MaxElements:     1000,
		DisallowedTypes: []string{"func", "chan", "unsafe.Pointer"},
		EnableStrict:    true,
		MaxStringLength: 1024 * 1024,
		MaxBinarySize:   10 * 1024 * 1024,
	}
}

type HandlerConfig struct {
	MaxConnections      int
	MaxConnectionsPerIP int
	MessageChanBufSize  int
	MessageSize         int64
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	PingPeriod          time.Duration
	PongWait            time.Duration
	ConnectionTimeout   time.Duration
	AllowedOrigins      []string
	RelevantHeaders     []string
	DefaultEncoding     EncodingType // default message encoding
	Serialization       SerializationConfig
}

func DefaultHandlerConfig() *HandlerConfig {
	return &HandlerConfig{
		MaxConnections:      1000,
		MaxConnectionsPerIP: 10,
		MessageSize:         512 * 1024,
		ReadTimeout:         60 * time.Second,
		WriteTimeout:        10 * time.Second,
		PingPeriod:          54 * time.Second,
		PongWait:            60 * time.Second,
		ConnectionTimeout:   300 * time.Second,
		DefaultEncoding:     JSON,
		Serialization:       DefaultSerializerConfig(),
		MessageChanBufSize:  256,
	}
}

type ServerConfig struct {
	Port       int
	Path       string
	EnableCORS bool
	EnableSSL  bool
	CertFile   string
	KeyFile    string
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:       8080,
		Path:       "/ws",
		EnableCORS: true,
		EnableSSL:  false,
	}
}
