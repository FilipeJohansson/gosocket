// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package message

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/FilipeJohansson/gosocket/internal/errors"
)

type SerializationConfig struct {
	MaxDepth        int
	MaxKeys         int
	MaxElements     int
	MaxStringLength int
	MaxBinarySize   int64
	DisallowedTypes []string
	EnableStrict    bool
}

func DefaultSerializerConfig() SerializationConfig {
	return SerializationConfig{
		MaxDepth:        10,
		MaxKeys:         100,
		MaxElements:     1000,
		DisallowedTypes: []string{"func", "chan", "unsafe.Pointer"},
		EnableStrict:    true,
		MaxStringLength: 1024 * 1024,
		MaxBinarySize:   10 * 1024 * 1024,
	}
}

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

	Configure(config SerializationConfig)

	ValidateValue(v interface{}, depth int) error

	ValidateType(t reflect.Type, depth int) error
}

type EncodingType int

const (
	JSON EncodingType = iota
	Raw               // to raw binary data
)

func CreateSerializer(encoding EncodingType, config SerializationConfig) Serializer {
	switch encoding {
	case JSON:
		return NewJSONSerializer(config)
	case Raw:
		return NewRawSerializer(config)
	default:
		return NewJSONSerializer(config) // Fallback
	}
}

type BaseSerializer struct {
	Config SerializationConfig
}

func NewBaseSerializer(config SerializationConfig) BaseSerializer {
	return BaseSerializer{Config: config}
}

func (b *BaseSerializer) Configure(config SerializationConfig) {
	b.Config = config
}

func (b *BaseSerializer) ValidateValue(v interface{}, depth int) error {
	if depth > b.Config.MaxDepth {
		return errors.NewMaxDepthExceededError(b.Config.MaxDepth)
	}

	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return nil
	}

	switch val.Kind() {
	case reflect.Map:
		if val.Len() > b.Config.MaxKeys {
			return errors.NewMaxKeyLengthExceededError(val.Len())
		}

		for _, key := range val.MapKeys() {
			if err := b.ValidateValue(key.Interface(), depth+1); err != nil {
				return err
			}
			if err := b.ValidateValue(val.MapIndex(key).Interface(), depth+1); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		if val.Len() > b.Config.MaxElements {
			return errors.NewMaxElementsExceededError(val.Len())
		}

		for i := 0; i < val.Len(); i++ {
			if err := b.ValidateValue(val.Index(i).Interface(), depth+1); err != nil {
				return err
			}
		}

	case reflect.String:
		if val.Len() > b.Config.MaxStringLength {
			return errors.NewMaxStringLengthExceededError(val.Len())
		}

	case reflect.Pointer, reflect.Interface:
		if !val.IsNil() {
			return b.ValidateValue(val.Elem().Interface(), depth+1)
		}

	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.CanInterface() {
				if err := b.ValidateValue(field.Interface(), depth+1); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (b *BaseSerializer) ValidateType(t reflect.Type, depth int) error {
	if depth > b.Config.MaxDepth {
		return errors.NewMaxDepthExceededError(b.Config.MaxDepth)
	}

	if t == nil {
		return nil
	}

	typeName := t.String()
	for _, disallowed := range b.Config.DisallowedTypes {
		if strings.Contains(typeName, disallowed) {
			return errors.NewTypeNotAllowedError(typeName)
		}
	}

	switch t.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return errors.NewTypeNotAllowedError(t.Kind().String())
	case reflect.Pointer:
		return b.ValidateType(t.Elem(), depth+1)
	case reflect.Array, reflect.Slice:
		return b.ValidateType(t.Elem(), depth+1)
	case reflect.Map:
		if err := b.ValidateType(t.Key(), depth+1); err != nil {
			return err
		}
		return b.ValidateType(t.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if err := b.ValidateType(t.Field(i).Type, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

type JSONSerializer struct {
	BaseSerializer
}

func NewJSONSerializer(config SerializationConfig) *JSONSerializer {
	return &JSONSerializer{
		BaseSerializer: NewBaseSerializer(config),
	}
}

func (j *JSONSerializer) Marshal(v interface{}) ([]byte, error) {
	if err := j.ValidateType(reflect.TypeOf(v), 0); err != nil {
		return nil, errors.NewTypeNotAllowedError(err.Error())
	}

	if err := j.ValidateValue(v, 0); err != nil {
		return nil, errors.NewInvalidValueError(err.Error())
	}

	return json.Marshal(v)
}

func (j *JSONSerializer) Unmarshal(data []byte, v interface{}) error {
	if len(data) == 0 {
		return errors.ErrEmptyData
	}

	if int64(len(data)) > j.Config.MaxBinarySize {
		return errors.NewDataTooLongError(len(data))
	}

	if err := j.ValidateType(reflect.TypeOf(v), 0); err != nil {
		return errors.NewTypeNotAllowedError(err.Error())
	}

	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return errors.NewInvalidJsonError(err)
	}

	if err := j.ValidateValue(temp, 0); err != nil {
		return errors.NewInvalidStructError(err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	if j.Config.EnableStrict {
		decoder.DisallowUnknownFields()
		decoder.UseNumber()
	}

	return decoder.Decode(v)
}

func (j *JSONSerializer) ContentType() string {
	return "application/json"
}

func (j *JSONSerializer) EncodingType() EncodingType {
	return JSON
}

// RawSerializer to raw binary data
type RawSerializer struct {
	BaseSerializer
}

func NewRawSerializer(config SerializationConfig) *RawSerializer {
	return &RawSerializer{
		BaseSerializer: NewBaseSerializer(config),
	}
}

func (r *RawSerializer) Marshal(v interface{}) ([]byte, error) {
	data, ok := v.([]byte)
	if !ok {
		return nil, errors.ErrRawSerializer
	}

	if int64(len(data)) > r.Config.MaxBinarySize {
		return nil, errors.NewDataTooLongError(len(data))
	}

	return data, nil
}

func (r *RawSerializer) Unmarshal(data []byte, v interface{}) error {
	if int64(len(data)) > r.Config.MaxBinarySize {
		return errors.NewDataTooLongError(len(data))
	}

	ptr, ok := v.(*[]byte)
	if !ok {
		return errors.ErrRawSerializerPtr
	}

	*ptr = data
	return nil
}

func (r *RawSerializer) ContentType() string {
	return "application/octet-stream"
}

func (r *RawSerializer) EncodingType() EncodingType {
	return Raw
}
