// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/FilipeJohansson/gosocket/internal/transport/websocket"
)

func DefaultLoggerConfig() (logger.Logger, map[logger.LogType]logger.LogLevel) {
	return logger.DefaultLoggerConfig()
}

func DefaultRateLimiterConfig() *transport.RateLimiterConfig {
	return transport.DefaultRateLimiterConfig()
}

func DefaultSerializerConfig() message.SerializationConfig {
	return message.DefaultSerializerConfig()
}

func DefaultHandlerConfig() *websocket.HandlerConfig {
	return websocket.DefaultHandlerConfig()
}

func DefaultServerConfig() *websocket.ServerConfig {
	return websocket.DefaultServerConfig()
}

// DefaultHubConfig returns a minimal HubConfig with a default logger.
func DefaultHubConfig() *hub.HubConfig {
	return hub.DefaultHubConfig()
}
