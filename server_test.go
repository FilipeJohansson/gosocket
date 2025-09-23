package gosocket

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name     string
		expected func(*Server)
	}{
		{
			name: "creates server with default values",
			expected: func(s *Server) {
				assert.NotNil(t, s.handler)
				assert.NotNil(t, s.config)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer()
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithPort(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		expectError bool
		expected    func(*Server)
	}{
		{
			name:        "sets port correctly",
			port:        8080,
			expectError: false,
			expected:    nil,
		},
		{
			name:        "sets port off range",
			port:        70000,
			expectError: true,
			expected: func(s *Server) {
				assert.Equal(t, 8080, s.config.Port)
			},
		},
		{
			name:        "sets negative port",
			port:        -1,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "sets zero port",
			port:        0,
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithPort(tt.port))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrInvalidPort.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected func(*Server)
	}{
		{
			name: "sets path correctly",
			path: "/ws",
			expected: func(s *Server) {
				assert.Equal(t, "/ws", s.config.Path)
			},
		},
		{
			name: "sets empty path",
			path: "",
			expected: func(s *Server) {
				assert.Equal(t, "/", s.config.Path)
			},
		},
		{
			name: "sets path without leading slash",
			path: "ws",
			expected: func(s *Server) {
				assert.Equal(t, "/ws", s.config.Path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithPath(tt.path))
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithCORS(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected func(*Server)
	}{
		{
			name:    "enables CORS",
			enabled: true,
			expected: func(s *Server) {
				assert.True(t, s.config.EnableCORS)
			},
		},
		{
			name:    "disables CORS",
			enabled: false,
			expected: func(s *Server) {
				assert.False(t, s.config.EnableCORS)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithCORS(tt.enabled))
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithSSL(t *testing.T) {
	tests := []struct {
		name        string
		certFile    string
		keyFile     string
		expectError bool
		expected    func(*Server)
	}{
		{
			name:        "sets SSL cert and key files",
			certFile:    "server.crt",
			keyFile:     "server.key",
			expectError: false,
			expected: func(s *Server) {
				assert.True(t, s.config.EnableSSL)
				assert.Equal(t, "server.crt", s.config.CertFile)
				assert.Equal(t, "server.key", s.config.KeyFile)
			},
		},
		{
			name:        "sets empty cert and key files",
			certFile:    "",
			keyFile:     "",
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithSSL(tt.certFile, tt.keyFile))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrSSLFilesEmpty.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithMaxConnections(t *testing.T) {
	tests := []struct {
		name        string
		maxConns    int
		expectError bool
		expected    func(*Server)
	}{
		{
			name:        "sets positive max connections",
			maxConns:    100,
			expectError: false,
			expected: func(s *Server) {
				assert.Equal(t, 100, s.handler.Config().MaxConnections)
			},
		},
		{
			name:        "sets zero max connections",
			maxConns:    0,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "sets negative max connections",
			maxConns:    -10,
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxConnections(tt.maxConns))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrMaxConnectionsLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithMessageSize(t *testing.T) {
	tests := []struct {
		name        string
		size        int64
		expectError bool
		expected    func(*Server)
	}{
		{
			name:        "sets positive message size",
			size:        2048,
			expectError: false,
			expected: func(s *Server) {
				assert.Equal(t, int64(2048), s.handler.Config().MessageSize)
			},
		},
		{
			name:        "sets zero message size",
			size:        0,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "sets negative message size",
			size:        -100,
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMessageSize(tt.size))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrMessageSizeLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithTimeout(t *testing.T) {
	tests := []struct {
		name        string
		read        time.Duration
		write       time.Duration
		expectError bool
		expected    func(*Server)
	}{
		{
			name:        "sets read and write timeouts",
			read:        10 * time.Second,
			write:       15 * time.Second,
			expectError: false,
			expected: func(s *Server) {
				assert.Equal(t, 10*time.Second, s.handler.Config().ReadTimeout)
				assert.Equal(t, 15*time.Second, s.handler.Config().WriteTimeout)
			},
		},
		{
			name:        "sets zero timeouts",
			read:        0,
			write:       0,
			expectError: true,
			expected:    nil,
		},
		{
			name:        "sets negative timeouts",
			read:        -5 * time.Second,
			write:       -10 * time.Second,
			expectError: true,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithTimeout(tt.read, tt.write))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), ErrTimeoutsLessThanOne.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithPingPong(t *testing.T) {
	tests := []struct {
		name          string
		pingPeriod    time.Duration
		pongWait      time.Duration
		isExpectError bool
		expectError   error
		expected      func(*Server)
	}{
		{
			name:       "sets ping period and pong wait",
			pingPeriod: 30 * time.Second,
			pongWait:   60 * time.Second,
			expected: func(s *Server) {
				assert.Equal(t, 30*time.Second, s.handler.Config().PingPeriod)
				assert.Equal(t, 60*time.Second, s.handler.Config().PongWait)
			},
		},
		{
			name:          "sets zero ping period and pong wait",
			pingPeriod:    0,
			pongWait:      0,
			isExpectError: true,
			expectError:   ErrPingPongLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets negative ping period and pong wait",
			pingPeriod:    -10 * time.Second,
			pongWait:      -20 * time.Second,
			isExpectError: true,
			expectError:   ErrPingPongLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets pong wait less than ping period",
			pingPeriod:    10 * time.Second,
			pongWait:      5 * time.Second,
			isExpectError: true,
			expectError:   ErrPongWaitLessThanPing,
			expected:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithPingPong(tt.pingPeriod, tt.pongWait))
			if tt.isExpectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithAllowedOrigins(t *testing.T) {
	tests := []struct {
		name     string
		origins  []string
		expected func(*Server)
	}{
		{
			name:    "sets allowed origins",
			origins: []string{"http://example.com", "http://localhost"},
			expected: func(s *Server) {
				assert.Equal(t, []string{"http://example.com", "http://localhost"}, s.handler.Config().AllowedOrigins)
			},
		},
		{
			name:    "sets empty allowed origins",
			origins: []string{},
			expected: func(s *Server) {
				assert.Empty(t, s.handler.Config().AllowedOrigins)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithAllowedOrigins(tt.origins))
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithEncoding(t *testing.T) {
	tests := []struct {
		name     string
		encoding EncodingType
		expected func(*Server)
	}{
		{
			name:     "sets JSON encoding",
			encoding: JSON,
			expected: func(s *Server) {
				assert.Equal(t, JSON, s.handler.Config().DefaultEncoding)
			},
		},
		{
			name:     "sets Raw encoding",
			encoding: Raw,
			expected: func(s *Server) {
				assert.Equal(t, Raw, s.handler.Config().DefaultEncoding)
			},
		},
		{
			name:     "sets Protobuf encoding",
			encoding: Protobuf,
			expected: func(s *Server) {
				assert.Equal(t, Protobuf, s.handler.Config().DefaultEncoding)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithEncoding(tt.encoding))
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithSerializer(t *testing.T) {
	tests := []struct {
		name       string
		encoding   EncodingType
		serializer Serializer
		expected   func(*Server)
	}{
		{
			name:       "sets JSON serializer",
			encoding:   JSON,
			serializer: CreateSerializer(JSON, DefaultSerializerConfig()),
			expected: func(s *Server) {
				ser, exists := s.handler.Serializers()[JSON]
				assert.True(t, exists)
				assert.IsType(t, &JSONSerializer{}, ser)
			},
		},
		{
			name:       "sets Protobuf serializer",
			encoding:   Protobuf,
			serializer: CreateSerializer(Protobuf, DefaultSerializerConfig()),
			expected: func(s *Server) {
				ser, exists := s.handler.Serializers()[Protobuf]
				assert.True(t, exists)
				assert.IsType(t, &ProtobufSerializer{}, ser)
			},
		},
		{
			name:       "sets Raw serializer",
			encoding:   Raw,
			serializer: CreateSerializer(Raw, DefaultSerializerConfig()),
			expected: func(s *Server) {
				ser, exists := s.handler.Serializers()[Raw]
				assert.True(t, exists)
				assert.IsType(t, &RawSerializer{}, ser)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithSerializer(tt.encoding, tt.serializer))
			if err != nil {
				t.Fatal(err)
			}
			tt.expected(server)
		})
	}
}

func TestServer_WithJSONSerializer(t *testing.T) {
	server, err := NewServer(WithJSONSerializer())
	if err != nil {
		t.Fatal(err)
	}

	ser, exists := server.handler.Serializers()[JSON]
	assert.True(t, exists)
	assert.IsType(t, &JSONSerializer{}, ser)
}

func TestServer_WithRawSerializer(t *testing.T) {
	server, err := NewServer(WithRawSerializer())
	if err != nil {
		t.Fatal(err)
	}

	ser, exists := server.handler.Serializers()[Raw]
	assert.True(t, exists)
	assert.IsType(t, &RawSerializer{}, ser)
}

func TestServer_WithMiddleware(t *testing.T) {
	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-1", "true")
			next.ServeHTTP(w, r)
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-2", "true")
			next.ServeHTTP(w, r)
		})
	}

	server, err := NewServer(
		WithMiddleware(middleware1),
		WithMiddleware(middleware2),
	)
	if err != nil {
		t.Fatal(err)
	}

	assert.Len(t, server.handler.Middlewares(), 2)
}

func TestServer_WithAuth(t *testing.T) {
	authFunc := func(r *http.Request) (map[string]interface{}, error) {
		token := r.Header.Get("Authorization")
		if token == "" {
			return nil, errors.New("missing authorization header")
		}
		return map[string]interface{}{"user_id": "123"}, nil
	}

	server, err := NewServer(WithAuth(authFunc))
	if err != nil {
		t.Fatal(err)
	}

	assert.NotNil(t, server.handler.AuthFunc())
}

func TestServer_EventHandlers(t *testing.T) {
	tests := []struct {
		name     string
		setup    UniversalOption
		validate func(*Server)
	}{
		{
			name:  "OnConnect handler",
			setup: OnConnect(func(c *Client, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnConnect)
			},
		},
		{
			name:  "OnDisconnect handler",
			setup: OnDisconnect(func(c *Client, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnDisconnect)
			},
		},
		{
			name:  "OnMessage handler",
			setup: OnMessage(func(c *Client, m *Message, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnMessage)
			},
		},
		{
			name:  "OnRawMessage handler",
			setup: OnRawMessage(func(c *Client, data []byte, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnRawMessage)
			},
		},
		{
			name:  "OnJSONMessage handler",
			setup: OnJSONMessage(func(c *Client, data interface{}, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnJSONMessage)
			},
		},
		{
			name:  "OnProtobufMessage handler",
			setup: OnProtobufMessage(func(c *Client, data interface{}, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnProtobufMessage)
			},
		},
		{
			name:  "OnError handler",
			setup: OnError(func(c *Client, err error, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnError)
			},
		},
		{
			name:  "OnPing handler",
			setup: OnPing(func(c *Client, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnPing)
			},
		},
		{
			name:  "OnPong handler",
			setup: OnPong(func(c *Client, ctx *Context) error { return nil }),
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.Handlers().OnPong)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer()
			if err != nil {
				t.Fatal(err)
			}
			_ = tt.setup(server)
			tt.validate(server)
		})
	}
}

func TestServer_Broadcast(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		message         []byte
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts message successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			message:         []byte("test message"),
			isExpectedError: false,
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler = nil
				return server
			},
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrServerNotInitialized,
		},
		{
			name: "fails when hub is nil",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler.SetHub(nil)
				return server
			},
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrServerNotInitialized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.Broadcast(tt.message)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastMessage(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		message         *Message
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts message successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			message:         NewMessage(TextMessage, "test"),
			isExpectedError: false,
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler = nil
				return server
			},
			message:         NewMessage(TextMessage, "test"),
			isExpectedError: true,
			expectedError:   ErrServerNotInitialized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastMessage(tt.message)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastJSON(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		data            interface{}
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts JSON successfully",
			setupServer: func() *Server {
				server, err := NewServer(WithJSONSerializer())
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			data:            map[string]string{"key": "value"},
			isExpectedError: false,
		},
		{
			name: "fails with invalid JSON data",
			setupServer: func() *Server {
				server, err := NewServer(WithJSONSerializer())
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				server.handler.SetHub(mockHub)
				return server
			},
			data:            make(chan int), // channels can't be marshaled to JSON
			isExpectedError: true,
			expectedError:   ErrSerializeData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastJSON(tt.data)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastToRoom(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		room            string
		message         []byte
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts to room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastToRoom", "test-room", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			room:            "test-room",
			message:         []byte("test message"),
			isExpectedError: false,
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler = nil
				return server
			},
			room:            "test-room",
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrServerNotInitialized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastToRoom(tt.room, tt.message)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastToRoomJSON(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		room            string
		data            interface{}
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts JSON to room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastToRoom", "test-room", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			room:            "test-room",
			data:            map[string]string{"key": "value"},
			isExpectedError: false,
		},
		{
			name: "fails with invalid JSON data",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				server.handler.SetHub(mockHub)
				return server
			},
			room:            "test-room",
			data:            make(chan int),
			isExpectedError: true,
			expectedError:   ErrSerializeData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastToRoomJSON(tt.room, tt.data)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_GetClients(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		expected    int
	}{
		{
			name: "returns empty slice when hub is nil",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler.SetHub(nil)
				return server
			},
			expected: 0,
		},
		{
			name: "returns clients from hub",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())

				// Add some mock clients
				client1 := NewClient("client1", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				client2 := NewClient("client2", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)

				hub.Clients.Add(client1, client1.ID)
				hub.Clients.Add(client2, client2.ID)

				server.handler.SetHub(hub)
				return server
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			clients := server.GetClients()

			assert.Len(t, clients, tt.expected)
		})
	}
}

func TestServer_GetClient(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		clientID    string
		expectFound bool
	}{
		{
			name: "finds existing client",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())

				client := NewClient("test-client", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				hub.Clients.Add(client, client.ID)

				server.handler.SetHub(hub)
				return server
			},
			clientID:    "test-client",
			expectFound: true,
		},
		{
			name: "returns nil for non-existing client",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())
				server.handler.SetHub(hub)
				return server
			},
			clientID:    "non-existing",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			client := server.GetClient(tt.clientID)

			if tt.expectFound {
				assert.NotNil(t, client)
				assert.Equal(t, tt.clientID, client.ID)
			} else {
				assert.Nil(t, client)
			}
		})
	}
}

func TestServer_GetClientCount(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		expected    int
	}{
		{
			name: "returns 0 when hub is nil",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler.SetHub(nil)
				return server
			},
			expected: 0,
		},
		{
			name: "returns correct client count",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())

				// Add some mock clients
				client1 := NewClient("client1", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				client2 := NewClient("client2", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				client3 := NewClient("client3", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)

				hub.Clients.Add(client1, client1.ID)
				hub.Clients.Add(client2, client2.ID)
				hub.Clients.Add(client3, client3.ID)

				server.handler.SetHub(hub)
				return server
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			count := server.GetClientCount()

			assert.Equal(t, tt.expected, count)
		})
	}
}

func TestServer_RoomManagement(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		operation       string
		roomName        string
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "creates room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", mock.AnythingOfType("string"), "test-room")
				mockHub.On("Log", mock.AnythingOfType("LogType"), mock.AnythingOfType("LogLevel"), mock.AnythingOfType("string"), mock.AnythingOfType("[]interface {}"))
				server.handler.SetHub(mockHub)
				return server
			},
			operation:       "create",
			roomName:        "test-room",
			isExpectedError: false,
		},
		{
			name: "fails to create room with empty name",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", mock.AnythingOfType("string"), "").Return(ErrRoomNameEmpty)
				server.handler.SetHub(mockHub)
				return server
			},
			operation:       "create",
			roomName:        "",
			isExpectedError: true,
			expectedError:   ErrRoomNameEmpty,
		},
		{
			name: "deletes room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", mock.AnythingOfType("string"), "test-room").Return(nil)
				mockHub.On("DeleteRoom", "test-room").Return(nil)
				mockHub.On("Log", mock.AnythingOfType("LogType"), mock.AnythingOfType("LogLevel"), mock.AnythingOfType("string"), mock.AnythingOfType("[]interface {}"))
				server.handler.SetHub(mockHub)
				_, _ = server.CreateRoom("test-room")
				return server
			},
			operation:       "delete",
			roomName:        "test-room",
			isExpectedError: false,
		},
		{
			name: "fails to delete non-existing room",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("DeleteRoom", "non-existing").Return(newRoomNotFoundError("non-existing"))
				server.handler.SetHub(mockHub)
				return server
			},
			operation:       "delete",
			roomName:        "non-existing",
			isExpectedError: true,
			expectedError:   newRoomNotFoundError("non-existing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			var err error
			switch tt.operation {
			case "create":
				_, err = server.CreateRoom(tt.roomName)
			case "delete":
				err = server.DeleteRoom(tt.roomName)
			}

			if !tt.isExpectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}

			if mockHub, ok := server.handler.Hub().(*MockHub); ok {
				mockHub.AssertExpectations(t)
			}
		})
	}
}

