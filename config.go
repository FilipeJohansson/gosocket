// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"net/http"
	"time"
)

type SerializationConfig struct {
	MaxDepth        int
	MaxKeys         int
	MaxElements     int
	MaxStringLength int
	MaxBinarySize   int64
	DisallowedTypes []string
	EnableStrict    bool
}

type HandlerConfig struct {
	MaxConnections     int
	MessageSize        int64
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	PingPeriod         time.Duration
	PongWait           time.Duration
	AllowedOrigins     []string
	DefaultEncoding    EncodingType // default message encoding
	Serialization      SerializationConfig
	MessageChanBufSize int
}

type ServerConfig struct {
	Port       int
	Path       string
	EnableCORS bool
	EnableSSL  bool
	CertFile   string
	KeyFile    string
}

type Middleware func(http.Handler) http.Handler
type AuthFunc func(*http.Request) (map[string]interface{}, error)

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:       8080,
		Path:       "/ws",
		EnableCORS: true,
		EnableSSL:  false,
	}
}

func DefaultHandlerConfig() *HandlerConfig {
	return &HandlerConfig{
		MaxConnections:     1000,
		MessageSize:        512 * 1024,
		ReadTimeout:        60 * time.Second,
		WriteTimeout:       10 * time.Second,
		PingPeriod:         54 * time.Second,
		PongWait:           60 * time.Second,
		DefaultEncoding:    JSON,
		Serialization:      DefaultSerializerConfig(),
		MessageChanBufSize: 256,
	}
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
