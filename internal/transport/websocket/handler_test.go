package websocket

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	gsErrors "github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockWebSocketConn struct {
	mock.Mock
	closed bool
}

func (m *MockWebSocketConn) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			// If no expectation was set on the mock, recover and return nil
			err = nil
		}
	}()

	args := m.Called()
	m.closed = true
	err = args.Error(0)
	return
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	args := m.Called(messageType, data)
	return args.Error(0)
}

func (m *MockWebSocketConn) ReadMessage() (messageType int, p []byte, err error) {
	args := m.Called()
	return args.Int(0), args.Get(1).([]byte), args.Error(2)
}

func TestHandler_NewHandler(t *testing.T) {
	cfg := DefaultHandlerConfig()
	handler, err := NewHandler()

	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.Config)
	assert.NotNil(t, handler.Events)
	assert.Equal(t, cfg.ConnectionPoolConfig, handler.Config.ConnectionPoolConfig)
	assert.Equal(t, cfg.MessageSize, handler.Config.MessageSize)
}

func TestHandler_DefaultHandlerConfig(t *testing.T) {
	cfg := DefaultHandlerConfig()

	assert.Equal(t, 1000, cfg.ConnectionPoolConfig.MaxTotal)
	assert.Equal(t, 10, cfg.ConnectionPoolConfig.MaxPerIP)
	assert.Equal(t, int64(512*1024), cfg.MessageSize)
	assert.Equal(t, 60*time.Second, cfg.ReadTimeout)
	assert.Equal(t, 10*time.Second, cfg.WriteTimeout)
	assert.Equal(t, 54*time.Second, cfg.PingPeriod)
	assert.Equal(t, 60*time.Second, cfg.PongWait)
	assert.NotNil(t, cfg.Serializers)
	assert.Equal(t, 2, len(cfg.Serializers)) // JSON and Raw
}

func TestHandler_ConfigCustom(t *testing.T) {
	cfg := &HandlerConfig{
		ConnectionPoolConfig: transport.ConnectionPoolConfig{MaxTotal: 500},
		MessageSize:          1024 * 1024,
		ReadTimeout:          30 * time.Second,
		WriteTimeout:         15 * time.Second,
		PingPeriod:           45 * time.Second,
		PongWait:             50 * time.Second,
		DefaultEncoding:      1, // JSON
		Serializers: map[message.EncodingType]message.Serializer{
			message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
		},
	}

	handler := &Handler{
		Config: cfg,
	}

	assert.Equal(t, 500, handler.Config.ConnectionPoolConfig.MaxTotal)
	assert.Equal(t, int64(1024*1024), handler.Config.MessageSize)
	assert.Equal(t, 30*time.Second, handler.Config.ReadTimeout)
}

func TestHandler_Stop(t *testing.T) {
	handler, err := NewHandler()
	require.NoError(t, err)

	// Not running initially
	assert.False(t, handler.running.Load())

	// Stop when not running should succeed (stopOnce)
	result := handler.Stop()
	// Stop() checks if dispatcher is nil, which it is
	assert.Nil(t, result) // or NoError if err return
}

func TestHandler_WithCustomBufferSizes(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.ReadBufferSize = 512
	cfg.WriteBufferSize = 512
	cfg.SendChanBufSize = 128

	handler := &Handler{
		Config: cfg,
	}

	assert.Equal(t, 512, handler.Config.ReadBufferSize)
	assert.Equal(t, 512, handler.Config.WriteBufferSize)
	assert.Equal(t, 128, handler.Config.SendChanBufSize)
}

func TestHandler_OriginCheck(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.CheckOrigin = func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://localhost:3000"
	}

	handler := &Handler{
		Config: cfg,
	}

	req1 := httptest.NewRequest("GET", "/ws", nil)
	req1.Header.Set("Origin", "http://localhost:3000")

	req2 := httptest.NewRequest("GET", "/ws", nil)
	req2.Header.Set("Origin", "http://evil.com")

	assert.True(t, handler.Config.CheckOrigin(req1))
	assert.False(t, handler.Config.CheckOrigin(req2))
}

func TestHandler_WithAuth(t *testing.T) {
	authFunc := func(r *http.Request) (map[string]interface{}, error) {
		return map[string]interface{}{"user_id": "123"}, nil
	}

	cfg := DefaultHandlerConfig()
	cfg.AuthFunc = authFunc

	handler := &Handler{
		Config: cfg,
	}

	assert.NotNil(t, handler.Config.AuthFunc)

	userData, err := handler.Config.AuthFunc(&http.Request{})
	assert.NoError(t, err)
	assert.Equal(t, "123", userData["user_id"])
}