func TestServer_GetRooms(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		expected    []string
	}{
		{
			name: "returns empty slice when hub is nil",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler.SetHub(nil)
				return server
			},
			expected: []string{},
		},
		{
			name: "returns room names",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}

				_, _ = server.CreateRoom("room1")
				_, _ = server.CreateRoom("room2")
				_, _ = server.CreateRoom("room3")

				return server
			},
			expected: []string{"room1", "room2", "room3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			rooms := server.GetRooms()

			assert.ElementsMatch(t, tt.expected, rooms)
		})
	}
}

func TestServer_ClientRoomOperations(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		operation       string
		clientID        string
		room            string
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "joins room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())

				client := NewClient("test-client", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				hub.Clients.Add(client, client.ID)

				_, _ = hub.CreateRoom("test-client", "test-room")

				server.handler.SetHub(hub)
				return server
			},
			operation:       "join",
			clientID:        "test-client",
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails to join room with non-existing client",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())
				server.handler.SetHub(hub)
				return server
			},
			operation:       "join",
			clientID:        "non-existing",
			room:            "test-room",
			isExpectedError: true,
			expectedError:   newClientNotFoundError("non-existing"),
		},
		{
			name: "leaves room successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())

				client := NewClient("test-client", &MockWebSocketConn{}, hub, server.handler.config.MessageChanBufSize)
				hub.Clients.Add(client, client.ID)

				server.handler.SetHub(hub)
				return server
			},
			operation:       "leave",
			clientID:        "test-client",
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails to leave room with non-existing client",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				hub := NewHub(DefaultLoggerConfig())
				server.handler.SetHub(hub)
				return server
			},
			operation:       "leave",
			clientID:        "non-existing",
			room:            "test-room",
			isExpectedError: true,
			expectedError:   newClientNotFoundError("non-existing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			var err error
			switch tt.operation {
			case "join":
				err = server.JoinRoom(tt.clientID, tt.room)
			case "leave":
				err = server.LeaveRoom(tt.clientID, tt.room)
			}

			if !tt.isExpectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_DisconnectClient(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		clientID        string
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "disconnects client successfully",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()

				mockConn := &MockWebSocketConn{}
				mockConn.On("Close").Return(nil)

				client := NewClient("test-client", mockConn, mockHub, server.handler.config.MessageChanBufSize)

				// Mock the hub methods that will be called during disconnect
				mockHub.On("RemoveClient", client).Return()
				mockHub.On("GetClients")
				mockHub.Clients.Add(client, client.ID)

				server.handler.SetHub(mockHub)
				return server
			},
			clientID:        "test-client",
			isExpectedError: false,
		},
		{
			name: "fails to disconnect non-existing client",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()

				mockHub.On("GetClients")
				mockHub.Clients = NewSharedCollection[*Client, string]() // Empty clients collection
				server.handler.SetHub(mockHub)
				return server
			},
			clientID:        "non-existing",
			isExpectedError: true,
			expectedError:   newClientNotFoundError("non-existing"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.DisconnectClient(tt.clientID)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_Stop(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "fails when server is not running",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.isRunning = false
				return server
			},
			isExpectedError: true,
			expectedError:   ErrServerNotRunning,
		},
		{
			name: "fails when server is nil",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.isRunning = true
				server.server = nil
				return server
			},
			isExpectedError: true,
			expectedError:   ErrServerNotRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.Stop()

			if !tt.isExpectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastData(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		data            interface{}
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts data with JSON serializer",
			setupServer: func() *Server {
				server, err := NewServer(WithJSONSerializer())
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			data:            map[string]string{"key": "value"},
			isExpectedError: false,
		},
		{
			name: "falls back to JSON when no serializer",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				server.handler.SetDefaultEncoding(Raw) // No serializer for Raw
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			data:            map[string]string{"key": "value"},
			isExpectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastData(tt.data)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_BroadcastDataWithEncoding(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		data            interface{}
		encoding        EncodingType
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "broadcasts with JSON encoding",
			setupServer: func() *Server {
				server, err := NewServer(WithJSONSerializer())
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			data:            map[string]string{"key": "value"},
			encoding:        JSON,
			isExpectedError: false,
		},
		{
			name: "fails with unsupported encoding",
			setupServer: func() *Server {
				server, err := NewServer()
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				server.handler.SetHub(mockHub)
				return server
			},
			data:            "test data",
			encoding:        EncodingType(999),
			isExpectedError: true,
			expectedError:   newSerializerNotFoundError(EncodingType(999)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastDataWithEncoding(tt.data, tt.encoding)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestServer_ValidateValue(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		value       interface{}
		expectError bool
		expectedErr string
	}{
		{
			name: "validates simple string within limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       "hello world",
			expectError: false,
		},
		{
			name: "validates map within limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxKeys(3),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectError: false,
		},
		{
			name: "fails when map exceeds max keys",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxKeys(2),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			expectError: true,
			expectedErr: "key length",
		},
		{
			name: "validates slice within limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxElements(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       []string{"item1", "item2", "item3"},
			expectError: false,
		},
		{
			name: "fails when slice exceeds max elements",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxElements(2),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       []string{"item1", "item2", "item3"},
			expectError: true,
			expectedErr: "elements",
		},
		{
			name: "fails when depth exceeded",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(1),
					WithMaxKeys(10),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: map[string]interface{}{
				"level1": map[string]string{
					"level2": "deep value",
				},
			},
			expectError: true,
			expectedErr: "depth",
		},
		{
			name: "validates array within limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxElements(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       [3]int{1, 2, 3},
			expectError: false,
		},
		{
			name: "validates nil pointer",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       (*string)(nil),
			expectError: false,
		},
		{
			name: "validates struct with fields",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: struct {
				Name string
				Age  int
			}{
				Name: "test",
				Age:  25,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			mockHub := NewMockHub()

			if !tt.expectError {
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
			}

			server.handler.SetHub(mockHub)

			err := server.BroadcastData(tt.value)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectedErr))
				}
			} else {
				assert.NoError(t, err)
				mockHub.AssertExpectations(t)
			}
		})
	}
}

