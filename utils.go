// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"fmt"
	"time"
)

type IWebSocketConn interface {
	Close() error
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
}

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
