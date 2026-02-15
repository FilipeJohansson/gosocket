package gosocket

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/FilipeJohansson/gosocket/internal/transport/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustServer(t *testing.T, s *websocket.Server, err error) *websocket.Server {
	t.Helper()
	require.NoError(t, err)
	if s != nil {
		t.Cleanup(func() { _ = s.Stop() })
	}
	return s
}

func TestServer_WithMaxConnections(t *testing.T) {
	tests := []struct {
		name          string
		maxConns      int
		maxPerIP      int
		expectError   bool
		errorExpected error
		expected      func(*websocket.Server)
	}{
		{
			name:        "sets positive max connections",
			maxConns:    100,
			maxPerIP:    10,
			expectError: false,
			expected: func(s *websocket.Server) {
				assert.Equal(t, 100, s.Handler().Config.ConnectionPoolConfig.MaxTotal)
				assert.Equal(t, 10, s.Handler().Config.ConnectionPoolConfig.MaxPerIP)
			},
		},
		{
			name:          "sets zero max connections",
			maxConns:      0,
			expectError:   true,
			errorExpected: errors.ErrMaxConnectionsLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets negative max connections",
			maxConns:      -10,
			expectError:   true,
			errorExpected: errors.ErrMaxConnectionsLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets zero max connections per IP",
			maxConns:      100,
			maxPerIP:      0,
			expectError:   true,
			errorExpected: errors.ErrMaxConnectionsPerIPLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets negative max connections per IP",
			maxConns:      100,
			maxPerIP:      -10,
			expectError:   true,
			errorExpected: errors.ErrMaxConnectionsPerIPLessThanOne,
			expected:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxConnections(ConnectionPoolConfig{MaxTotal: tt.maxConns, MaxPerIP: tt.maxPerIP}))
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorExpected.Error())
			} else {
				server = mustServer(t, server, err)
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
		expected    func(*websocket.Server)
	}{
		{
			name:        "sets positive message size",
			size:        2048,
			expectError: false,
			expected: func(s *websocket.Server) {
				assert.Equal(t, int64(2048), s.Handler().Config.MessageSize)
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
				assert.Contains(t, err.Error(), errors.ErrMessageSizeLessThanOne.Error())
			} else {
				server = mustServer(t, server, err)
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
		expected    func(*websocket.Server)
	}{
		{
			name:        "sets read and write timeouts",
			read:        10 * time.Second,
			write:       15 * time.Second,
			expectError: false,
			expected: func(s *websocket.Server) {
				assert.Equal(t, 10*time.Second, s.Handler().Config.ReadTimeout)
				assert.Equal(t, 15*time.Second, s.Handler().Config.WriteTimeout)
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
				assert.Contains(t, err.Error(), errors.ErrTimeoutsLessThanOne.Error())
			} else {
				server = mustServer(t, server, err)
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
		expected      func(*websocket.Server)
	}{
		{
			name:       "sets ping period and pong wait",
			pingPeriod: 30 * time.Second,
			pongWait:   60 * time.Second,
			expected: func(s *websocket.Server) {
				assert.Equal(t, 30*time.Second, s.Handler().Config.PingPeriod)
				assert.Equal(t, 60*time.Second, s.Handler().Config.PongWait)
			},
		},
		{
			name:          "sets zero ping period and pong wait",
			pingPeriod:    0,
			pongWait:      0,
			isExpectError: true,
			expectError:   errors.ErrPingPongLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets negative ping period and pong wait",
			pingPeriod:    -10 * time.Second,
			pongWait:      -20 * time.Second,
			isExpectError: true,
			expectError:   errors.ErrPingPongLessThanOne,
			expected:      nil,
		},
		{
			name:          "sets pong wait less than ping period",
			pingPeriod:    10 * time.Second,
			pongWait:      5 * time.Second,
			isExpectError: true,
			expectError:   errors.ErrPongWaitLessThanPing,
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
				server = mustServer(t, server, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithCheckOrigin(t *testing.T) {
	checkOriginFn := func(r *http.Request) bool {
		return true
	}
	server, err := NewServer(WithCheckOrigin(checkOriginFn))
	server = mustServer(t, server, err)

	assert.NotNil(t, server.Handler().Config.CheckOrigin)
	assert.Equal(t, checkOriginFn(&http.Request{}), server.Handler().Config.CheckOrigin(&http.Request{}))

	checkOriginFn = func(r *http.Request) bool {
		return false
	}
	server, err = NewServer(WithCheckOrigin(checkOriginFn))
	server = mustServer(t, server, err)

	assert.NotNil(t, server.Handler().Config.CheckOrigin)
	assert.Equal(t, checkOriginFn(&http.Request{}), server.Handler().Config.CheckOrigin(&http.Request{}))
}

func TestServer_WithEncoding(t *testing.T) {
	tests := []struct {
		name     string
		encoding EncodingType
		expected func(*websocket.Server)
	}{
		{
			name:     "sets JSON encoding",
			encoding: JSON,
			expected: func(s *websocket.Server) {
				assert.Equal(t, JSON, s.Handler().Config.DefaultEncoding)
			},
		},
		{
			name:     "sets Raw encoding",
			encoding: Raw,
			expected: func(s *websocket.Server) {
				assert.Equal(t, Raw, s.Handler().Config.DefaultEncoding)
			},
		},
		{
			name:     "sets Protobuf encoding",
			encoding: message.Protobuf,
			expected: func(s *websocket.Server) {
				assert.Equal(t, message.Protobuf, s.Handler().Config.DefaultEncoding)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithEncoding(tt.encoding))
			server = mustServer(t, server, err)
			tt.expected(server)
		})
	}
}

func TestServer_WithSerializer(t *testing.T) {
	tests := []struct {
		name       string
		encoding   EncodingType
		serializer message.Serializer
		expected   func(*websocket.Server)
	}{
		{
			name:       "sets JSON serializer",
			encoding:   JSON,
			serializer: message.CreateSerializer(JSON, DefaultSerializerConfig()),
			expected: func(s *websocket.Server) {
				ser, exists := s.Handler().Config.Serializers[JSON]
				assert.True(t, exists)
				assert.IsType(t, &message.JSONSerializer{}, ser)
			},
		},
		{
			name:       "sets Protobuf serializer",
			encoding:   message.Protobuf,
			serializer: message.CreateSerializer(message.Protobuf, DefaultSerializerConfig()),
			expected: func(s *websocket.Server) {
				ser, exists := s.Handler().Config.Serializers[message.Protobuf]
				assert.True(t, exists)
				assert.IsType(t, &message.ProtobufSerializer{}, ser)
			},
		},
		{
			name:       "sets Raw serializer",
			encoding:   Raw,
			serializer: message.CreateSerializer(Raw, DefaultSerializerConfig()),
			expected: func(s *websocket.Server) {
				ser, exists := s.Handler().Config.Serializers[Raw]
				assert.True(t, exists)
				assert.IsType(t, &message.RawSerializer{}, ser)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithSerializer(tt.encoding, tt.serializer))
			server = mustServer(t, server, err)
			tt.expected(server)
		})
	}
}

func TestServer_WithJSONSerializer(t *testing.T) {
	server, err := NewServer(WithJSONSerializer())
	server = mustServer(t, server, err)

	ser, exists := server.Handler().Config.Serializers[JSON]
	assert.True(t, exists)
	assert.IsType(t, &message.JSONSerializer{}, ser)
}

func TestServer_WithRawSerializer(t *testing.T) {
	server, err := NewServer(WithRawSerializer())
	server = mustServer(t, server, err)

	ser, exists := server.Handler().Config.Serializers[Raw]
	assert.True(t, exists)
	assert.IsType(t, &message.RawSerializer{}, ser)
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
	server = mustServer(t, server, err)

	assert.Len(t, server.Handler().Config.Middlewares, 2)
}

func TestServer_WithAuth(t *testing.T) {
	authFn := func(r *http.Request) (map[string]interface{}, error) {
		token := r.Header.Get("Authorization")
		if token == "" {
			return nil, errors.ErrAuthFailure
		}
		return map[string]interface{}{"user_id": "123"}, nil
	}

	server, err := NewServer(WithAuth(authFn))
	server = mustServer(t, server, err)

	assert.NotNil(t, server.Handler().Config.AuthFunc)
}

func TestServer_WithCustomClientID(t *testing.T) {
	customClientIDFn := func(r *http.Request, userData map[string]interface{}) (string, error) {
		return "custom-client-id", nil
	}

	server, err := NewServer(WithCustomClientID(customClientIDFn))
	server = mustServer(t, server, err)

	assert.NotNil(t, server.Handler().Config.ClientIdGenerator)
	customClientIDResult, err := server.Handler().Config.ClientIdGenerator(&http.Request{}, map[string]interface{}{})
	assert.NoError(t, err)
	assert.Equal(t, "custom-client-id", customClientIDResult)

	customClientIDFn = func(r *http.Request, userData map[string]interface{}) (string, error) {
		return "", errors.ErrAuthFailure
	}

	server, err = NewServer(WithCustomClientID(customClientIDFn))
	server = mustServer(t, server, err)

	assert.NotNil(t, server.Handler().Config.ClientIdGenerator)
	_, err = server.Handler().Config.ClientIdGenerator(&http.Request{}, map[string]interface{}{})
	assert.Error(t, err)
	assert.Equal(t, errors.ErrAuthFailure, err)
}

func TestServer_WithMaxDepth(t *testing.T) {
	tests := []struct {
		name      string
		maxDepth  int
		mustError bool
		err       error
	}{
		{
			name:     "valid max depth",
			maxDepth: 10,
		},
		{
			name:      "invalid max depth",
			maxDepth:  0,
			mustError: true,
			err:       errors.ErrDepthLessThanOne,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxDepth(tt.maxDepth))
			if tt.mustError {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.maxDepth, server.Handler().Config.Serialization.MaxDepth)
			}
		})
	}
}

func TestServer_WithMaxKeys(t *testing.T) {
	tests := []struct {
		name      string
		maxKeys   int
		mustError bool
		err       error
	}{
		{
			name:    "valid max keys",
			maxKeys: 10,
		},
		{
			name:      "invalid max keys",
			maxKeys:   0,
			mustError: true,
			err:       errors.ErrMaxKeyLengthLessThanOne,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxKeys(tt.maxKeys))
			if tt.mustError {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.maxKeys, server.Handler().Config.Serialization.MaxKeys)
			}
		})
	}
}

