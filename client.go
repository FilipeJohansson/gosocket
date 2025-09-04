package gosocket

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

type Client struct {
	ID          string
	Conn        interface{} // WebSocket connection (gorilla/websocket.Conn)
	MessageChan chan []byte
	Hub         *Hub
	UserData    map[string]interface{} // user custom data
}

func NewClient(id string, conn interface{}, hub *Hub) *Client {
	return &Client{
		ID:          id,
		Conn:        conn,
		Hub:         hub,
		MessageChan: make(chan []byte, 256),
		UserData:    make(map[string]interface{}),
	}
}

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

func (c *Client) SendData(data interface{}) error {
	return c.SendJSON(data) // JSON by default
}

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

func (c *Client) SendJSON(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return c.Send(jsonData)
}

func (c *Client) SendProtobuf(data interface{}) error {
	return fmt.Errorf("protobuf serialization not yet implemented")
}

func (c *Client) JoinRoom(room string) error {
	if c.Hub == nil {
		return fmt.Errorf("client hub is nil")
	}
	c.Hub.JoinRoom(c, room)
	return nil
}

func (c *Client) LeaveRoom(room string) error {
	if c.Hub == nil {
		return fmt.Errorf("client hub is nil")
	}
	c.Hub.LeaveRoom(c, room)
	return nil
}

func (c *Client) GetRooms() []string {
	if c.Hub == nil {
		return []string{}
	}

	var rooms []string
	c.Hub.mu.RLock()
	for roomName, clients := range c.Hub.Rooms {
		if _, exists := clients[c]; exists {
			rooms = append(rooms, roomName)
		}
	}
	c.Hub.mu.RUnlock()
	return rooms
}

func (c *Client) Disconnect() error {
	if c.Hub != nil {
		c.Hub.RemoveClient(c)
	}

	if conn, ok := c.Conn.(*websocket.Conn); ok {
		return conn.Close()
	}
	return nil
}

func (c *Client) SetUserData(key string, value interface{}) {
	c.UserData[key] = value
}

func (c *Client) GetUserData(key string) interface{} {
	return c.UserData[key]
}