func TestServer_ValidateType(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		value       interface{}
		expectError bool
		expectedErr string
	}{
		{
			name: "allows basic types",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       "string",
			expectError: false,
		},
		{
			name: "rejects function type",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       func() {},
			expectError: true,
			expectedErr: "not allowed",
		},
		{
			name: "rejects channel type",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       make(chan int),
			expectError: true,
			expectedErr: "not allowed",
		},
		{
			name: "rejects unsafe pointer type",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       unsafe.Pointer(&struct{}{}),
			expectError: true,
			expectedErr: "not allowed",
		},
		{
			name: "allows pointer to valid type",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: func() interface{} {
				s := "test"
				return &s
			}(),
			expectError: false,
		},
		{
			name: "validates nested pointer types",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value: func() interface{} {
				s := "test"
				p := &s
				return &p
			}(),
			expectError: false,
		},
		{
			name: "rejects disallowed custom types",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithDisallowedTypes([]string{"chan"}),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       make(chan int),
			expectError: true,
			expectedErr: "not allowed",
		},
		{
			name: "fails when type depth exceeded",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(1),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			value:       [][]string{{"nested"}},
			expectError: true,
			expectedErr: "depth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			mockHub := NewMockHub()

			if !tt.expectError {
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
			}

			server.handler.SetHub(mockHub)

			err := server.BroadcastData(tt.value)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectedErr))
				}
			} else {
				assert.NoError(t, err)
				mockHub.AssertExpectations(t)
			}
		})
	}
}