func TestServer_WithMaxElements(t *testing.T) {
	tests := []struct {
		name        string
		maxElements int
		mustError   bool
		err         error
	}{
		{
			name:        "valid max elements",
			maxElements: 10,
		},
		{
			name:        "invalid max elements",
			maxElements: 0,
			mustError:   true,
			err:         errors.ErrMaxElementsLessThanOne,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxElements(tt.maxElements))
			if tt.mustError {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.maxElements, server.Handler().Config.Serialization.MaxElements)
			}
		})
	}
}

func TestServer_WithDisallowedTypes(t *testing.T) {
	disallowedTypes := []string{"interface", "chan"}
	server, err := NewServer(WithDisallowedTypes(disallowedTypes))
	assert.NoError(t, err)
	assert.Equal(t, server.Handler().Config.Serialization.DisallowedTypes, disallowedTypes)
}

func TestServer_WithStrictSerialization(t *testing.T) {
	server, err := NewServer(WithStrictSerialization(true))
	assert.NoError(t, err)
	assert.True(t, server.Handler().Config.Serialization.EnableStrict)

	server, err = NewServer(WithStrictSerialization(false))
	assert.NoError(t, err)
	assert.False(t, server.Handler().Config.Serialization.EnableStrict)
}

func TestServer_WithMaxBinarySize(t *testing.T) {
	tests := []struct {
		name      string
		maxSize   int64
		mustError bool
		err       error
	}{
		{
			name:    "valid max binary size",
			maxSize: 10,
		},
		{
			name:      "invalid max binary size",
			maxSize:   0,
			mustError: true,
			err:       errors.ErrMaxBinarySizeLessThanOne,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithMaxBinarySize(tt.maxSize))
			if tt.mustError {
				assert.Error(t, err)
				assert.Equal(t, tt.err, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.maxSize, server.Handler().Config.Serialization.MaxBinarySize)
			}
		})
	}
}

