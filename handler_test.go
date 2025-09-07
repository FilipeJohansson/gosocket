package gosocket

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func MockAuthSuccess(r *http.Request) (map[string]interface{}, error) {
	return map[string]interface{}{
		"user_id": "123",
		"role":    "admin",
	}, nil
}

func MockAuthFailure(r *http.Request) (map[string]interface{}, error) {
	return nil, errors.New("authentication failed")
}

func MockMiddleware1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Middleware-1", "applied")
		next.ServeHTTP(w, r)
	})
}

func MockMiddleware2(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Middleware-2", "applied")
		next.ServeHTTP(w, r)
	})
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler()

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.hub)
	assert.NotNil(t, handler.config)
	assert.NotNil(t, handler.handlers)
	assert.NotNil(t, handler.serializers)
	assert.Equal(t, 1024, handler.upgrader.ReadBufferSize)
	assert.Equal(t, 1024, handler.upgrader.WriteBufferSize)
}

func TestHandler_FluentInterface_MaxConnections(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{
			name:     "sets valid max connections",
			input:    500,
			expected: 500,
		},
		{
			name:     "handles zero max connections",
			input:    0,
			expected: 1000, // default
		},
		{
			name:     "handles negative max connections",
			input:    -100,
			expected: 1000, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithMaxConnections(tt.input)
			assert.Equal(t, tt.expected, handler.config.MaxConnections)
		})
	}
}

func TestHandler_FluentInterface_MessageSize(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int64
	}{
		{
			name:     "sets valid message size",
			input:    2048,
			expected: 2048,
		},
		{
			name:     "handles zero message size",
			input:    0,
			expected: 1024, // default
		},
		{
			name:     "handles negative message size",
			input:    -100,
			expected: 1024, // default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithMessageSize(tt.input)
			assert.Equal(t, tt.expected, handler.config.MessageSize)
		})
	}
}

func TestHandler_FluentInterface_Timeout(t *testing.T) {
	tests := []struct {
		name          string
		read          time.Duration
		write         time.Duration
		expectedRead  time.Duration
		expectedWrite time.Duration
	}{
		{
			name:          "sets valid timeouts",
			read:          30 * time.Second,
			write:         10 * time.Second,
			expectedRead:  30 * time.Second,
			expectedWrite: 10 * time.Second,
		},
		{
			name:          "handles negative timeouts",
			read:          -10 * time.Second,
			write:         -5 * time.Second,
			expectedRead:  0,
			expectedWrite: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithTimeout(tt.read, tt.write)
			assert.Equal(t, tt.expectedRead, handler.config.ReadTimeout)
			assert.Equal(t, tt.expectedWrite, handler.config.WriteTimeout)
		})
	}
}

func TestHandler_FluentInterface_PingPong(t *testing.T) {
	tests := []struct {
		name               string
		pingPeriod         time.Duration
		pongWait           time.Duration
		expectedPingPeriod time.Duration
		expectedPongWait   time.Duration
	}{
		{
			name:               "sets valid ping pong settings",
			pingPeriod:         30 * time.Second,
			pongWait:           35 * time.Second,
			expectedPingPeriod: 30 * time.Second,
			expectedPongWait:   35 * time.Second,
		},
		{
			name:               "handles invalid ping pong settings",
			pingPeriod:         0,
			pongWait:           -5 * time.Second,
			expectedPingPeriod: 0,
			expectedPongWait:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithPingPong(tt.pingPeriod, tt.pongWait)
			assert.Equal(t, tt.expectedPingPeriod, handler.config.PingPeriod)
			assert.Equal(t, tt.expectedPongWait, handler.config.PongWait)
		})
	}
}

func TestHandler_FluentInterface_AllowedOrigins(t *testing.T) {
	origins := []string{"http://localhost:3000", "https://example.com"}
	handler := NewHandler().WithAllowedOrigins(origins)

	assert.Equal(t, origins, handler.config.AllowedOrigins)
}

func TestHandler_FluentInterface_Encoding(t *testing.T) {
	handler := NewHandler().WithEncoding(Protobuf)
	assert.Equal(t, Protobuf, handler.config.DefaultEncoding)
}

