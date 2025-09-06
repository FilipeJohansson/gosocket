// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"encoding/json"
	"fmt"
)

// Serializer defines an interface for serializing and deserializing data.
type Serializer interface {
	// Marshal converts a Go value into a byte slice representation.
	//
	// The provided value is marshaled into a byte slice, which can be written to a stream or stored in a buffer.
	// Any error that occurs during marshaling is returned.
	Marshal(v interface{}) ([]byte, error)

	// Unmarshal converts a byte slice into a Go value.
	//
	// The provided byte slice is unmarshaled into the provided value.
	// Any error that occurs during unmarshaling is returned.
	Unmarshal(data []byte, v interface{}) error

	// ContentType returns the content type associated with the serialized data.
	//
	// The returned content type is a string that identifies the format of the serialized data.
	ContentType() string

	// EncodingType returns the type of encoding used by the serializer.
	//
	// The returned encoding type is an EncodingType value that identifies the encoding scheme used by the serializer.
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
