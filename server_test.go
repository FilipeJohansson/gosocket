package gosocket

import (
	"fmt"
	"net/http"
	"testing"
	"time"

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
			server := NewServer()
			tt.expected(server)
		})
	}
}

func TestServer_WithPort(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		config   *ServerConfig
		expected func(*Server)
	}{
		{
			name: "sets port correctly",
			port: 8080,
			expected: func(s *Server) {
				assert.Equal(t, 8080, s.config.Port)
			},
		},
		{
			name: "sets port off range",
			port: 70000,
			expected: func(s *Server) {
				assert.Equal(t, 8080, s.config.Port)
			},
		},
		{
			name: "sets negative port",
			port: -1,
			expected: func(s *Server) {
				assert.Equal(t, 8080, s.config.Port)
			},
		},
		{
			name: "sets zero port",
			port: 0,
			expected: func(s *Server) {
				assert.Equal(t, 8080, s.config.Port)
			},
		},
		{
			name:   "should set config to default when nil",
			port:   8081,
			config: nil,
			expected: func(s *Server) {
				assert.NotNil(t, s.config)
				assert.Equal(t, 8081, s.config.Port)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer()
			server.config = tt.config // Override config if provided
			server.WithPort(tt.port)
			tt.expected(server)
		})
	}
}

func TestServer_WithPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		config   *ServerConfig
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
			server := NewServer()
			server.config = tt.config // Override config if provided
			server.WithPath(tt.path)
			tt.expected(server)
		})
	}
}

func TestServer_WithCORS(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		config   *ServerConfig
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
			server := NewServer()
			server.config = tt.config // Override config if provided
			server.WithCORS(tt.enabled)
			tt.expected(server)
		})
	}
}

func TestServer_WithSSL(t *testing.T) {
	tests := []struct {
		name     string
		certFile string
		keyFile  string
		config   *ServerConfig
		expected func(*Server)
	}{
		{
			name:     "sets SSL cert and key files",
			certFile: "server.crt",
			keyFile:  "server.key",
			expected: func(s *Server) {
				assert.True(t, s.config.EnableSSL)
				assert.Equal(t, "server.crt", s.config.CertFile)
				assert.Equal(t, "server.key", s.config.KeyFile)
			},
		},
		{
			name:     "sets empty cert and key files",
			certFile: "",
			keyFile:  "",
			expected: func(s *Server) {
				assert.False(t, s.config.EnableSSL)
				assert.Equal(t, "", s.config.CertFile)
				assert.Equal(t, "", s.config.KeyFile)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer()
			server.config = tt.config // Override config if provided
			server.WithSSL(tt.certFile, tt.keyFile)
			tt.expected(server)
		})
	}
}

func TestServer_WithMaxConnections(t *testing.T) {
	tests := []struct {
		name     string
		maxConns int
		expected func(*Server)
	}{
		{
			name:     "sets positive max connections",
			maxConns: 100,
			expected: func(s *Server) {
				assert.Equal(t, 100, s.handler.config.MaxConnections)
			},
		},
		{
			name:     "sets zero max connections",
			maxConns: 0,
			expected: func(s *Server) {
				assert.Equal(t, 1000, s.handler.config.MaxConnections)
			},
		},
		{
			name:     "sets negative max connections",
			maxConns: -10,
			expected: func(s *Server) {
				assert.Equal(t, 1000, s.handler.config.MaxConnections)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithMaxConnections(tt.maxConns)
			tt.expected(server)
		})
	}
}

func TestServer_WithMessageSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected func(*Server)
	}{
		{
			name: "sets positive message size",
			size: 2048,
			expected: func(s *Server) {
				assert.Equal(t, int64(2048), s.handler.config.MessageSize)
			},
		},
		{
			name: "sets zero message size",
			size: 0,
			expected: func(s *Server) {
				assert.Equal(t, int64(1024), s.handler.config.MessageSize)
			},
		},
		{
			name: "sets negative message size",
			size: -100,
			expected: func(s *Server) {
				assert.Equal(t, int64(1024), s.handler.config.MessageSize)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithMessageSize(tt.size)
			tt.expected(server)
		})
	}
}

