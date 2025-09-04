package gosocket

import (
	"encoding/json"
	"fmt"
)

type Serializer interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
	ContentType() string
	EncodingType() EncodingType
}

type EncodingType int

const (
	JSON EncodingType = iota
	Protobuf
	MessagePack
	CBOR
	Raw // to raw binary data
)

type JSONSerializer struct{}

func (j JSONSerializer) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (j JSONSerializer) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (j JSONSerializer) ContentType() string {
	return "application/json"
}

func (j JSONSerializer) EncodingType() EncodingType {
	return JSON
}

// ProtobufSerializer default Protobuf implementation
type ProtobufSerializer struct{}

func (p ProtobufSerializer) Marshal(v interface{}) ([]byte, error) {
	// implement proto.Marshal
	return nil, nil
}

func (p ProtobufSerializer) Unmarshal(data []byte, v interface{}) error {
	// implement proto.Unmarshal
	return nil
}

func (p ProtobufSerializer) ContentType() string {
	return "application/x-protobuf"
}

func (p ProtobufSerializer) EncodingType() EncodingType {
	return Protobuf
}

// RawSerializer to raw binary data
type RawSerializer struct{}

func (r RawSerializer) Marshal(v interface{}) ([]byte, error) {
	if data, ok := v.([]byte); ok {
		return data, nil
	}
	return nil, fmt.Errorf("raw serializer expects []byte")
}

func (r RawSerializer) Unmarshal(data []byte, v interface{}) error {
	if ptr, ok := v.(*[]byte); ok {
		*ptr = data
		return nil
	}
	return fmt.Errorf("raw serializer expects *[]byte")
}

func (r RawSerializer) ContentType() string {
	return "application/octet-stream"
}

func (r RawSerializer) EncodingType() EncodingType {
	return Raw
}