func TestServer_WithRelevantHeaders(t *testing.T) {
	relevantHeaders := []string{"Authorization", "Cookie", "Custom-Header"}
	server, err := NewServer(WithRelevantHeaders(relevantHeaders))
	assert.NoError(t, err)
	assert.Equal(t, server.Handler().Config.RelevantHeaders, relevantHeaders)
}

func TestServer_WithMessageBufferSize(t *testing.T) {
	messageBufferSize := 10
	server, err := NewServer(WithMessageBufferSize(messageBufferSize))
	assert.NoError(t, err)
	assert.Equal(t, server.Handler().Config.SendChanBufSize, messageBufferSize)
}

func TestServer_WithRateLimit(t *testing.T) {
	rateLimitConfig := &transport.RateLimiterConfig{
		PerClientRate:          1000,
		PerClientBurst:         1000,
		PerIPRate:              1000,
		PerIPBurst:             1000,
		CleanupInterval:        1000,
		EntryTTL:               1000,
		MaxRateLimitViolations: 1000,
	}
	server, err := NewServer(WithRateLimit(rateLimitConfig))
	assert.NoError(t, err)
	assert.Equal(t, server.Handler().Config.RateLimiter.Config(), rateLimitConfig)
}

type loggerTest struct{}

func (l *loggerTest) Log(logType LogType, level LogLevel, msg string, args ...interface{}) {
}