func TestServer_WithTimeout(t *testing.T) {
	tests := []struct {
		name     string
		read     time.Duration
		write    time.Duration
		expected func(*Server)
	}{
		{
			name:  "sets read and write timeouts",
			read:  10 * time.Second,
			write: 15 * time.Second,
			expected: func(s *Server) {
				assert.Equal(t, 10*time.Second, s.handler.config.ReadTimeout)
				assert.Equal(t, 15*time.Second, s.handler.config.WriteTimeout)
			},
		},
		{
			name:  "sets zero timeouts",
			read:  0,
			write: 0,
			expected: func(s *Server) {
				assert.Equal(t, 0*time.Second, s.handler.config.ReadTimeout)
				assert.Equal(t, 0*time.Second, s.handler.config.WriteTimeout)
			},
		},
		{
			name:  "sets negative timeouts",
			read:  -5 * time.Second,
			write: -10 * time.Second,
			expected: func(s *Server) {
				assert.Equal(t, 0*time.Second, s.handler.config.ReadTimeout)
				assert.Equal(t, 0*time.Second, s.handler.config.WriteTimeout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithTimeout(tt.read, tt.write)
			tt.expected(server)
		})
	}
}

func TestServer_WithPingPong(t *testing.T) {
	tests := []struct {
		name       string
		pingPeriod time.Duration
		pongWait   time.Duration
		expected   func(*Server)
	}{
		{
			name:       "sets ping period and pong wait",
			pingPeriod: 30 * time.Second,
			pongWait:   60 * time.Second,
			expected: func(s *Server) {
				assert.Equal(t, 30*time.Second, s.handler.config.PingPeriod)
				assert.Equal(t, 60*time.Second, s.handler.config.PongWait)
			},
		},
		{
			name:       "sets zero ping period and pong wait",
			pingPeriod: 0,
			pongWait:   0,
			expected: func(s *Server) {
				assert.Equal(t, 0*time.Second, s.handler.config.PingPeriod)
				assert.Equal(t, 0*time.Second, s.handler.config.PongWait)
			},
		},
		{
			name:       "sets negative ping period and pong wait",
			pingPeriod: -10 * time.Second,
			pongWait:   -20 * time.Second,
			expected: func(s *Server) {
				assert.Equal(t, 0*time.Second, s.handler.config.PingPeriod)
				assert.Equal(t, 0*time.Second, s.handler.config.PongWait)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithPingPong(tt.pingPeriod, tt.pongWait)
			tt.expected(server)
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
				assert.Equal(t, []string{"http://example.com", "http://localhost"}, s.handler.config.AllowedOrigins)
			},
		},
		{
			name:    "sets empty allowed origins",
			origins: []string{},
			expected: func(s *Server) {
				assert.Empty(t, s.handler.config.AllowedOrigins)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithAllowedOrigins(tt.origins)
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
				assert.Equal(t, JSON, s.handler.config.DefaultEncoding)
			},
		},
		{
			name:     "sets Raw encoding",
			encoding: Raw,
			expected: func(s *Server) {
				assert.Equal(t, Raw, s.handler.config.DefaultEncoding)
			},
		},
		{
			name:     "sets Protobuf encoding",
			encoding: Protobuf,
			expected: func(s *Server) {
				assert.Equal(t, Protobuf, s.handler.config.DefaultEncoding)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithEncoding(tt.encoding)
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
			serializer: JSONSerializer{},
			expected: func(s *Server) {
				ser, exists := s.handler.serializers[JSON]
				assert.True(t, exists)
				assert.IsType(t, JSONSerializer{}, ser)
			},
		},
		{
			name:       "sets Protobuf serializer",
			encoding:   Protobuf,
			serializer: ProtobufSerializer{},
			expected: func(s *Server) {
				ser, exists := s.handler.serializers[Protobuf]
				assert.True(t, exists)
				assert.IsType(t, ProtobufSerializer{}, ser)
			},
		},
		{
			name:       "sets Raw serializer",
			encoding:   Raw,
			serializer: RawSerializer{},
			expected: func(s *Server) {
				ser, exists := s.handler.serializers[Raw]
				assert.True(t, exists)
				assert.IsType(t, RawSerializer{}, ser)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer().WithSerializer(tt.encoding, tt.serializer)
			tt.expected(server)
		})
	}
}

func TestServer_WithJSONSerializer(t *testing.T) {
	server := NewServer().WithJSONSerializer()

	ser, exists := server.handler.serializers[JSON]
	assert.True(t, exists)
	assert.IsType(t, JSONSerializer{}, ser)
}

func TestServer_WithRawSerializer(t *testing.T) {
	server := NewServer().WithRawSerializer()

	ser, exists := server.handler.serializers[Raw]
	assert.True(t, exists)
	assert.IsType(t, RawSerializer{}, ser)
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

	server := NewServer().
		WithMiddleware(middleware1).
		WithMiddleware(middleware2)

	assert.Len(t, server.handler.middlewares, 2)
}

func TestServer_WithAuth(t *testing.T) {
	authFunc := func(r *http.Request) (map[string]interface{}, error) {
		token := r.Header.Get("Authorization")
		if token == "" {
			return nil, fmt.Errorf("missing authorization header")
		}
		return map[string]interface{}{"user_id": "123"}, nil
	}

	server := NewServer().WithAuth(authFunc)

	assert.NotNil(t, server.handler.authFunc)
}

func TestServer_EventHandlers(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*Server)
		validate func(*Server)
	}{
		{
			name: "OnConnect handler",
			setup: func(s *Server) {
				s.OnConnect(func(c *Client) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnConnect)
			},
		},
		{
			name: "OnDisconnect handler",
			setup: func(s *Server) {
				s.OnDisconnect(func(c *Client) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnDisconnect)
			},
		},
		{
			name: "OnMessage handler",
			setup: func(s *Server) {
				s.OnMessage(func(c *Client, m *Message) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnMessage)
			},
		},
		{
			name: "OnRawMessage handler",
			setup: func(s *Server) {
				s.OnRawMessage(func(c *Client, data []byte) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnRawMessage)
			},
		},
		{
			name: "OnJSONMessage handler",
			setup: func(s *Server) {
				s.OnJSONMessage(func(c *Client, data interface{}) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnJSONMessage)
			},
		},
		{
			name: "OnProtobufMessage handler",
			setup: func(s *Server) {
				s.OnProtobufMessage(func(c *Client, data interface{}) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnProtobufMessage)
			},
		},
		{
			name: "OnError handler",
			setup: func(s *Server) {
				s.OnError(func(c *Client, err error) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnError)
			},
		},
		{
			name: "OnPing handler",
			setup: func(s *Server) {
				s.OnPing(func(c *Client) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnPing)
			},
		},
		{
			name: "OnPong handler",
			setup: func(s *Server) {
				s.OnPong(func(c *Client) error { return nil })
			},
			validate: func(s *Server) {
				assert.NotNil(t, s.handler.handlers.OnPong)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer()
			tt.setup(server)
			tt.validate(server)
		})
	}
}

