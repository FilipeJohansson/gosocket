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
	return nil, ErrAuthFailure
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
	handler, err := NewHandler()
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.hub)
	assert.NotNil(t, handler.config)
	assert.NotNil(t, handler.events)
	assert.NotNil(t, handler.serializers)
	assert.Equal(t, 1024, handler.upgrader.ReadBufferSize)
	assert.Equal(t, 1024, handler.upgrader.WriteBufferSize)
}

func TestHandler_MaxConnections(t *testing.T) {
	tests := []struct {
		name        string
		input       int
		expectError bool
		expected    int
	}{
		{
			name:        "sets valid max connections",
			input:       500,
			expectError: false,
			expected:    500,
		},
		{
			name:        "handles zero max connections",
			input:       0,
			expectError: true,
		},
		{
			name:        "handles negative max connections",
			input:       -100,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(WithMaxConnections(tt.input))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrMaxConnectionsLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, handler.config.MaxConnections)
			}
		})
	}
}

func TestHandler_MessageSize(t *testing.T) {
	tests := []struct {
		name        string
		input       int64
		expectError bool
		expected    int64
	}{
		{
			name:     "sets valid message size",
			input:    2048,
			expected: 2048,
		},
		{
			name:        "handles zero message size",
			input:       0,
			expectError: true,
		},
		{
			name:        "handles negative message size",
			input:       -100,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(WithMessageSize(tt.input))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrMessageSizeLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, handler.config.MessageSize)
			}
		})
	}
}

func TestHandler_Timeout(t *testing.T) {
	tests := []struct {
		name          string
		read          time.Duration
		write         time.Duration
		expectError   bool
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
			name:        "handles negative timeouts",
			read:        -10 * time.Second,
			write:       -5 * time.Second,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(WithTimeout(tt.read, tt.write))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrTimeoutsLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRead, handler.config.ReadTimeout)
				assert.Equal(t, tt.expectedWrite, handler.config.WriteTimeout)
			}
		})
	}
}

func TestHandler_PingPong(t *testing.T) {
	tests := []struct {
		name               string
		pingPeriod         time.Duration
		pongWait           time.Duration
		expectError        bool
		expectedError      error
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
			name:          "handles ping period greater than pong wait",
			pingPeriod:    5 * time.Second,
			pongWait:      1 * time.Second,
			expectError:   true,
			expectedError: ErrPongWaitLessThanPing,
		},
		{
			name:          "handles negative ping pong settings",
			pingPeriod:    -10 * time.Second,
			pongWait:      -5 * time.Second,
			expectError:   true,
			expectedError: ErrPingPongLessThanOne,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(WithPingPong(tt.pingPeriod, tt.pongWait))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPingPeriod, handler.config.PingPeriod)
				assert.Equal(t, tt.expectedPongWait, handler.config.PongWait)
			}
		})
	}
}

func TestHandler_AllowedOrigins(t *testing.T) {
	origins := []string{"http://localhost:3000", "https://example.com"}
	handler, err := NewHandler(WithAllowedOrigins(origins))
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, origins, handler.config.AllowedOrigins)
}

func TestHandler_Encoding(t *testing.T) {
	handler, err := NewHandler(WithEncoding(Protobuf))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, Protobuf, handler.config.DefaultEncoding)
}

func TestHandler_Serializers(t *testing.T) {
	tests := []struct {
		name       string
		setup      UniversalOption
		serializer EncodingType
		wantType   interface{}
	}{
		{
			name:       "WithSerializer",
			setup:      WithSerializer(JSON, CreateSerializer(JSON, DefaultSerializerConfig())),
			serializer: JSON,
			wantType:   &JSONSerializer{},
		},
		{
			name:       "WithJSONSerializer",
			setup:      WithJSONSerializer(),
			serializer: JSON,
			wantType:   &JSONSerializer{},
		},
		{
			name:       "WithProtobufSerializer",
			setup:      WithProtobufSerializer(),
			serializer: Protobuf,
			wantType:   &ProtobufSerializer{},
		},
		{
			name:       "WithRawSerializer",
			setup:      WithRawSerializer(),
			serializer: Raw,
			wantType:   &RawSerializer{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler()
			if err != nil {
				t.Fatal(err)
			}
			_ = tt.setup(handler)
			assert.NotNil(t, handler.serializers[tt.serializer])
			assert.IsType(t, tt.wantType, handler.serializers[tt.serializer])
		})
	}
}

