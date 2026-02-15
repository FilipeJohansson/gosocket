package message

import (
	"reflect"
	"testing"

	errorspkg "github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unsupportedStruct struct {
	C chan int
}

func TestDefaultSerializerConfig(t *testing.T) {
	cfg := DefaultSerializerConfig()
	assert.Equal(t, 10, cfg.MaxDepth)
	assert.Equal(t, 100, cfg.MaxKeys)
	assert.Equal(t, 1000, cfg.MaxElements)
	assert.True(t, cfg.EnableStrict)
	assert.Positive(t, cfg.MaxBinarySize)
	assert.NotEmpty(t, cfg.DisallowedTypes)
}

func TestCreateSerializer(t *testing.T) {
	cfg := DefaultSerializerConfig()
	assert.IsType(t, &JSONSerializer{}, CreateSerializer(JSON, cfg))
	assert.IsType(t, &RawSerializer{}, CreateSerializer(Raw, cfg))
}

func TestBaseSerializer(t *testing.T) {
	base := NewBaseSerializer(DefaultSerializerConfig())
	base.Configure(SerializationConfig{MaxDepth: 1, MaxKeys: 1, MaxElements: 1, MaxStringLength: 2, MaxBinarySize: 8})

	err := base.ValidateValue(map[string]int{"a": 1, "b": 2}, 0)
	assert.ErrorIs(t, err, errorspkg.ErrMaxKeyLengthExceeded)

	err = base.ValidateValue([]int{1, 2}, 0)
	assert.ErrorIs(t, err, errorspkg.ErrMaxElementsExceeded)

	err = base.ValidateValue("abc", 0)
	assert.ErrorIs(t, err, errorspkg.ErrMaxStringLengthExceeded)

	err = base.ValidateValue(map[string]string{"a": "b"}, 2)
	assert.ErrorIs(t, err, errorspkg.ErrMaxDepthExceeded)

	err = base.ValidateType(reflect.TypeOf(make(chan int)), 0)
	assert.ErrorIs(t, err, errorspkg.ErrTypeNotAllowed)

	err = base.ValidateType(reflect.TypeOf((*int)(nil)), 2)
	assert.ErrorIs(t, err, errorspkg.ErrMaxDepthExceeded)

	assert.NoError(t, base.ValidateType(nil, 0))
}

func TestJSONSerializer(t *testing.T) {
	cfg := DefaultSerializerConfig()
	cfg.MaxStringLength = 4
	cfg.MaxBinarySize = 32
	jsonSer := NewJSONSerializer(cfg)

	data, err := jsonSer.Marshal(map[string]string{"ok": "yes"})
	require.NoError(t, err)
	require.NotEmpty(t, data)

	_, err = jsonSer.Marshal(unsupportedStruct{})
	assert.ErrorIs(t, err, errorspkg.ErrTypeNotAllowed)

	_, err = jsonSer.Marshal(map[string]string{"x": "12345"})
	assert.ErrorIs(t, err, errorspkg.ErroInvalidValue)

	var decoded map[string]string
	err = jsonSer.Unmarshal([]byte(`{"ok":"yes"}`), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "yes", decoded["ok"])

	err = jsonSer.Unmarshal(nil, &decoded)
	assert.ErrorIs(t, err, errorspkg.ErrEmptyData)

	err = jsonSer.Unmarshal(make([]byte, 33), &decoded)
	assert.ErrorIs(t, err, errorspkg.ErrDataTooLong)

	err = jsonSer.Unmarshal([]byte("{"), &decoded)
	assert.ErrorIs(t, err, errorspkg.ErrInvalidJSON)

	strictCfg := DefaultSerializerConfig()
	strictCfg.EnableStrict = true
	strict := NewJSONSerializer(strictCfg)
	var typed struct {
		Name string `json:"name"`
	}
	err = strict.Unmarshal([]byte(`{"name":"ok","extra":1}`), &typed)
	assert.Error(t, err)

	assert.Equal(t, "application/json", jsonSer.ContentType())
	assert.Equal(t, JSON, jsonSer.EncodingType())
}

func TestRawSerializer(t *testing.T) {
	cfg := DefaultSerializerConfig()
	cfg.MaxBinarySize = 4
	rawSer := NewRawSerializer(cfg)

	out, err := rawSer.Marshal([]byte("1234"))
	require.NoError(t, err)
	assert.Equal(t, []byte("1234"), out)

	_, err = rawSer.Marshal("not-bytes")
	assert.ErrorIs(t, err, errorspkg.ErrRawSerializer)

	_, err = rawSer.Marshal([]byte("12345"))
	assert.ErrorIs(t, err, errorspkg.ErrDataTooLong)

	var outBytes []byte
	err = rawSer.Unmarshal([]byte("12"), &outBytes)
	require.NoError(t, err)
	assert.Equal(t, []byte("12"), outBytes)

	err = rawSer.Unmarshal([]byte("12345"), &outBytes)
	assert.ErrorIs(t, err, errorspkg.ErrDataTooLong)

	var wrongType string
	err = rawSer.Unmarshal([]byte("12"), &wrongType)
	assert.ErrorIs(t, err, errorspkg.ErrRawSerializerPtr)

	assert.Equal(t, "application/octet-stream", rawSer.ContentType())
	assert.Equal(t, Raw, rawSer.EncodingType())
}
