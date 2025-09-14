// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"
)

var clientCounter atomic.Uint64

type UniversalOption func(HasHandler) error

type HasHandler interface {
	Handler() *Handler
}

type IWebSocketConn interface {
	Close() error
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
}

type ConnectionInfo struct {
	ClientIP  string
	UserAgent string
	Origin    string
	Headers   map[string]string
	RequestID string
}

func GenerateClientID() string {
	return fmt.Sprintf("client_%d_%d", time.Now().UnixNano(), clientCounter.Add(1))
}

func safeGoroutine(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("PANIC RECOVERED in %s: %v\nStack trace:\n%s\n",
					name, r, string(debug.Stack()))
			}
		}()
		fn()
	}()
}
