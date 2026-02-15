package gosocket

import (
	"errors"
	"net/http"
	"testing"

	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageConstructorsFacade(t *testing.T) {
	msg := NewMessage(TextMessage, map[string]string{"ok": "true"})
	require.NotNil(t, msg)
	assert.Equal(t, TextMessage, msg.Type)
	assert.NotZero(t, msg.Created)

	msgWithEnc := NewMessageWithEncoding(BinaryMessage, "data", Raw)
	require.NotNil(t, msgWithEnc)
	assert.Equal(t, BinaryMessage, msgWithEnc.Type)
	assert.Equal(t, Raw, msgWithEnc.Encoding)
	assert.NotZero(t, msgWithEnc.Created)

	raw := []byte("payload")
	rawMsg := NewRawMessage(BinaryMessage, raw)
	require.NotNil(t, rawMsg)
	assert.Equal(t, message.Raw, rawMsg.Encoding)
	assert.Equal(t, raw, rawMsg.RawData)
}

func TestTestHelpersFacade(t *testing.T) {
	mgr := NewTestMemoryManager()
	require.NotNil(t, mgr)
	assert.NoError(t, mgr.Publish(nil))

	state := NewTestMemoryStateStore()
	require.NotNil(t, state)
}

func TestNewHandlerFacade(t *testing.T) {
	h, err := NewHandler()
	require.NoError(t, err)
	require.NotNil(t, h)
	assert.NotNil(t, h.Dispatcher())
	assert.NoError(t, h.Stop())
}

func TestEventOptions_SetHandlers(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "on_start",
			run: func(t *testing.T) {
				called := false
				s, err := NewServer(OnStart(func(ctx *Context) error {
					called = true
					return nil
				}))
				require.NoError(t, err)
				err = s.Handler().Events.OnStart(nil)
				assert.NoError(t, err)
				assert.True(t, called)
			},
		},
		{
			name: "before_connect_connect_disconnect_message",
			run: func(t *testing.T) {
				onBefore := false
				onConnect := false
				onDisconnect := false
				onMessage := false
				s, err := NewServer(
					OnBeforeConnect(func(r *http.Request, ctx *Context) error {
						onBefore = true
						return nil
					}),
					OnConnect(func(d Dispatcher, ctx *Context) error {
						onConnect = true
						return nil
					}),
					OnDisconnect(func(d Dispatcher, ctx *Context) error {
						onDisconnect = true
						return nil
					}),
					OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
						onMessage = true
						return nil
					}),
				)
				require.NoError(t, err)
				assert.NoError(t, s.Handler().Events.OnBeforeConnect(nil, nil))
				assert.NoError(t, s.Handler().Events.OnConnect(nil, nil))
				assert.NoError(t, s.Handler().Events.OnDisconnect(nil, nil))
				assert.NoError(t, s.Handler().Events.OnMessage(NewRawMessage(TextMessage, []byte("x")), nil, nil))
				assert.True(t, onBefore)
				assert.True(t, onConnect)
				assert.True(t, onDisconnect)
				assert.True(t, onMessage)
			},
		},
		{
			name: "raw_json_protobuf_ping_pong_error",
			run: func(t *testing.T) {
				onRaw := false
				onJSON := false
				onProto := false
				onPing := false
				onPong := false
				onErr := false
				s, err := NewServer(
					OnRawMessage(func(m []byte, d Dispatcher, ctx *Context) error {
						onRaw = true
						return nil
					}),
					OnJSONMessage(func(m interface{}, d Dispatcher, ctx *Context) error {
						onJSON = true
						return nil
					}),
					OnProtobufMessage(func(m interface{}, d Dispatcher, ctx *Context) error {
						onProto = true
						return nil
					}),
					OnPing(func(d Dispatcher, ctx *Context) error {
						onPing = true
						return nil
					}),
					OnPong(func(d Dispatcher, ctx *Context) error {
						onPong = true
						return nil
					}),
					OnError(func(err error, d Dispatcher, ctx *Context) error {
						onErr = true
						return nil
					}),
				)
				require.NoError(t, err)
				assert.NoError(t, s.Handler().Events.OnRawMessage([]byte("x"), nil, nil))
				assert.NoError(t, s.Handler().Events.OnJSONMessage(map[string]any{"x": 1}, nil, nil))
				assert.NoError(t, s.Handler().Events.OnProtobufMessage(struct{}{}, nil, nil))
				assert.NoError(t, s.Handler().Events.OnPing(nil, nil))
				assert.NoError(t, s.Handler().Events.OnPong(nil, nil))
				assert.NoError(t, s.Handler().Events.OnError(errors.New("x"), nil, nil))
				assert.True(t, onRaw)
				assert.True(t, onJSON)
				assert.True(t, onProto)
				assert.True(t, onPing)
				assert.True(t, onPong)
				assert.True(t, onErr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}