func TestServer_WithLogger(t *testing.T) {
	logger := &loggerTest{}
	loggerLevels := map[LogType]LogLevel{
		LogTypeServer: LogLevelInfo,
		LogTypeClient: LogLevelDebug,
		LogTypeAuth:   LogLevelInfo,
	}
	server, err := NewServer(WithLogger(logger, loggerLevels))
	assert.NoError(t, err)
	assert.Equal(t, server.Handler().Config.Logger.Logger, logger)
	assert.Equal(t, server.Handler().Config.Logger.Level, loggerLevels)
}

type memoryManagerTest struct{}

func (m *memoryManagerTest) Publish(evt *ClusterEvent) error { return nil }
func (m *memoryManagerTest) Subscribe(ctx context.Context, nodeID string) (ClusterSubscription, error) {
	return nil, nil
}

func TestServer_WithCluster(t *testing.T) {
	tests := []struct {
		name          string
		manager       ClusterManager
		nodeID        string
		expected      func(*websocket.Server, ClusterManager, string)
		isExpectError bool
		expectError   error
	}{
		{
			name:    "sets cluster manager correctly",
			manager: &memoryManagerTest{},
			nodeID:  "123",
			expected: func(s *websocket.Server, manager ClusterManager, nodeID string) {
				assert.NotNil(t, s.Handler().Config.ClusterManager)
				assert.NotNil(t, s.Handler().Config.NodeID)
				assert.Equal(t, manager, s.Handler().Config.ClusterManager)
				assert.Equal(t, nodeID, s.Handler().Config.NodeID)
			},
		},
		{
			name:          "sets cluster manager correctly when manager is nil",
			manager:       nil,
			nodeID:        "123",
			isExpectError: true,
			expectError:   errors.ErrClusterManagerNotProvided,
		},
		{
			name:    "sets cluster manager correctly when nodeID is empty",
			manager: &memoryManagerTest{},
			nodeID:  "",
			expected: func(s *websocket.Server, manager ClusterManager, _ string) {
				assert.NotNil(t, s.Handler().Config.ClusterManager)
				assert.NotNil(t, s.Handler().Config.NodeID)
				assert.Equal(t, manager, s.Handler().Config.ClusterManager)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.isExpectError {
				_, err := NewServer(WithCluster(ClusterConfig{Manager: tt.manager, NodeID: tt.nodeID}))
				assert.ErrorIs(t, err, tt.expectError)
				return
			}
			server, err := NewServer(WithCluster(ClusterConfig{Manager: tt.manager, NodeID: tt.nodeID}))
			server = mustServer(t, server, err)
			tt.expected(server, tt.manager, tt.nodeID)
		})
	}
}