func TestHandler_EventsInitialized(t *testing.T) {
	handler, err := NewHandler()
	require.NoError(t, err)

	assert.NotNil(t, handler.Events)
	// Events fields should be nil until assigned
	assert.NotNil(t, handler.Events.OnStart)
	assert.NotNil(t, handler.Events.OnConnect)
	assert.NotNil(t, handler.Events.OnDisconnect)
	assert.NotNil(t, handler.Events.OnMessage)
	assert.NotNil(t, handler.Events.OnRawMessage)
	assert.NotNil(t, handler.Events.OnJSONMessage)
	assert.NotNil(t, handler.Events.OnError)
	assert.NotNil(t, handler.Events.OnPing)
	assert.NotNil(t, handler.Events.OnPong)
}

func TestHandler_EventHandlerAssignment(t *testing.T) {
	handler, err := NewHandler()
	require.NoError(t, err)

	connectCalled := false
	handler.Events.OnConnect = func(d dispatcher.Dispatcher, ctx *Context) error {
		connectCalled = true
		return nil
	}

	assert.NotNil(t, handler.Events.OnConnect)
	assert.True(t, connectCalled == false) // not called yet
}

func TestHandler_SerializersInitialized(t *testing.T) {
	cfg := DefaultHandlerConfig()

	assert.NotNil(t, cfg.Serializers)
	assert.NotNil(t, cfg.Serializers[message.JSON])
	assert.NotNil(t, cfg.Serializers[message.Raw])
}

func TestHandler_CustomSerializerConfig(t *testing.T) {
	cfg := DefaultHandlerConfig()
	customSerializer := message.NewJSONSerializer(message.DefaultSerializerConfig())
	cfg.Serializers[message.JSON] = customSerializer

	assert.Equal(t, customSerializer, cfg.Serializers[message.JSON])
}

func TestHandler_MiddlewaresConfiguration(t *testing.T) {
	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Middleware-1", "applied")
			next.ServeHTTP(w, r)
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Middleware-2", "applied")
			next.ServeHTTP(w, r)
		})
	}

	cfg := DefaultHandlerConfig()
	cfg.Middlewares = []Middleware{middleware1, middleware2}

	handler := &Handler{
		Config: cfg,
	}

	assert.Equal(t, 2, len(handler.Config.Middlewares))
}

func TestApplyMiddlewares(t *testing.T) {
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware", "applied")
			next.ServeHTTP(w, r)
		})
	}

	cfg := DefaultHandlerConfig()
	cfg.Middlewares = []Middleware{middleware}

	handler := &Handler{
		Config: cfg,
	}

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := handler.ApplyMiddlewares(baseHandler)
	assert.NotNil(t, wrappedHandler)
}

func TestHAndler_HandlerAccessors(t *testing.T) {
	handler, err := NewHandler()
	require.NoError(t, err)

	assert.Equal(t, handler, handler.Handler())
}

func TestHandler_ConnectionPoolInitialization(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.ConnectionPoolConfig.MaxTotal = 1000
	cfg.ConnectionPoolConfig.MaxPerIP = 10

	handler := &Handler{
		Config: cfg,
	}

	handler.initConnectionPool()

	assert.NotNil(t, handler.connectionPool)
}

func TestHandler_ConnectionPoolDisabled(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.ConnectionPoolConfig.MaxTotal = 0 // disabled

	handler := &Handler{
		Config: cfg,
	}

	handler.initConnectionPool()

	assert.Nil(t, handler.connectionPool)
}

func TestHandler_ConcurrentHandlerCreation(t *testing.T) {
	var wg sync.WaitGroup
	numGoroutines := 10
	handlers := make([]*Handler, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := DefaultHandlerConfig()
			cfg.ConnectionPoolConfig.MaxTotal = (idx + 1) * 100
			handlers[idx] = &Handler{
				Config: cfg,
				Events: &Events{},
			}
		}(i)
	}

	wg.Wait()

	for i, h := range handlers {
		assert.NotNil(t, h, "Handler %d is nil", i)
		assert.NotNil(t, h.Config)
	}
}

func TestHandler_StopOnceSynchronization(t *testing.T) {
	handler, err := NewHandler()
	require.NoError(t, err)

	callCount := 0
	handler.stopOnce.Do(func() {
		callCount++
	})

	handler.stopOnce.Do(func() {
		callCount++
	})

	assert.Equal(t, 1, callCount)
}

func TestHandler_ZeroTimeoutConfiguration(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.ReadTimeout = 0
	cfg.WriteTimeout = 0

	handler := &Handler{
		Config: cfg,
	}

	assert.Equal(t, time.Duration(0), handler.Config.ReadTimeout)
}

func TestHandler_LargeMessageSize(t *testing.T) {
	cfg := DefaultHandlerConfig()
	cfg.MessageSize = 100 * 1024 * 1024 // 100MB

	handler := &Handler{
		Config: cfg,
	}

	assert.Equal(t, int64(100*1024*1024), handler.Config.MessageSize)
}

func TestHandler_ClientIdGeneratorConfiguration(t *testing.T) {
	customGenerator := func(r *http.Request, userData map[string]interface{}) (string, error) {
		return "custom-id", nil
	}

	cfg := DefaultHandlerConfig()
	cfg.ClientIdGenerator = customGenerator

	handler := &Handler{
		Config: cfg,
	}

	assert.NotNil(t, handler.Config.ClientIdGenerator)

	id, err := handler.Config.ClientIdGenerator(&http.Request{}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "custom-id", id)
}