func TestHandler_FluentInterface_Serializers(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Handler)
		serializer EncodingType
		wantType   interface{}
	}{
		{
			name: "WithSerializer",
			setup: func(h *Handler) {
				h.WithSerializer(JSON, JSONSerializer{})
			},
			serializer: JSON,
			wantType:   JSONSerializer{},
		},
		{
			name: "WithJSONSerializer",
			setup: func(h *Handler) {
				h.WithJSONSerializer()
			},
			serializer: JSON,
			wantType:   JSONSerializer{},
		},
		{
			name: "WithProtobufSerializer",
			setup: func(h *Handler) {
				h.WithProtobufSerializer()
			},
			serializer: Protobuf,
			wantType:   ProtobufSerializer{},
		},
		{
			name: "WithRawSerializer",
			setup: func(h *Handler) {
				h.WithRawSerializer()
			},
			serializer: Raw,
			wantType:   RawSerializer{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler()
			tt.setup(handler)
			assert.NotNil(t, handler.serializers[tt.serializer])
			assert.IsType(t, tt.wantType, handler.serializers[tt.serializer])
		})
	}
}

func TestHandler_FluentInterface_Middleware(t *testing.T) {
	handler := NewHandler()

	// Inicialmente deve estar vazio
	assert.Nil(t, handler.middlewares)

	handler.WithMiddleware(MockMiddleware1)
	assert.Len(t, handler.middlewares, 1)

	handler.WithMiddleware(MockMiddleware2)
	assert.Len(t, handler.middlewares, 2)
}

func TestHandler_FluentInterface_Auth(t *testing.T) {
	handler := NewHandler().WithAuth(MockAuthSuccess)
	assert.NotNil(t, handler.authFunc)

	// test that auth function works
	userData, err := handler.authFunc(&http.Request{})
	assert.NoError(t, err)
	assert.Equal(t, "123", userData["user_id"])
}

func TestHandler_EventHandlers(t *testing.T) {
	var (
		connectCalled     bool
		disconnectCalled  bool
		messageCalled     bool
		rawMessageCalled  bool
		jsonMessageCalled bool
		errorCalled       bool
		pingCalled        bool
		pongCalled        bool
	)

	handler := NewHandler().
		OnConnect(func(c *Client, ctx *HandlerContext) error {
			connectCalled = true
			return nil
		}).
		OnDisconnect(func(c *Client, ctx *HandlerContext) error {
			disconnectCalled = true
			return nil
		}).
		OnMessage(func(c *Client, m *Message, ctx *HandlerContext) error {
			messageCalled = true
			return nil
		}).
		OnRawMessage(func(c *Client, data []byte, ctx *HandlerContext) error {
			rawMessageCalled = true
			return nil
		}).
		OnJSONMessage(func(c *Client, data interface{}, ctx *HandlerContext) error {
			jsonMessageCalled = true
			return nil
		}).
		OnError(func(c *Client, err error, ctx *HandlerContext) error {
			errorCalled = true
			return nil
		}).
		OnPing(func(c *Client, ctx *HandlerContext) error {
			pingCalled = true
			return nil
		}).
		OnPong(func(c *Client, ctx *HandlerContext) error {
			pongCalled = true
			return nil
		})

	assert.NotNil(t, handler.handlers.OnConnect)
	assert.NotNil(t, handler.handlers.OnDisconnect)
	assert.NotNil(t, handler.handlers.OnMessage)
	assert.NotNil(t, handler.handlers.OnRawMessage)
	assert.NotNil(t, handler.handlers.OnJSONMessage)
	assert.NotNil(t, handler.handlers.OnError)
	assert.NotNil(t, handler.handlers.OnPing)
	assert.NotNil(t, handler.handlers.OnPong)

	// test that handlers can be called
	client := NewClient("test", &MockWebSocketConn{}, NewHub())
	message := NewMessage(TextMessage, "test")
	ctx := NewHandlerContext(handler)

	handler.handlers.OnConnect(client, ctx)
	handler.handlers.OnDisconnect(client, ctx)
	handler.handlers.OnMessage(client, message, ctx)
	handler.handlers.OnRawMessage(client, []byte("test"), ctx)
	handler.handlers.OnJSONMessage(client, map[string]string{"test": "data"}, ctx)
	handler.handlers.OnError(client, errors.New("test error"), ctx)
	handler.handlers.OnPing(client, ctx)
	handler.handlers.OnPong(client, ctx)

	assert.True(t, connectCalled)
	assert.True(t, disconnectCalled)
	assert.True(t, messageCalled)
	assert.True(t, rawMessageCalled)
	assert.True(t, jsonMessageCalled)
	assert.True(t, errorCalled)
	assert.True(t, pingCalled)
	assert.True(t, pongCalled)
}

