// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"encoding/json"
	"fmt"
	"sync"
)

// IClient is an interface for a client in a WebSocket application.
// It defines the methods that a client must implement to interact with the server.
type IClient interface {
	// Send sends a raw byte message to the server.
	// The message is sent as-is, without any encoding or processing.
	Send(message []byte) error

	// SendMessage sends a message to the server, with the option to specify encoding.
	// The message is sent with the specified encoding, if any.
	SendMessage(message *Message) error

	// SendData sends arbitrary data to the server, automatically encoding it as JSON.
	// The data is marshaled to JSON and sent to the server.
	SendData(data interface{}) error

	// SendDataWithEncoding sends arbitrary data to the server with a specified encoding.
	// The data is marshaled to the specified encoding and sent to the server.
	SendDataWithEncoding(data interface{}, encoding EncodingType) error

	// SendJSON sends JSON-encoded data to the server.
	// The data is marshaled to JSON and sent to the server.
	SendJSON(data interface{}) error

	// SendProtobuf sends Protobuf-encoded data to the server.
	// The data is marshaled to Protobuf and sent to the server.
	SendProtobuf(data interface{}) error

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
	MessageChan chan []byte
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
func NewClient(id string, conn IWebSocketConn, hub IHub) *Client {
	return &Client{
		ID:          id,
		Conn:        conn,
		Hub:         hub,
		MessageChan: make(chan []byte, 256),
		UserData:    make(map[string]interface{}),
		ConnInfo:    nil, // will be defined at HandleWebSocket
	}
}

// Send sends a message to the client. It will send the message if the message
// channel is not full. If the channel is full, it will return an error.
//
// This method is safe to call concurrently.
func (c *Client) Send(message []byte) error {
	if c.Conn == nil {
		return fmt.Errorf("client connection is nil")
	}

	select {
	case c.MessageChan <- message:
		return nil
	default:
		return fmt.Errorf("client message channel is full")
	}
}

// SendMessage sends a message to the client.
//
// If the message has a RawData field that is not nil, it will be sent directly.
// Otherwise, the Data field will be sent with the Encoding specified in the
// message. If the Encoding field is 0, it will be assumed to be JSON.
//
// The SendJSON and Send methods will be used to send the message.
//
// If the message has no data to send, an error will be returned.
//
// This method is safe to call concurrently.
func (c *Client) SendMessage(message *Message) error {
	if message.RawData != nil {
		return c.Send(message.RawData)
	}

	if message.Data != nil {
		// JSON fallback
		if message.Encoding == 0 {
			message.Encoding = JSON
		}

		switch message.Encoding {
		case JSON:
			return c.SendJSON(message.Data)
		case Raw:
			if rawData, ok := message.Data.([]byte); ok {
				return c.Send(rawData)
			}
			return fmt.Errorf("raw encoding expects []byte data")
		default:
			return fmt.Errorf("unsupported encoding: %d", message.Encoding)
		}
	}

	return fmt.Errorf("message has no data to send")
}

// SendData sends the given data to the client. It will be sent as JSON if no encoding type is specified.
//
// This method is safe to call concurrently.
func (c *Client) SendData(data interface{}) error {
	return c.SendJSON(data) // JSON by default
}

// SendDataWithEncoding sends the given data to the client using the specified
// encoding type.
//
// If the encoding type is JSON, it will be sent as JSON.
// If the encoding type is Raw, it will be sent directly as a byte slice. If the
// data is not a byte slice, an error will be returned.
//
// If the encoding type is not supported, an error will be returned.
//
// This method is safe to call concurrently.
func (c *Client) SendDataWithEncoding(data interface{}, encoding EncodingType) error {
	switch encoding {
	case JSON:
		return c.SendJSON(data)
	case Raw:
		if rawData, ok := data.([]byte); ok {
			return c.Send(rawData)
		}
		return fmt.Errorf("raw encoding expects []byte data")
	default:
		return fmt.Errorf("unsupported encoding: %d", encoding)
	}
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
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return c.Send(jsonData)
}

// SendProtobuf sends the given data to the client as a Protobuf message.
//
// Currently, this is not implemented and will return an error.
//
// This method is safe to call concurrently.
//
// TODO: implement
func (c *Client) SendProtobuf(data interface{}) error {
	return fmt.Errorf("protobuf serialization not yet implemented")
}

// JoinRoom joins the given room. It will return an error if the client's hub is
// nil. Otherwise, it will call the hub's JoinRoom method with the client and room.
//
// This method is safe to call concurrently.
func (c *Client) JoinRoom(room string) error {
	if c.Hub == nil {
		return fmt.Errorf("client hub is nil")
	}
	c.Hub.JoinRoom(c, room)
	return nil
}

// LeaveRoom leaves the given room. It will return an error if the client's hub is
// nil. Otherwise, it will call the hub's LeaveRoom method with the client and room.
//
// This method is safe to call concurrently.
func (c *Client) LeaveRoom(room string) error {
	if c.Hub == nil {
		return fmt.Errorf("client hub is nil")
	}
	c.Hub.LeaveRoom(c, room)
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
	for roomName, clients := range c.Hub.GetRooms() {
		if _, exists := clients[c]; exists {
			rooms = append(rooms, roomName)
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
	if c.Hub != nil {
		c.Hub.RemoveClient(c)
	}

	if c.Conn != nil {
		return c.Conn.Close()
	}
	return nil
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