func TestServer_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		jsonData        string
		target          interface{}
		isExpectedError bool
		expectedError   string
		validateResult  func(interface{}) bool
	}{
		{
			name: "unmarshals valid JSON object",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxKeys(10),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData: `{"name": "test", "age": 25}`,
			target:   &map[string]interface{}{},
			validateResult: func(result interface{}) bool {
				data := result.(*map[string]interface{})
				return (*data)["name"] == "test" && (*data)["age"].(json.Number).String() == "25"
			},
			isExpectedError: false,
		},
		{
			name: "unmarshals valid JSON array",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxElements(5),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData: `["item1", "item2", "item3"]`,
			target:   &[]string{},
			validateResult: func(result interface{}) bool {
				data := result.(*[]string)
				return len(*data) == 3 && (*data)[0] == "item1"
			},
			isExpectedError: false,
		},
		{
			name: "fails with empty data",
			setupServer: func() *Server {
				server, err := NewServer(WithJSONSerializer())
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData:        "",
			target:          &map[string]interface{}{},
			isExpectedError: true,
			expectedError:   "empty data",
		},
		{
			name: "fails with data exceeding max binary size",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxBinarySize(10),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData:        `{"very_long_key_name": "very long value that exceeds the limit"}`,
			target:          &map[string]interface{}{},
			isExpectedError: true,
			expectedError:   "data too long",
		},
		{
			name: "fails with invalid JSON",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData:        `{"name": "test", "age": }`,
			target:          &map[string]interface{}{},
			isExpectedError: true,
			expectedError:   "invalid json",
		},
		{
			name: "fails with unsupported target type",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData:        `{"name": "test"}`,
			target:          func() {},
			isExpectedError: true,
			expectedError:   "not allowed",
		},
		{
			name: "validates data structure limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(5),
					WithMaxKeys(2),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData:        `{"key1": "val1", "key2": "val2", "key3": "val3"}`,
			target:          &map[string]interface{}{},
			isExpectedError: true,
			expectedError:   "invalid struct",
		},
		{
			name: "works with strict mode enabled",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithStrictSerialization(true),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData: `{"name": "test", "age": 25}`,
			target: &struct {
				Name string `json:"name"`
				Age  int    `json:"age"`
			}{},
			validateResult: func(result interface{}) bool {
				data := result.(*struct {
					Name string `json:"name"`
					Age  int    `json:"age"`
				})
				return data.Name == "test" && data.Age == 25
			},
			isExpectedError: false,
		},
		{
			name: "fails in strict mode with unknown fields",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithStrictSerialization(true),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			jsonData: `{"name": "test", "unknown_field": "value"}`,
			target: &struct {
				Name string `json:"name"`
			}{},
			isExpectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Get the JSON serializer from the server
			serializers := server.handler.Serializers()
			jsonSerializer, exists := serializers[JSON]
			assert.True(t, exists)

			err := jsonSerializer.Unmarshal([]byte(tt.jsonData), tt.target)

			if tt.isExpectedError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectedError))
				}
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					assert.True(t, tt.validateResult(tt.target))
				}
			}
		})
	}
}

