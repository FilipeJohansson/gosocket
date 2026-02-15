package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageConstructors(t *testing.T) {
	msg := NewMessage(TextMessage, map[string]string{"a": "b"})
	require.NotNil(t, msg)
	assert.Equal(t, TextMessage, msg.Type)
	assert.NotZero(t, msg.Created)

	msgWithEnc := NewMessageWithEncoding(BinaryMessage, "raw", JSON)
	require.NotNil(t, msgWithEnc)
	assert.Equal(t, BinaryMessage, msgWithEnc.Type)
	assert.Equal(t, JSON, msgWithEnc.Encoding)
	assert.NotZero(t, msgWithEnc.Created)

	rawBytes := []byte("hello")
	rawMsg := NewRawMessage(BinaryMessage, rawBytes)
	require.NotNil(t, rawMsg)
	assert.Equal(t, Raw, rawMsg.Encoding)
	assert.Equal(t, rawBytes, rawMsg.RawData)
	assert.NotZero(t, rawMsg.Created)
}
