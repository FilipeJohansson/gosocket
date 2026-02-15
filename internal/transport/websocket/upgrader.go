// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
)

type UpgraderConfig struct {
	ReadBufferSize  int
	WriteBufferSize int
	CheckOrigin     func(r *http.Request) bool
	Subprotocols    []string
}

func DefaultUpgraderConfig() *UpgraderConfig {
	return &UpgraderConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}

func NewUpgrader(cfg *UpgraderConfig) websocket.Upgrader {
	if cfg == nil {
		cfg = DefaultUpgraderConfig()
	}

	return websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		CheckOrigin:     cfg.CheckOrigin,
		Subprotocols:    cfg.Subprotocols,
	}
}