func TestServer_WithPort(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		expectError bool
		expected    func(*websocket.Server)
	}{
		{
			name:        "sets port correctly",
			port:        8081,
			expectError: false,
			expected: func(s *websocket.Server) {
				assert.Equal(t, 8081, s.Config.Port)
			},
		},
		{
			name:        "sets port off range",
			port:        70000,
			expectError: true,
			expected: func(s *websocket.Server) {
				assert.Equal(t, 8080, s.Config.Port)
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
				assert.Contains(t, err.Error(), errors.ErrInvalidPort.Error())
			} else {
				server = mustServer(t, server, err)
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
		expected func(*websocket.Server)
	}{
		{
			name: "sets path correctly",
			path: "/path",
			expected: func(s *websocket.Server) {
				assert.Equal(t, "/path", s.Config.Path)
			},
		},
		{
			name: "sets empty path",
			path: "",
			expected: func(s *websocket.Server) {
				assert.Equal(t, "/ws", s.Config.Path)
			},
		},
		{
			name: "sets path without leading slash",
			path: "ws",
			expected: func(s *websocket.Server) {
				assert.Equal(t, "/ws", s.Config.Path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithPath(tt.path))
			server = mustServer(t, server, err)
			tt.expected(server)
		})
	}
}

func TestServer_WithCORS(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		expected func(*websocket.Server)
	}{
		{
			name:    "enables CORS",
			enabled: true,
			expected: func(s *websocket.Server) {
				assert.True(t, s.Config.EnableCORS)
			},
		},
		{
			name:    "disables CORS",
			enabled: false,
			expected: func(s *websocket.Server) {
				assert.False(t, s.Config.EnableCORS)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(WithCORS(tt.enabled))
			server = mustServer(t, server, err)
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
		expected    func(*websocket.Server)
	}{
		{
			name:        "sets SSL cert and key files",
			certFile:    "server.crt",
			keyFile:     "server.key",
			expectError: false,
			expected: func(s *websocket.Server) {
				assert.True(t, s.Config.EnableSSL)
				assert.Equal(t, "server.crt", s.Config.CertFile)
				assert.Equal(t, "server.key", s.Config.KeyFile)
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
				assert.Contains(t, err.Error(), errors.ErrSSLFilesRequired.Error())
			} else {
				server = mustServer(t, server, err)
				if tt.expected != nil {
					tt.expected(server)
				}
			}
		})
	}
}

func TestServer_WithProtobufSerializer(t *testing.T) {
	server, err := NewServer(WithProtobufSerializer())
	server = mustServer(t, server, err)

	ser, exists := server.Handler().Config.Serializers[message.Protobuf]
	assert.True(t, exists)
	assert.IsType(t, &message.ProtobufSerializer{}, ser)
}

func TestServer_WithDebugLogger(t *testing.T) {
	server, err := NewServer(WithDebugLogger())
	server = mustServer(t, server, err)
	require.NotNil(t, server.Handler().Config.Logger)

	levels := server.Handler().Config.Logger.Level
	assert.Equal(t, LogLevelDebug, levels[LogTypeServer])
	assert.Equal(t, LogLevelDebug, levels[LogTypeClient])
	assert.Equal(t, LogLevelError, levels[LogTypeError])
}