func TestServer_RawMarshal(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		data            interface{}
		isExpectedError bool
		expectedError   string
		expectedResult  []byte
	}{
		{
			name: "marshals byte slice successfully",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte("hello world"),
			isExpectedError: false,
			expectedResult:  []byte("hello world"),
		},
		{
			name: "fails with non-byte-slice data",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            "string data",
			isExpectedError: true,
			expectedError:   ErrRawSerializer.Error(),
		},
		{
			name: "fails when data exceeds max binary size",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte("hello world"),
			isExpectedError: true,
			expectedError:   "data too long",
		},
		{
			name: "marshals empty byte slice",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte{},
			isExpectedError: false,
			expectedResult:  []byte{},
		},
		{
			name: "marshals binary data",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte{0x00, 0x01, 0x02, 0xFF},
			isExpectedError: false,
			expectedResult:  []byte{0x00, 0x01, 0x02, 0xFF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Get the Raw serializer from the server
			serializers := server.handler.Serializers()
			rawSerializer, exists := serializers[Raw]
			assert.True(t, exists)

			result, err := rawSerializer.Marshal(tt.data)

			if tt.isExpectedError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestServer_RawUnmarshal(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *Server
		data            []byte
		target          interface{}
		isExpectedError bool
		expectedError   string
		validateResult  func(interface{}) bool
	}{
		{
			name: "unmarshals to byte slice pointer successfully",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:   []byte("hello world"),
			target: &[]byte{},
			validateResult: func(result interface{}) bool {
				data := result.(*[]byte)
				return string(*data) == "hello world"
			},
			isExpectedError: false,
		},
		{
			name: "fails when data exceeds max binary size",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte("hello world"),
			target:          &[]byte{},
			isExpectedError: true,
			expectedError:   "data too long",
		},
		{
			name: "fails with non-byte-slice-pointer target",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte("hello world"),
			target:          &[]string{},
			isExpectedError: true,
			expectedError:   ErrRawSerializerPtr.Error(),
		},
		{
			name: "unmarshals empty data",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:   []byte{},
			target: &[]byte{},
			validateResult: func(result interface{}) bool {
				data := result.(*[]byte)
				return len(*data) == 0
			},
			isExpectedError: false,
		},
		{
			name: "unmarshals binary data",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:   []byte{0x00, 0x01, 0x02, 0xFF},
			target: &[]byte{},
			validateResult: func(result interface{}) bool {
				data := result.(*[]byte)
				return len(*data) == 4 &&
					(*data)[0] == 0x00 &&
					(*data)[1] == 0x01 &&
					(*data)[2] == 0x02 &&
					(*data)[3] == 0xFF
			},
			isExpectedError: false,
		},
		{
			name: "fails with non-pointer target",
			setupServer: func() *Server {
				server, err := NewServer(
					WithRawSerializer(),
					WithMaxBinarySize(1024),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			data:            []byte("hello world"),
			target:          []byte{},
			isExpectedError: true,
			expectedError:   ErrRawSerializerPtr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Get the Raw serializer from the server
			serializers := server.handler.Serializers()
			rawSerializer, exists := serializers[Raw]
			assert.True(t, exists)

			err := rawSerializer.Unmarshal(tt.data, tt.target)

			if tt.isExpectedError {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					assert.True(t, tt.validateResult(tt.target))
				}
			}
		})
	}
}

func TestServer_SerializationIntegration(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *Server
		operation   func(*Server) error
		expectError bool
		expectedErr string
	}{
		{
			name: "broadcasts data with validation limits",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(2),
					WithMaxKeys(3),
					WithMaxElements(5),
				)
				if err != nil {
					t.Fatal(err)
				}
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.SetHub(mockHub)
				return server
			},
			operation: func(s *Server) error {
				return s.BroadcastData(map[string]interface{}{
					"message": "hello",
					"count":   42,
				})
			},
			expectError: false,
		},
		{
			name: "fails broadcast when validation limits exceeded",
			setupServer: func() *Server {
				server, err := NewServer(
					WithJSONSerializer(),
					WithMaxDepth(1),
					WithMaxKeys(1),
				)
				if err != nil {
					t.Fatal(err)
				}
				return server
			},
			operation: func(s *Server) error {
				return s.BroadcastData(map[string]interface{}{
					"key1": "value1",
					"key2": "value2", // Exceeds MaxKeys limit
				})
			},
			expectError: true,
			expectedErr: "key length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := tt.operation(server)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.expectedErr))
				}
			} else {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.Hub().(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			}
		})
	}
}