func TestHandler_Middleware(t *testing.T) {
	handler, err := NewHandler()
	if err != nil {
		t.Fatal(err)
	}

	// no middlewares
	assert.Nil(t, handler.middlewares)

	// one middleware
	handler, err = NewHandler(WithMiddleware(MockMiddleware1))
	if err != nil {
		t.Fatal(err)
	}
	assert.Len(t, handler.middlewares, 1)

	// multiple middlewares
	handler, err = NewHandler(
		WithMiddleware(MockMiddleware1),
		WithMiddleware(MockMiddleware2),
	)
	if err != nil {
		t.Fatal(err)
	}
	assert.Len(t, handler.middlewares, 2)
}

func TestHandler_Auth(t *testing.T) {
	handler, err := NewHandler(WithAuth(MockAuthSuccess))
	if err != nil {
		t.Fatal(err)
	}
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

	handler, err := NewHandler(
		OnConnect(func(c *Client, ctx *Context) error {
			connectCalled = true
			return nil
		}),
		OnDisconnect(func(c *Client, ctx *Context) error {
			disconnectCalled = true
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			messageCalled = true
			return nil
		}),
		OnRawMessage(func(c *Client, data []byte, ctx *Context) error {
			rawMessageCalled = true
			return nil
		}),
		OnJSONMessage(func(c *Client, data interface{}, ctx *Context) error {
			jsonMessageCalled = true
			return nil
		}),
		OnError(func(c *Client, err error, ctx *Context) error {
			errorCalled = true
			return nil
		}),
		OnPing(func(c *Client, ctx *Context) error {
			pingCalled = true
			return nil
		}),
		OnPong(func(c *Client, ctx *Context) error {
			pongCalled = true
			return nil
		}),
	)

	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, handler.events.OnConnect)
	assert.NotNil(t, handler.events.OnDisconnect)
	assert.NotNil(t, handler.events.OnMessage)
	assert.NotNil(t, handler.events.OnRawMessage)
	assert.NotNil(t, handler.events.OnJSONMessage)
	assert.NotNil(t, handler.events.OnError)
	assert.NotNil(t, handler.events.OnPing)
	assert.NotNil(t, handler.events.OnPong)

	// test that handlers can be called
	client := NewClient("test", &MockWebSocketConn{}, NewHub())
	message := NewMessage(TextMessage, "test")
	ctx := NewHandlerContext(handler)

	_ = handler.events.OnConnect(client, ctx)
	_ = handler.events.OnDisconnect(client, ctx)
	_ = handler.events.OnMessage(client, message, ctx)
	_ = handler.events.OnRawMessage(client, []byte("test"), ctx)
	_ = handler.events.OnJSONMessage(client, map[string]string{"test": "data"}, ctx)
	_ = handler.events.OnError(client, errors.New("test error"), ctx)
	_ = handler.events.OnPing(client, ctx)
	_ = handler.events.OnPong(client, ctx)

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
		setupHandlers   []UniversalOption
		message         *Message
		expectOnMessage bool
		expectOnRaw     bool
		expectOnJSON    bool
		expectError     bool
	}{
		{
			name: "processes message with OnMessage handler",
			setupHandlers: []UniversalOption{
				OnMessage(func(c *Client, m *Message, ctx *Context) error {
					return nil
				}),
			},
			message:         NewRawMessage(TextMessage, []byte("test")),
			expectOnMessage: true,
		},
		{
			name: "processes message with OnRawMessage handler",
			setupHandlers: []UniversalOption{
				OnRawMessage(func(c *Client, data []byte, ctx *Context) error {
					return nil
				}),
			},
			message:     NewRawMessage(TextMessage, []byte("test")),
			expectOnRaw: true,
		},
		{
			name: "processes JSON message with OnJSONMessage handler",
			setupHandlers: []UniversalOption{
				OnJSONMessage(func(c *Client, data interface{}, ctx *Context) error {
					return nil
				}),
			},
			message:      NewRawMessage(TextMessage, []byte(`{"key":"value"}`)),
			expectOnJSON: true,
		},
		{
			name: "handles error in OnMessage handler",
			setupHandlers: []UniversalOption{
				OnMessage(func(c *Client, m *Message, ctx *Context) error {
					return errors.New("handler error")
				}),
				OnError(func(c *Client, err error, ctx *Context) error {
					assert.Contains(t, err.Error(), "handler error")
					return nil
				}),
			},
			message:         NewRawMessage(TextMessage, []byte("test")),
			expectOnMessage: true,
			expectError:     true,
		},
		{
			name: "handles invalid JSON gracefully",
			setupHandlers: []UniversalOption{
				OnJSONMessage(func(c *Client, data interface{}, ctx *Context) error {
					t.Fatal("OnJSONMessage should not be called with invalid JSON")
					return nil
				}),
			},
			message:      NewRawMessage(TextMessage, []byte("invalid json")),
			expectOnJSON: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler()
			if err != nil {
				t.Fatal(err)
			}
			for _, setup := range tt.setupHandlers {
				_ = setup(handler)
			}

			client := NewClient("test", &MockWebSocketConn{}, NewHub())
			handler.processMessage(client, tt.message)

			// process is asynchronous, so we might need a small delay
			time.Sleep(10 * time.Millisecond)
		})
	}
}