func TestHandler_ProcessMessage(t *testing.T) {
	tests := []struct {
		name            string
		setupHandlers   func(*Handler)
		message         *Message
		expectOnMessage bool
		expectOnRaw     bool
		expectOnJSON    bool
		expectError     bool
	}{
		{
			name: "processes message with OnMessage handler",
			setupHandlers: func(h *Handler) {
				h.OnMessage(func(c *Client, m *Message, ctx *HandlerContext) error {
					return nil
				})
			},
			message:         NewRawMessage(TextMessage, []byte("test")),
			expectOnMessage: true,
		},
		{
			name: "processes message with OnRawMessage handler",
			setupHandlers: func(h *Handler) {
				h.OnRawMessage(func(c *Client, data []byte, ctx *HandlerContext) error {
					return nil
				})
			},
			message:     NewRawMessage(TextMessage, []byte("test")),
			expectOnRaw: true,
		},
		{
			name: "processes JSON message with OnJSONMessage handler",
			setupHandlers: func(h *Handler) {
				h.OnJSONMessage(func(c *Client, data interface{}, ctx *HandlerContext) error {
					return nil
				})
			},
			message:      NewRawMessage(TextMessage, []byte(`{"key":"value"}`)),
			expectOnJSON: true,
		},
		{
			name: "handles error in OnMessage handler",
			setupHandlers: func(h *Handler) {
				h.OnMessage(func(c *Client, m *Message, ctx *HandlerContext) error {
					return errors.New("handler error")
				})
				h.OnError(func(c *Client, err error, ctx *HandlerContext) error {
					assert.Contains(t, err.Error(), "handler error")
					return nil
				})
			},
			message:         NewRawMessage(TextMessage, []byte("test")),
			expectOnMessage: true,
			expectError:     true,
		},
		{
			name: "handles invalid JSON gracefully",
			setupHandlers: func(h *Handler) {
				h.OnJSONMessage(func(c *Client, data interface{}, ctx *HandlerContext) error {
					t.Fatal("OnJSONMessage should not be called with invalid JSON")
					return nil
				})
			},
			message:      NewRawMessage(TextMessage, []byte("invalid json")),
			expectOnJSON: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler()
			tt.setupHandlers(handler)

			client := NewClient("test", &MockWebSocketConn{}, NewHub())
			handler.processMessage(client, tt.message)

			// process is asynchronous, so we might need a small delay
			time.Sleep(10 * time.Millisecond)
		})
	}
}

func TestHandler_EnsureHubRunning(t *testing.T) {
	handler := NewHandler()

	// mock the hub's Run method to track if it was called
	mockHub := NewMockHub()
	mockHub.On("Run").Return()
	handler.hub = mockHub

	// call ensureHubRunning twice
	handler.ensureHubRunning()
	handler.ensureHubRunning()

	// wait a moment for goroutine to start
	time.Sleep(10 * time.Millisecond)

	// verify Run was called exactly once due to sync.Once
	mockHub.AssertNumberOfCalls(t, "Run", 1)
}

func TestHandler_ServeHTTP_OriginCheck(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		expectUpgrade  bool
	}{
		{
			name:           "allows any origin when none specified",
			allowedOrigins: []string{},
			requestOrigin:  "http://example.com",
			expectUpgrade:  true,
		},
		{
			name:           "allows specified origin",
			allowedOrigins: []string{"http://localhost:3000", "https://example.com"},
			requestOrigin:  "http://localhost:3000",
			expectUpgrade:  true,
		},
		{
			name:           "blocks unspecified origin",
			allowedOrigins: []string{"http://localhost:3000"},
			requestOrigin:  "http://malicious.com",
			expectUpgrade:  false,
		},
		{
			name:           "blocks empty origin when origins are specified",
			allowedOrigins: []string{"http://localhost:3000"},
			requestOrigin:  "",
			expectUpgrade:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithAllowedOrigins(tt.allowedOrigins)

			req := httptest.NewRequest("GET", "/ws", nil)
			req.Header.Set("Connection", "upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set("Sec-WebSocket-Version", "13")

			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}

			if handler.upgrader.CheckOrigin == nil {
				handler.upgrader.CheckOrigin = func(r *http.Request) bool {
					if len(handler.config.AllowedOrigins) == 0 {
						return true
					}

					origin := r.Header.Get("Origin")
					for _, allowed := range handler.config.AllowedOrigins {
						if origin == allowed {
							return true
						}
					}
					return false
				}
			}

			originAllowed := handler.upgrader.CheckOrigin(req)

			if tt.expectUpgrade {
				assert.True(t, originAllowed, "Origin should be allowed")
			} else {
				assert.False(t, originAllowed, "Origin should be blocked")
			}
		})
	}
}

