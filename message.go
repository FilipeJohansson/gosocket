package gosocket

import "time"

// generic WebSocket message
type Message struct {
	Type     MessageType  `json:"type"`
	Data     interface{}  `json:"data"`     // original data (struct, map, etc)
	RawData  []byte       `json:"raw_data"` // serialized data
	Encoding EncodingType `json:"encoding"` // used encoding type
	From     string       `json:"from,omitempty"`
	To       string       `json:"to,omitempty"`
	Room     string       `json:"room,omitempty"`
	Created  time.Time    `json:"created"`
}

type MessageType int

const (
	TextMessage MessageType = iota
	BinaryMessage
	PingMessage
	PongMessage
	CloseMessage
)

func NewMessage(msgType MessageType, data interface{}) *Message {
	return &Message{
		Type:    msgType,
		Data:    data,
		Created: time.Now(),
	}
}

func NewMessageWithEncoding(msgType MessageType, data interface{}, encoding EncodingType) *Message {
	return &Message{
		Type:     msgType,
		Data:     data,
		Encoding: encoding,
		Created:  time.Now(),
	}
}

func NewRawMessage(msgType MessageType, rawData []byte) *Message {
	return &Message{
		Type:     msgType,
		RawData:  rawData,
		Encoding: Raw,
		Created:  time.Now(),
	}
}
