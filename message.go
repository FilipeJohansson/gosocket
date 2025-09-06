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

// NewMessage creates a new Message with the given MessageType and data.
//
// The data must be a valid JSON structure, and will be serialized to a byte slice.
// If the data is not a valid JSON structure, an error will be returned.
//
// The Encoding field of the Message will be set to JSON.
func NewMessage(msgType MessageType, data interface{}) *Message {
	return &Message{
		Type:    msgType,
		Data:    data,
		Created: time.Now(),
	}
}

// NewMessageWithEncoding creates a new Message with the given MessageType, data, and Encoding type.
//
// The data must be a valid JSON structure, and will be serialized to a byte slice.
// If the data is not a valid JSON structure, an error will be returned.
//
// The Encoding field of the Message will be set to the provided Encoding type.
func NewMessageWithEncoding(msgType MessageType, data interface{}, encoding EncodingType) *Message {
	return &Message{
		Type:     msgType,
		Data:     data,
		Encoding: encoding,
		Created:  time.Now(),
	}
}

// NewRawMessage creates a new Message with the given MessageType and raw data.
//
// The RawData field of the Message will be set to the provided raw data.
// The Encoding field of the Message will be set to Raw.
func NewRawMessage(msgType MessageType, rawData []byte) *Message {
	return &Message{
		Type:     msgType,
		RawData:  rawData,
		Encoding: Raw,
		Created:  time.Now(),
	}
}