func TestHandler_AuthenticationIntegration(t *testing.T) {
	tests := []struct {
		name       string
		authFunc   func(*http.Request) (map[string]interface{}, error)
		header     map[string]string
		wantUserID string
		wantRole   string
		wantErr    bool
	}{
		{
			name:       "successful authentication",
			authFunc:   MockAuthSuccess,
			header:     map[string]string{"Authorization": "Bearer valid-token"},
			wantUserID: "123",
			wantRole:   "admin",
			wantErr:    false,
		},
		{
			name:     "failed authentication",
			authFunc: MockAuthFailure,
			header:   map[string]string{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler().WithAuth(tt.authFunc)
			req := httptest.NewRequest("GET", "/ws", nil)
			for k, v := range tt.header {
				req.Header.Set(k, v)
			}

			userData, err := handler.authFunc(req)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, userData)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantUserID, userData["user_id"])
				assert.Equal(t, tt.wantRole, userData["role"])
			}
		})
	}
}

func TestHandler_ConcurrentAccess(t *testing.T) {
	handler := NewHandler()

	var wg sync.WaitGroup
	numGoroutines := 10

	// test concurrent configuration changes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			handler.WithMaxConnections(id * 100).
				WithMessageSize(int64(id * 1024)).
				WithEncoding(JSON).
				WithJSONSerializer()
		}(i)
	}

	// wait for completion
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// success
		assert.NotNil(t, handler.config)
		assert.NotNil(t, handler.serializers)
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible race condition")
	}
}

func TestHandler_DefaultConfig(t *testing.T) {
	config := DefaultHandlerConfig()

	assert.Equal(t, 1000, config.MaxConnections)
	assert.Equal(t, int64(512), config.MessageSize)
	assert.Equal(t, 60*time.Second, config.ReadTimeout)
	assert.Equal(t, 10*time.Second, config.WriteTimeout)
	assert.Equal(t, 54*time.Second, config.PingPeriod)
	assert.Equal(t, 60*time.Second, config.PongWait)
	assert.Equal(t, JSON, config.DefaultEncoding)
}

func TestHandler_ChainedConfiguration(t *testing.T) {
	// test that fluent interface returns the same handler instance
	handler := NewHandler().
		WithMaxConnections(500).
		WithMessageSize(2048).
		WithTimeout(30*time.Second, 15*time.Second).
		WithPingPong(45*time.Second, 50*time.Second).
		WithAllowedOrigins([]string{"http://localhost:3000"}).
		WithEncoding(JSON).
		WithJSONSerializer().
		WithAuth(MockAuthSuccess).
		OnConnect(func(c *Client, ctx *HandlerContext) error { return nil }).
		OnDisconnect(func(c *Client, ctx *HandlerContext) error { return nil })

	assert.Equal(t, 500, handler.config.MaxConnections)
	assert.Equal(t, int64(2048), handler.config.MessageSize)
	assert.Equal(t, 30*time.Second, handler.config.ReadTimeout)
	assert.Equal(t, 15*time.Second, handler.config.WriteTimeout)
	assert.Equal(t, 45*time.Second, handler.config.PingPeriod)
	assert.Equal(t, 50*time.Second, handler.config.PongWait)
	assert.Equal(t, []string{"http://localhost:3000"}, handler.config.AllowedOrigins)
	assert.Equal(t, JSON, handler.config.DefaultEncoding)
	assert.NotNil(t, handler.serializers[JSON])
	assert.NotNil(t, handler.authFunc)
	assert.NotNil(t, handler.handlers.OnConnect)
	assert.NotNil(t, handler.handlers.OnDisconnect)
}