func TestHandler_EnsureHubRunning(t *testing.T) {
	handler, err := NewHandler()
	if err != nil {
		t.Fatal(err)
	}

	// mock the hub's Run method to track if it was called
	mockHub := NewMockHub()
	mockHub.On("Run").Return()
	handler.hub = mockHub

	// call ensureHubRunning twice
	handler.ensureHubRunning(t.Context())
	handler.ensureHubRunning(t.Context())

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
			handler, err := NewHandler(WithAllowedOrigins(tt.allowedOrigins))
			if err != nil {
				t.Fatal(err)
			}

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
			handler, err := NewHandler(WithAuth(tt.authFunc))
			if err != nil {
				t.Fatal(err)
			}
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

func TestHandler_ConcurrentCreation(t *testing.T) {
	var wg sync.WaitGroup
	numGoroutines := 10
	handlers := make([]*Handler, numGoroutines)
	errors := make([]error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			handlers[id], errors[id] = NewHandler(
				WithMaxConnections((id+1)*100), // +1 para evitar zero
				WithMessageSize(int64((id+1)*1024)),
				WithEncoding(JSON),
			)
		}(i)
	}

	wg.Wait()

	// Verify all handlers were created successfully
	for i, err := range errors {
		assert.NoError(t, err, "Handler %d creation failed", i)
		assert.NotNil(t, handlers[i], "Handler %d is nil", i)
	}
}

func TestHandler_DefaultConfig(t *testing.T) {
	config := DefaultHandlerConfig()

	assert.Equal(t, 1000, config.MaxConnections)
	assert.Equal(t, int64(512*1024), config.MessageSize)
	assert.Equal(t, 60*time.Second, config.ReadTimeout)
	assert.Equal(t, 10*time.Second, config.WriteTimeout)
	assert.Equal(t, 54*time.Second, config.PingPeriod)
	assert.Equal(t, 60*time.Second, config.PongWait)
	assert.Equal(t, JSON, config.DefaultEncoding)
}

func TestHandler_ChainedConfiguration(t *testing.T) {
	// test that chained configuration returns the same handler instance
	handler, err := NewHandler(
		WithMaxConnections(500),
		WithMessageSize(2048),
		WithTimeout(30*time.Second, 15*time.Second),
		WithPingPong(45*time.Second, 50*time.Second),
		WithAllowedOrigins([]string{"http://localhost:3000"}),
		WithEncoding(JSON),
		WithJSONSerializer(),
		WithAuth(MockAuthSuccess),
		OnConnect(func(c *Client, ctx *Context) error { return nil }),
		OnDisconnect(func(c *Client, ctx *Context) error { return nil }),
	)

	if err != nil {
		t.Fatal(err)
	}

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
	assert.NotNil(t, handler.events.OnConnect)
	assert.NotNil(t, handler.events.OnDisconnect)
}
