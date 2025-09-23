// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"errors"
	"sync"
)

// IClient is an interface for a client in a WebSocket application.
// It defines the methods that a client must implement to interact with the server.
type IClient interface {
	// Send sends a raw byte message to the server.
	Send(message *Message) error

	// SendTo sends a message to a specific client.
	SendTo(id string, message *Message) error

	// SendJSON sends JSON-encoded data to the server.
	SendJSON(data interface{}) error

	// SendProtobuf sends Protobuf-encoded data to the server.
	SendProtobuf(data interface{}) error

	// Send sends a raw byte message to the server.
	// The message is sent as-is, without any encoding or processing.
	SendRaw(message []byte) error

	BroadcastToRoom(room string, message *Message) error

	// JoinRoom joins a specific room on the server.
	// The client becomes a member of the specified room.
	JoinRoom(room string) error

	// LeaveRoom leaves a specific room on the server.
	// The client is removed from the specified room.
	LeaveRoom(room string) error

	// GetRooms retrieves a list of rooms the client is currently in.
	// The list of rooms is returned as a slice of strings.
	GetRooms() []string

	// Disconnect disconnects the client from the server.
	// The client's connection to the server is closed.
	Disconnect() error

	// IsConnected checks if the client is currently connected to the server.
	IsConnected() bool

	// SetUserData sets arbitrary user data associated with the client.
	// The data is stored on the client and can be retrieved later.
	SetUserData(key string, value interface{})

	// GetUserData retrieves arbitrary user data associated with the client.
	// The data is returned as an interface{} value.
	GetUserData(key string) interface{}
}

type Client struct {
	ID          string
	Conn        IWebSocketConn // WebSocket connection (gorilla/websocket.Conn)
	MessageChan chan *Message
	Hub         IHub
	UserData    map[string]interface{} // user custom data
	ConnInfo    *ConnectionInfo
	mu          sync.RWMutex
}

// NewClient creates a new Client instance.
//
// The id parameter should be a unique identifier for the client.
// The conn parameter should be a WebSocket connection.
// The hub parameter should be the Hub to which the client belongs.
//
// The created Client instance will have a message channel with a capacity of 256.
// The channel will receive messages from the underlying WebSocket connection.
//
// The created Client instance will also have a map to store user custom data.
// The map will be empty initially.
func NewClient(id string, conn IWebSocketConn, hub IHub, messageChanBufSize int) *Client {
	return &Client{
		ID:          id,
		Conn:        conn,
		Hub:         hub,
		MessageChan: make(chan *Message, messageChanBufSize),
		UserData:    make(map[string]interface{}),
		ConnInfo:    nil, // will be defined at HandleWebSocket
	}
}

// Send sends a message to the client. It will send the message if the message
// channel is not full. If the channel is full, it will return an error.
//
// This method is safe to call concurrently.
func (c *Client) Send(message *Message) error {
	return c.Hub.SendToClient(c.ID, c.ID, message)
}

// SendTo sends a message to a specific client.
func (c *Client) SendTo(id string, message *Message) error {
	return c.Hub.SendToClient(c.ID, id, message)
}

// SendJSON sends the given data to the client as JSON.
//
// It will marshal the given data to JSON and send it to the client.
// If the marshaling fails, an error will be returned.
//
// The given data must be a valid JSON structure.
//
// This method is safe to call concurrently.
func (c *Client) SendJSON(data interface{}) error {
	return c.Send(&Message{
		Data:     data,
		Type:     TextMessage,
		Encoding: JSON,
	})
}

// SendProtobuf sends the given data to the client as a Protobuf message.
//
// Currently, this is not implemented and will return an error.
//
// This method is safe to call concurrently.
//
// TODO: implement
func (c *Client) SendProtobuf(data interface{}) error {
	return errors.New("protobuf serialization not yet implemented")
}

// SendRaw sends the given raw data to the client.
//
// This method is safe to call concurrently.
func (c *Client) SendRaw(data []byte) error {
	return c.Send(&Message{
		RawData:  data,
		Type:     BinaryMessage,
		Encoding: Raw,
	})
}

// JoinRoom joins the given room. It will return an error if the client's hub is
// nil. Otherwise, it will call the hub's JoinRoom method with the client and room.
//
// This method is safe to call concurrently.
func (c *Client) JoinRoom(roomId string) error {
	if c.Hub == nil {
		return ErrHubIsNil
	}
	return c.Hub.JoinRoom(c, roomId)
}

// LeaveRoom leaves the given room. It will return an error if the client's hub is
// nil. Otherwise, it will call the hub's LeaveRoom method with the client and room.
//
// This method is safe to call concurrently.
func (c *Client) LeaveRoom(roomId string) error {
	if c.Hub == nil {
		return ErrHubIsNil
	}
	c.Hub.LeaveRoom(c, roomId)
	return nil
}

// GetRooms returns the rooms the client is currently in. If the client's hub is nil,
// it will return an empty slice. Otherwise, it will return the names of the rooms
// the client is in.
//
// This method is safe to call concurrently.
func (c *Client) GetRooms() []string {
	if c.Hub == nil {
		return []string{}
	}

	var rooms []string
	for _, room := range c.Hub.GetRooms() {
		if _, exists := room.GetClient(c.ID); exists {
			rooms = append(rooms, room.Name())
		}
	}
	return rooms
}

// Disconnect removes the client from its hub and closes its connection. It will
// return nil if the client is not connected to a hub or if the connection is nil.
// Otherwise, it will return the error from closing the connection.
//
// This method is safe to call concurrently.
func (c *Client) Disconnect() error {
	var err error

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Hub != nil {
		c.Hub.RemoveClient(c)
	}

	if c.Conn != nil {
		err = c.Conn.Close()
		c.Conn = nil
	}

	return err
}

func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn != nil
}

// SetUserData sets a value for a key in the client's user data map.
//
// This method is safe to call concurrently.
func (c *Client) SetUserData(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.UserData[key] = value
}

// GetUserData gets a value from the client's user data map by its key.
//
// This method is safe to call concurrently.
func (c *Client) GetUserData(key string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.UserData[key]
}
