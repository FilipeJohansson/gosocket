// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package hub

import (
	"sync"
	"sync/atomic"

	"github.com/FilipeJohansson/gosocket/internal/message"
)

type WSConn interface {
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

type Client struct {
	id       string
	Conn     WSConn // WebSocket connection
	SendChan chan *message.Message

	userData map[string]interface{} // user custom data
	connInfo *ConnectionInfo

	mu     sync.RWMutex
	closed atomic.Bool
}

// NewClient creates a new Client instance.
//
// The id parameter should be a unique identifier for the client.
// The conn parameter should be a WebSocket connection.
// The connInfo parameter should be a ConnectionInfo struct.
// The sendChanBufSize parameter should be the capacity of the message channel.
//
// The send channel will receive messages from the underlying WebSocket connection.
//
// The created Client instance will also have a map to store user custom data.
// The map will be empty initially.
func NewClient(id string, conn WSConn, connInfo *ConnectionInfo, sendChanBufSize int) *Client {
	return &Client{
		id:       id,
		Conn:     conn,
		SendChan: make(chan *message.Message, sendChanBufSize),
		userData: make(map[string]interface{}),
		connInfo: connInfo,
	}
}

// GetID returns the client's ID.
func (c *Client) GetID() string {
	return c.id
}

// GetUserData returns the client's user data map.
//
// This method is safe to call concurrently.
func (c *Client) GetUserData() interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userData
}

// GetUserData gets a value from the client's user data map by its key.
//
// This method is safe to call concurrently.
func (c *Client) GetUserDataByKey(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userData[key]
}

// SetUserData sets a value for a key in the client's user data map.
//
// This method is safe to call concurrently.
func (c *Client) SetUserData(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userData[key] = value
}

// RemoveUserData removes a key from the client's user data map.
//
// This method is safe to call concurrently.
func (c *Client) RemoveUserData(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.userData, key)
}