func TestServer_Broadcast(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		message       []byte
		expectedError string
	}{
		{
			name: "broadcasts message successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			message:       []byte("test message"),
			expectedError: "",
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server := NewServer()
				server.handler = nil
				return server
			},
			message:       []byte("test message"),
			expectedError: "server not properly initialized",
		},
		{
			name: "fails when hub is nil",
			setupServer: func() *Server {
				server := NewServer()
				server.handler.hub = nil
				return server
			},
			message:       []byte("test message"),
			expectedError: "server not properly initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.Broadcast(tt.message)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastMessage(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		message       *Message
		expectedError string
	}{
		{
			name: "broadcasts message successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			message:       NewMessage(TextMessage, "test"),
			expectedError: "",
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server := NewServer()
				server.handler = nil
				return server
			},
			message:       NewMessage(TextMessage, "test"),
			expectedError: "server not properly initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastMessage(tt.message)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastJSON(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		data          interface{}
		expectedError string
	}{
		{
			name: "broadcasts JSON successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			data:          map[string]string{"key": "value"},
			expectedError: "",
		},
		{
			name: "fails with invalid JSON data",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				server.handler.hub = mockHub
				return server
			},
			data:          make(chan int), // channels can't be marshaled to JSON
			expectedError: "failed to marshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastJSON(tt.data)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastToRoom(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		room          string
		message       []byte
		expectedError string
	}{
		{
			name: "broadcasts to room successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastToRoom", "test-room", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			room:          "test-room",
			message:       []byte("test message"),
			expectedError: "",
		},
		{
			name: "fails when server not initialized",
			setupServer: func() *Server {
				server := NewServer()
				server.handler = nil
				return server
			},
			room:          "test-room",
			message:       []byte("test message"),
			expectedError: "server not properly initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastToRoom(tt.room, tt.message)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastToRoomJSON(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		room          string
		data          interface{}
		expectedError string
	}{
		{
			name: "broadcasts JSON to room successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastToRoom", "test-room", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			room:          "test-room",
			data:          map[string]string{"key": "value"},
			expectedError: "",
		},
		{
			name: "fails with invalid JSON data",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				server.handler.hub = mockHub
				return server
			},
			room:          "test-room",
			data:          make(chan int),
			expectedError: "failed to marshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastToRoomJSON(tt.room, tt.data)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
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
				server := NewServer()
				server.handler.hub = nil
				return server
			},
			expected: 0,
		},
		{
			name: "returns clients from hub",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()

				// Add some mock clients
				client1 := NewClient("client1", &MockWebSocketConn{}, hub)
				client2 := NewClient("client2", &MockWebSocketConn{}, hub)

				hub.Clients = map[*Client]bool{
					client1: true,
					client2: true,
				}

				server.handler.hub = hub
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
				server := NewServer()
				hub := NewHub()

				client := NewClient("test-client", &MockWebSocketConn{}, hub)
				hub.Clients = map[*Client]bool{client: true}

				server.handler.hub = hub
				return server
			},
			clientID:    "test-client",
			expectFound: true,
		},
		{
			name: "returns nil for non-existing client",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()
				server.handler.hub = hub
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
				server := NewServer()
				server.handler.hub = nil
				return server
			},
			expected: 0,
		},
		{
			name: "returns correct client count",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()

				// Add some mock clients
				client1 := NewClient("client1", &MockWebSocketConn{}, hub)
				client2 := NewClient("client2", &MockWebSocketConn{}, hub)
				client3 := NewClient("client3", &MockWebSocketConn{}, hub)

				hub.Clients = map[*Client]bool{
					client1: true,
					client2: true,
					client3: true,
				}

				server.handler.hub = hub
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
		name          string
		setupServer   func() *Server
		operation     string
		roomName      string
		expectedError string
	}{
		{
			name: "creates room successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", "test-room").Return(nil)
				server.handler.hub = mockHub
				return server
			},
			operation:     "create",
			roomName:      "test-room",
			expectedError: "",
		},
		{
			name: "fails to create room with empty name",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", "").Return(fmt.Errorf("room name cannot be empty"))
				server.handler.hub = mockHub
				return server
			},
			operation:     "create",
			roomName:      "",
			expectedError: "room name cannot be empty",
		},
		{
			name: "deletes room successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("CreateRoom", "test-room").Return(nil)
				mockHub.On("DeleteRoom", "test-room").Return(nil)
				server.handler.hub = mockHub
				server.CreateRoom("test-room")
				return server
			},
			operation:     "delete",
			roomName:      "test-room",
			expectedError: "",
		},
		{
			name: "fails to delete non-existing room",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.On("DeleteRoom", "non-existing").Return(fmt.Errorf("room not found: non-existing"))
				server.handler.hub = mockHub
				return server
			},
			operation:     "delete",
			roomName:      "non-existing",
			expectedError: "room not found: non-existing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			var err error
			switch tt.operation {
			case "create":
				err = server.CreateRoom(tt.roomName)
			case "delete":
				err = server.DeleteRoom(tt.roomName)
			}

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}

			if mockHub, ok := server.handler.hub.(*MockHub); ok {
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
				server := NewServer()
				server.handler.hub = nil
				return server
			},
			expected: []string{},
		},
		{
			name: "returns room names",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()

				// Create some rooms
				hub.Rooms = map[string]map[*Client]bool{
					"room1": {},
					"room2": {},
					"room3": {},
				}

				server.handler.hub = hub
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
		name          string
		setupServer   func() *Server
		operation     string
		clientID      string
		room          string
		expectedError string
	}{
		{
			name: "joins room successfully",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()

				client := NewClient("test-client", &MockWebSocketConn{}, hub)
				hub.Clients = map[*Client]bool{client: true}

				server.handler.hub = hub
				return server
			},
			operation:     "join",
			clientID:      "test-client",
			room:          "test-room",
			expectedError: "",
		},
		{
			name: "fails to join room with non-existing client",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()
				server.handler.hub = hub
				return server
			},
			operation:     "join",
			clientID:      "non-existing",
			room:          "test-room",
			expectedError: "client not found: non-existing",
		},
		{
			name: "leaves room successfully",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()

				client := NewClient("test-client", &MockWebSocketConn{}, hub)
				hub.Clients = map[*Client]bool{client: true}

				server.handler.hub = hub
				return server
			},
			operation:     "leave",
			clientID:      "test-client",
			room:          "test-room",
			expectedError: "",
		},
		{
			name: "fails to leave room with non-existing client",
			setupServer: func() *Server {
				server := NewServer()
				hub := NewHub()
				server.handler.hub = hub
				return server
			},
			operation:     "leave",
			clientID:      "non-existing",
			room:          "test-room",
			expectedError: "client not found: non-existing",
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

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_DisconnectClient(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		clientID      string
		expectedError string
	}{
		{
			name: "disconnects client successfully",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()

				mockConn := &MockWebSocketConn{}
				mockConn.On("Close").Return(nil)

				client := NewClient("test-client", mockConn, mockHub)

				// Mock the hub methods that will be called during disconnect
				mockHub.On("RemoveClient", client).Return()
				mockHub.Clients = map[*Client]bool{client: true}

				server.handler.hub = mockHub
				return server
			},
			clientID:      "test-client",
			expectedError: "",
		},
		{
			name: "fails to disconnect non-existing client",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				mockHub.Clients = map[*Client]bool{} // Empty clients map
				server.handler.hub = mockHub
				return server
			},
			clientID:      "non-existing",
			expectedError: "client not found: non-existing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.DisconnectClient(tt.clientID)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_Stop(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		expectedError string
	}{
		{
			name: "fails when server is not running",
			setupServer: func() *Server {
				server := NewServer()
				server.isRunning = false
				return server
			},
			expectedError: "server is not running",
		},
		{
			name: "fails when server is nil",
			setupServer: func() *Server {
				server := NewServer()
				server.isRunning = true
				server.server = nil
				return server
			},
			expectedError: "server is not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.Stop()

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastData(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		data          interface{}
		expectedError string
	}{
		{
			name: "broadcasts data with JSON serializer",
			setupServer: func() *Server {
				server := NewServer().WithJSONSerializer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			data:          map[string]string{"key": "value"},
			expectedError: "",
		},
		{
			name: "falls back to JSON when no serializer",
			setupServer: func() *Server {
				server := NewServer()
				server.handler.config.DefaultEncoding = Raw // No serializer for Raw
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			data:          map[string]string{"key": "value"},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastData(tt.data)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestServer_BroadcastDataWithEncoding(t *testing.T) {
	tests := []struct {
		name          string
		setupServer   func() *Server
		data          interface{}
		encoding      EncodingType
		expectedError string
	}{
		{
			name: "broadcasts with JSON encoding",
			setupServer: func() *Server {
				server := NewServer().WithJSONSerializer()
				mockHub := NewMockHub()
				mockHub.On("BroadcastMessage", mock.AnythingOfType("*gosocket.Message"))
				server.handler.hub = mockHub
				return server
			},
			data:          map[string]string{"key": "value"},
			encoding:      JSON,
			expectedError: "",
		},
		{
			name: "fails with unsupported encoding",
			setupServer: func() *Server {
				server := NewServer()
				mockHub := NewMockHub()
				server.handler.hub = mockHub
				return server
			},
			data:          "test data",
			encoding:      EncodingType(999),
			expectedError: "serializer not found for encoding: 999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			err := server.BroadcastDataWithEncoding(tt.data, tt.encoding)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				if mockHub, ok := server.handler.hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}