func TestHandler_RateLimiterConfiguration(t *testing.T) {
	cfg := DefaultHandlerConfig()

	assert.NotNil(t, cfg.RateLimiter)
}

func TestHandler_EventHandlers(t *testing.T) {
	var (
		startCalled         bool
		beforeConnectCalled bool
		connectCalled       bool
		disconnectCalled    bool
		messageCalled       bool
		rawMessageCalled    bool
		jsonMessageCalled   bool
		errorCalled         bool
		pingCalled          bool
		pongCalled          bool
	)

	OnStart := func(handler OnStartFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnStart = handler
				return nil
			},
		}
	}
	OnBeforeConnect := func(handler OnBeforeConnectFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnBeforeConnect = handler
				return nil
			},
		}
	}
	OnConnect := func(handler OnConnectFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnConnect = handler
				return nil
			},
		}
	}
	OnDisconnect := func(handler OnDisconnectFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnDisconnect = handler
				return nil
			},
		}
	}
	OnMessage := func(handler OnMessageFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnMessage = handler
				return nil
			},
		}
	}
	OnRawMessage := func(handler OnRawMessageFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnRawMessage = handler
				return nil
			},
		}
	}
	OnJSONMessage := func(handler OnJSONMessageFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnJSONMessage = handler
				return nil
			},
		}
	}
	OnError := func(handler OnErrorFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnError = handler
				return nil
			},
		}
	}
	OnPing := func(handler OnPingFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnPing = handler
				return nil
			},
		}
	}
	OnPong := func(handler OnPongFunc) UniversalOption {
		return UniversalOptionFunc{
			ApplyHandlerFn: func(h *Handler) error {
				if h == nil {
					return nil
				}
				h.Events.OnPong = handler
				return nil
			},
		}
	}

	handler, err := NewHandler(
		OnStart(func(ctx *Context) error {
			startCalled = true
			return nil
		}),
		OnBeforeConnect(func(r *http.Request, ctx *Context) error {
			beforeConnectCalled = true
			return nil
		}),
		OnConnect(func(d dispatcher.Dispatcher, ctx *Context) error {
			connectCalled = true
			return nil
		}),
		OnDisconnect(func(d dispatcher.Dispatcher, ctx *Context) error {
			disconnectCalled = true
			return nil
		}),
		OnMessage(func(m *message.Message, d dispatcher.Dispatcher, ctx *Context) error {
			messageCalled = true
			return nil
		}),
		OnRawMessage(func(m []byte, d dispatcher.Dispatcher, ctx *Context) error {
			rawMessageCalled = true
			return nil
		}),
		OnJSONMessage(func(m interface{}, d dispatcher.Dispatcher, ctx *Context) error {
			jsonMessageCalled = true
			return nil
		}),
		OnError(func(err error, d dispatcher.Dispatcher, ctx *Context) error {
			errorCalled = true
			return nil
		}),
		OnPing(func(d dispatcher.Dispatcher, ctx *Context) error {
			pingCalled = true
			return nil
		}),
		OnPong(func(d dispatcher.Dispatcher, ctx *Context) error {
			pongCalled = true
			return nil
		}),
	)
	require.NoError(t, err)

	assert.NotNil(t, handler.Events.OnConnect)
	assert.True(t, connectCalled == false) // not called yet

	h := hub.NewHub(hub.DefaultHubConfig())
	dispatcher := dispatcher.NewLocalDispatcher(h, DefaultHandlerConfig().Serializers)
	client := hub.NewClient("test-client", &MockWebSocketConn{}, &hub.ConnectionInfo{}, 256)
	ctx := NewInternalContext(client, handler)

	_ = handler.Events.OnStart(ctx)
	_ = handler.Events.OnBeforeConnect(nil, ctx)
	_ = handler.Events.OnConnect(dispatcher, ctx)
	_ = handler.Events.OnDisconnect(dispatcher, ctx)
	_ = handler.Events.OnMessage(&message.Message{}, dispatcher, ctx)
	_ = handler.Events.OnRawMessage([]byte(""), dispatcher, ctx)
	_ = handler.Events.OnJSONMessage(struct{}{}, dispatcher, ctx)
	_ = handler.Events.OnError(gsErrors.NewEventFailedError("test", errors.New("test")), dispatcher, ctx)
	_ = handler.Events.OnPing(dispatcher, ctx)
	_ = handler.Events.OnPong(dispatcher, ctx)

	assert.True(t, startCalled)
	assert.True(t, beforeConnectCalled)
	assert.True(t, connectCalled)
	assert.True(t, disconnectCalled)
	assert.True(t, messageCalled)
	assert.True(t, rawMessageCalled)
	assert.True(t, jsonMessageCalled)
	assert.True(t, errorCalled)
	assert.True(t, pingCalled)
	assert.True(t, pongCalled)
}
