// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package websocket

/**
 * handler.go implements an HTTP-compatible WebSocket handler that embeds
 * a GoSocket Runtime into an existing HTTP server.
 *
 * It upgrades incoming HTTP connections and binds them to the Runtime
 * without owning lifecycle or infrastructure responsibilities.
 *
 * This file enables embedded and platform-managed deployments.
 *
 * MUST NOT manage cluster lifecycle, runtime state, or networking outside
 * the HTTP handler context.
 */

import (
	"context"
	"encoding/json"
	e "errors"
	"fmt"
	"net/http"
	goRuntime "runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/ids"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/runtime"
	"github.com/FilipeJohansson/gosocket/internal/stats"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type Middleware func(http.Handler) http.Handler
type AuthFunc func(*http.Request) (map[string]interface{}, error)
type ClientIdGenerator func(r *http.Request, userData map[string]interface{}) (string, error)

// OnHandlerSetupFunc is called after the handler is initialized.
// The context do NOT have a client yet.
type OnStartFunc func(ctx *Context) error

// OnBeforeConnectFunc is called before a new connection is established.
// If it returns an error, the connection is rejected.
// The context do NOT have a client yet.
type OnBeforeConnectFunc func(r *http.Request, ctx *Context) error
type OnConnectFunc func(d dispatcher.Dispatcher, ctx *Context) error
type OnDisconnectFunc func(d dispatcher.Dispatcher, ctx *Context) error
type OnMessageFunc func(m *message.Message, d dispatcher.Dispatcher, ctx *Context) error // generic handler
type OnRawMessageFunc func(m []byte, d dispatcher.Dispatcher, ctx *Context) error        // raw data handler
type OnJSONMessageFunc func(m interface{}, d dispatcher.Dispatcher, ctx *Context) error  // JSON specific handler
// OnErrorFunc is called when an error occurs during any event.
// The context will only have a client after the handshake is complete.
type OnErrorFunc func(err error, d dispatcher.Dispatcher, ctx *Context) error
type OnPingFunc func(d dispatcher.Dispatcher, ctx *Context) error
type OnPongFunc func(d dispatcher.Dispatcher, ctx *Context) error

type Events struct {
	OnStart         OnStartFunc
	OnBeforeConnect OnBeforeConnectFunc
	OnConnect       OnConnectFunc
	OnDisconnect    OnDisconnectFunc
	OnMessage       OnMessageFunc
	OnRawMessage    OnRawMessageFunc
	OnJSONMessage   OnJSONMessageFunc
	OnError         OnErrorFunc
	OnPing          OnPingFunc
	OnPong          OnPongFunc
}

type Handler struct {
	Config *HandlerConfig
	Events *Events

	dispatcher dispatcher.Dispatcher
	upgrader   websocket.Upgrader

	running  atomic.Bool
	stopOnce sync.Once

	connectionPool *transport.ConnectionPool

	mu sync.RWMutex

	stats *stats.HandlerStats
}

type HandlerConfig struct {
	ReadBufferSize       int
	WriteBufferSize      int
	SendChanBufSize      int
	MessageSize          int64
	ConnectionPoolConfig transport.ConnectionPoolConfig

	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	PingPeriod        time.Duration
	PongWait          time.Duration
	ConnectionTimeout time.Duration

	// AllowedOrigins  []string
	CheckOrigin     func(r *http.Request) bool
	RelevantHeaders []string

	DefaultEncoding message.EncodingType // default message encoding
	Serialization   message.SerializationConfig

	Serializers map[message.EncodingType]message.Serializer

	AuthFunc          AuthFunc
	ClientIdGenerator ClientIdGenerator

	Middlewares []Middleware

	RateLimiter *transport.RateLimiterManager

	Logger *logger.LoggerConfig

	ClusterManager cluster.Manager
	ClusterState   store.StateStore
	NodeID         string
}

func DefaultHandlerConfig() *HandlerConfig {
	return &HandlerConfig{
		ReadBufferSize:       256,
		WriteBufferSize:      256,
		SendChanBufSize:      256,
		ConnectionPoolConfig: transport.DefaultConnectionPoolConfig(),
		MessageSize:          512 * 1024,
		ReadTimeout:          60 * time.Second,
		WriteTimeout:         10 * time.Second,
		PingPeriod:           54 * time.Second,
		PongWait:             60 * time.Second,
		ConnectionTimeout:    300 * time.Second,
		CheckOrigin:          func(r *http.Request) bool { return true },
		DefaultEncoding:      message.JSON,
		Serialization:        message.DefaultSerializerConfig(),
		Serializers: map[message.EncodingType]message.Serializer{
			message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
			message.Raw:  message.NewRawSerializer(message.DefaultSerializerConfig()),
		},
		RateLimiter: transport.NewRateLimiterManager(nil),
	}
}

func NewHandler(options ...UniversalOption) (*Handler, error) {
	h := &Handler{
		Config: DefaultHandlerConfig(),
		Events: &Events{
			OnStart:         func(ctx *Context) error { return nil },
			OnBeforeConnect: func(r *http.Request, ctx *Context) error { return nil },
			OnConnect:       func(d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnDisconnect:    func(d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnMessage:       func(m *message.Message, d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnRawMessage:    func(m []byte, d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnJSONMessage:   func(m interface{}, d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnError:         func(err error, d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnPing:          func(d dispatcher.Dispatcher, ctx *Context) error { return nil },
			OnPong:          func(d dispatcher.Dispatcher, ctx *Context) error { return nil },
		},
		mu: sync.RWMutex{},
		stats: &stats.HandlerStats{
			StartTime: time.Now(),
		},
	}

	h, err := h.with(options...)
	if err != nil {
		return nil, err
	}

	h.upgrader = NewUpgrader(&UpgraderConfig{
		ReadBufferSize:  h.Config.ReadBufferSize,
		WriteBufferSize: h.Config.WriteBufferSize,
		CheckOrigin:     h.Config.CheckOrigin,
		// Subprotocols: ,
	})

	h.initConnectionPool()

	return h, nil
}

// ServeHTTP implements the http.Handler interface.
// It is the entrypoint for handling all WebSocket requests.
// It first ensures that the hub is running, and then applies
// all configured middlewares to the request. Finally, it calls
// the configured WebSocket handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.running.Load() {
		http.Error(w, "handler stopped", http.StatusServiceUnavailable)
		return
	}

	if h.dispatcher == nil {
		http.Error(w, "dispatcher not attached", http.StatusServiceUnavailable)
		return
	}

	handler := http.Handler(
		http.HandlerFunc(h.handleWebSocket),
	)

	if len(h.Config.Middlewares) > 0 {
		handler = h.ApplyMiddlewares(handler)
	}

	handler.ServeHTTP(w, r)
}

func (h *Handler) AttachRuntime(rt *runtime.Runtime) {
	h.dispatcher = rt.Dispatcher()

	if h.Config.Logger != nil && rt != nil && rt.Hub() != nil {
		rt.Hub().Config.Logger = h.Config.Logger
	}

	//* OnStart
	ctx := NewInternalContext(nil, h)
	if err := h.Events.OnStart(ctx); err != nil {
		h.log(logger.LogTypeError, logger.LogLevelError,
			"OnStart failed: %v", err)
		return
	}

	h.running.Store(true)
}

func (h *Handler) Stop() error {
	var err error

	h.stopOnce.Do(func() {
		h.running.Store(false)

		h.mu.Lock()
		defer h.mu.Unlock()

		if h.dispatcher == nil {
			return
		}

		err = h.dispatcher.DisconnectAll()
	})

	return err
}

// ApplyMiddlewares applies all configured middlewares to the given handler.
// The middlewares are applied in reverse order of how they were added to the
// handler. If no middlewares are configured, the original handler is returned.
func (h *Handler) ApplyMiddlewares(handler http.Handler) http.Handler {
	if len(h.Config.Middlewares) == 0 {
		return handler
	}

	finalHandler := handler
	for i := len(h.Config.Middlewares) - 1; i >= 0; i-- {
		finalHandler = h.Config.Middlewares[i](finalHandler)
	}

	return finalHandler
}

func (h *Handler) Handler() *Handler { return h }

func (h *Handler) Dispatcher() dispatcher.Dispatcher { return h.dispatcher } // so handler.Dispatcher().Broadcast(...) can be done

func (h *Handler) GetClients() map[string]dispatcher.ClientDTO {
	if h.dispatcher == nil {
		return map[string]dispatcher.ClientDTO{}
	}
	return h.dispatcher.GetClients()
}

func (h *Handler) GetClientsGlobal(ctx context.Context) ([]store.ClientLocation, error) {
	if h.dispatcher == nil {
		return nil, errors.ErrDispatcherNotAvailable
	}
	return h.dispatcher.GetClientsGlobal(ctx)
}

func (h *Handler) GetStats() *stats.Stats {
	//* connection pool stats
	var connectionsPerIP map[string]int
	if h.connectionPool != nil {
		_, connectionsPerIP = h.connectionPool.GetStats()
	}

	//* system stats
	var memStats goRuntime.MemStats
	goRuntime.ReadMemStats(&memStats)

	//* hub stats
	// dispatcherStats := h.dispatcher.GetStats()

	//* handler stats
	handlerStats := h.stats.Snapshot(connectionsPerIP, h.Config.RateLimiter.GetViolations())

	return &stats.Stats{
		HandlerStats: handlerStats,
		// HubStats:         hubStats,
		AverageLatency:   h.stats.CalculateAverageLatency(),
		ActiveGoroutines: goRuntime.NumGoroutine(),
		MemoryUsage:      memStats.Alloc,
		Uptime:           time.Since(h.stats.StartTime),
		Timestamp:        time.Now(),
	}
}

func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	setupCtx := r.Context()
	if h.Config.ConnectionTimeout > 0 {
		var cancel context.CancelFunc
		setupCtx, cancel = context.WithTimeout(r.Context(), h.Config.ConnectionTimeout)
		defer cancel()
	}

	defer func() {
		if r := recover(); r != nil {
			h.log(logger.LogTypeError, logger.LogLevelError, "PANIC RECOVERED in HandleWebSocket: %v\nStack trace:\n%s\n", r, string(debug.Stack()))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}()

	//* Check HTTP method
	if r.Method != http.MethodGet {
		h.log(logger.LogTypeError, logger.LogLevelError, "method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := utils.GetIPFromRequest(r)

	//* Connection pool (IP-level)
	if h.connectionPool != nil {
		if err := h.connectionPool.Acquire(clientIP); err != nil {
			h.log(logger.LogTypeConnection, logger.LogLevelError, "connection pool error for IP %s: %v", clientIP, err)
			http.Error(w, "too many connections", http.StatusTooManyRequests)
			h.stats.IncrementRejectedConnections()
			return
		}

		defer h.connectionPool.Release(clientIP)
	}

	//* Set the CheckOrigin function if not set by the user - defaults to a function that allows all origins
	if h.upgrader.CheckOrigin == nil {
		h.upgrader.CheckOrigin = h.Config.CheckOrigin
	}

	//* Handler context (per connection)
	setupHandlerCtx := NewHandlerContextFromRequest(nil, h, r.WithContext(setupCtx))
	setupHandlerCtx.connInfo.ClientIP = clientIP
	setupHandlerCtx.connInfo.RequestID = ids.GenerateRequestID()
	defer setupHandlerCtx.cancel()

	//* Auth
	var userData map[string]interface{}
	if h.Config.AuthFunc != nil {
		h.log(logger.LogTypeAuth, logger.LogLevelInfo, "authenticating connection from %s", clientIP)
		var err error
		userData, err = h.Config.AuthFunc(r)
		if err != nil {
			h.log(logger.LogTypeAuth, logger.LogLevelError, "authentication failed for connection from %s: %v", clientIP, err)
			http.Error(w, errors.NewAuthFailureError(err).Error(), http.StatusUnauthorized)
			h.stats.IncrementAuthFailures()
			return
		}
	}

	//* Connection rate limit (IP-level)
	if h.Config.RateLimiter != nil {
		if !h.Config.RateLimiter.AllowIP(clientIP) {
			h.log(logger.LogTypeRateLimit, logger.LogLevelError, "rate limit exceeded for IP %s", clientIP)
			http.Error(w, errors.ErrTooManyRequests.Error(), http.StatusTooManyRequests)
			h.stats.IncrementRejectedConnections()
			return
		}
	}

	//* OnBeforeConnect
	if err := h.Events.OnBeforeConnect(r, setupHandlerCtx); err != nil {
		http.Error(w, "connection rejected", http.StatusForbidden)
		h.stats.IncrementRejectedConnections()
		return
	}

	//* Websocket upgrade
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log(logger.LogTypeConnection, logger.LogLevelError,
			"websocket upgrade failed: %v", err)
		h.stats.IncrementErrors(err)
		_ = h.Events.OnError(errors.NewUpgradeFailedError(err), h.dispatcher, setupHandlerCtx)
		return
	}
	h.stats.IncrementTotalConnections() // connection well-established

	//* Client ID
	var clientID string
	if h.Config.ClientIdGenerator != nil {
		if clientID, err = h.Config.ClientIdGenerator(r, userData); err != nil || clientID == "" {
			h.log(logger.LogTypeError, logger.LogLevelError, "custom client ID generator failed: %v", err)
			_ = h.Events.OnError(errors.NewClientIdGeneratorError(err), h.dispatcher, setupHandlerCtx)
			http.Error(w, errors.NewClientIdGeneratorError(err).Error(), http.StatusInternalServerError)
			_ = conn.Close()
			h.stats.IncrementErrors(err)
			return
		}
	} else {
		clientID = ids.GenerateClientID()
	}

	if strings.TrimSpace(clientID) == "" {
		h.log(logger.LogTypeError, logger.LogLevelError, "generated client ID is empty")
		_ = h.Events.OnError(errors.ErrInvalidClientId, h.dispatcher, setupHandlerCtx)
		http.Error(w, errors.ErrInvalidClientId.Error(), http.StatusInternalServerError)
		_ = conn.Close()
		h.stats.IncrementErrors(errors.ErrInvalidClientId)
		return
	}

	//* New hub Client
	client := hub.NewClient(clientID, conn, setupHandlerCtx.connInfo, h.Config.SendChanBufSize)
	for k, v := range userData {
		client.SetUserData(k, v)
	}

	//* Set read limit
	conn.SetReadLimit(h.Config.MessageSize)
	if err := conn.SetReadDeadline(time.Now().Add(h.Config.PongWait)); err != nil {
		_ = h.Events.OnError(errors.NewSetReadDeadlineError(err), h.dispatcher, setupHandlerCtx)
		_ = conn.Close()
		return
	}

	connectionHandlerCtx := NewHandlerContextFromRequest(client, h, r.WithContext(context.WithoutCancel(setupHandlerCtx.Context())))
	connectionHandlerCtx.connInfo = setupHandlerCtx.connInfo

	conn.SetPongHandler(func(string) error {
		if err := conn.SetReadDeadline(time.Now().Add(h.Config.PongWait)); err != nil {
			return errors.NewSetReadDeadlineError(err)
		}
		if err := h.Events.OnPong(h.dispatcher, connectionHandlerCtx); err != nil {
			_ = err
		}
		return nil
	})

	//* Register client in hub
	if err := h.dispatcher.RegisterClient(client); err != nil {
		_ = h.Events.OnError(err, h.dispatcher, connectionHandlerCtx)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		_ = conn.Close()
		h.stats.IncrementErrors(err)
		return
	}

	//* Start client handler
	clientHandler := NewClientHandler(h, &ClientHandlerConfig{
		Client: client,
		Conn:   conn,

		ReadTimeout:  h.Config.ReadTimeout,
		WriteTimeout: h.Config.WriteTimeout,
		PingPeriod:   h.Config.PingPeriod,
		PongWait:     h.Config.PongWait,

		Callbacks: ClientCallbacks{
			OnMessage: func(messageType message.MessageType, rawBytes []byte) {
				h.handleIncomingMessage(connectionHandlerCtx, messageType, rawBytes)
			},
			OnPing: func() {
				_ = h.Events.OnPing(h.dispatcher, connectionHandlerCtx)
			},
			OnError: func(err error) {
				h.stats.IncrementErrors(err)
				_ = h.Events.OnError(err, h.dispatcher, connectionHandlerCtx)
			},
			OnClose: func(err error) {
				h.cleanupClient(connectionHandlerCtx)
			},
			OnLog: func(logType logger.LogType, level logger.LogLevel, msg string, args ...interface{}) {
				h.log(logType, level, msg, args...)
			},
		},
	})

	//* Call OnConnect
	h.log(logger.LogTypeConnection, logger.LogLevelDebug, "Calling OnConnect for client %s", clientID)
	if err := h.Events.OnConnect(h.dispatcher, connectionHandlerCtx); err != nil {
		h.log(logger.LogTypeError, logger.LogLevelError, "Error calling OnConnect for client %s: %v", clientID, err)
		h.cleanupClient(connectionHandlerCtx)
		_ = h.Events.OnError(errors.NewEventFailedError("OnConnect", err), h.dispatcher, connectionHandlerCtx)
		_ = conn.Close()
		return
	}
	// h.stats.IncrementConnectedClients() // TODO: add on stats

	//* Start loops
	clientHandler.Start(connectionHandlerCtx)

	h.log(logger.LogTypeConnection, logger.LogLevelInfo,
		"client connected id=%s remote=%s duration=%s",
		clientID, r.RemoteAddr, time.Since(start))
}

// ===== Room Management =====

func (h *Handler) createRoom(clientID string, roomName string) (*hub.Room, error) {
	if h.dispatcher == nil {
		return nil, errors.ErrDispatcherNotAvailable
	}
	return h.dispatcher.CreateRoom(clientID, roomName)
}

func (h *Handler) joinRoom(clientID string, roomName string) error {
	if h.dispatcher == nil {
		return errors.ErrDispatcherNotAvailable
	}
	return h.dispatcher.JoinRoom(clientID, roomName)
}

func (h *Handler) leaveRoom(clientID string, roomName string) error {
	if h.dispatcher == nil {
		return errors.ErrDispatcherNotAvailable
	}
	return h.dispatcher.LeaveRoom(clientID, roomName)
}

// func (h *Handler) getRooms(clientID string) []string {
// 	if h.dispatcher == nil {
// 		return nil
// 	}
// 	return h.dispatcher.GetRooms(clientID)
// }

func (h *Handler) handleIncomingMessage(ctx *Context, msgType message.MessageType, raw []byte) {
	if ctx == nil || ctx.client == nil {
		return
	}

	h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Received message from client %s", ctx.client.GetID())
	msg := &message.Message{
		Type:    msgType,
		RawData: raw,
		From:    ctx.client.GetID(),
		//? To: do we need this?
		Created: time.Now(),
	}

	h.processMessage(msg, ctx)
}

func (h *Handler) processMessage(msg *message.Message, ctx *Context) {
	start := time.Now()
	defer func() {
		h.stats.UpdateLatencyStats(time.Since(start))
	}()

	h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Calling OnMessage for client %s", ctx.client.GetID())
	if err := h.Events.OnMessage(msg, h.dispatcher, ctx); err != nil {
		h.log(logger.LogTypeError, logger.LogLevelDebug, "Error calling OnMessage for client %s: %v", ctx.client.GetID(), err)
		h.stats.IncrementErrors(err)
		_ = h.Events.OnError(
			errors.NewEventFailedError("OnMessage", err),
			h.dispatcher,
			ctx,
		)
	}

	h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Calling OnRawMessage for client %s", ctx.client.GetID())
	if err := h.Events.OnRawMessage(msg.RawData, h.dispatcher, ctx); err != nil {
		h.log(logger.LogTypeError, logger.LogLevelDebug, "Error calling OnRawMessage for client %s: %v", ctx.client.GetID(), err)
		h.stats.IncrementErrors(err)
		_ = h.Events.OnError(
			errors.NewEventFailedError("OnRawMessage", err),
			h.dispatcher,
			ctx,
		)
	}

	h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Calling OnJSONMessage for client %s", ctx.client.GetID())
	var jsonData interface{}
	if err := json.Unmarshal(msg.RawData, &jsonData); e.Is(err, nil) {
		msg.Data = jsonData
		msg.Encoding = message.JSON
		if err := h.Events.OnJSONMessage(jsonData, h.dispatcher, ctx); err != nil {
			h.log(logger.LogTypeError, logger.LogLevelDebug, "Error calling OnJSONMessage for client %s: %v", ctx.client.GetID(), err)
			h.stats.IncrementErrors(err)
			_ = h.Events.OnError(
				errors.NewEventFailedError("OnJSONMessage", err),
				h.dispatcher,
				ctx,
			)
		}
	}

	// switch msg.Encoding {
	// case message.Raw:
	// 	msg.Data = msg.RawData

	// case message.JSON:
	// 	var data interface{}

	// 	serializer := h.Config.Serializers[message.JSON]
	// 	if serializer != nil {
	// 		if err := serializer.Unmarshal(msg.RawData, &data); err != nil {
	// 			h.log(logger.LogTypeError, logger.LogLevelDebug, "Error unmarshalling JSON for client %s: %v", ctx.client.GetID(), err)
	// 			h.stats.IncrementErrors(err)
	// 			if h.Events.OnError != nil {
	// 				_ = h.Events.OnError(err, h.dispatcher, ctx)
	// 			}
	// 			return
	// 		}
	// 	} else {
	// 		if err := json.Unmarshal(msg.RawData, &data); err != nil {
	// 			h.log(logger.LogTypeError, logger.LogLevelDebug, "Error unmarshalling JSON for client %s: %v", ctx.client.GetID(), err)
	// 			h.stats.IncrementErrors(err)
	// 			if h.Events.OnError != nil {
	// 				_ = h.Events.OnError(
	// 					errors.NewInvalidJsonError(err),
	// 					h.dispatcher,
	// 					ctx,
	// 				)
	// 			}
	// 			return
	// 		}
	// 	}

	// 	msg.Data = data

	// 	if h.Events.OnJSONMessage != nil {
	// 		h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Calling OnJSONMessage for client %s", ctx.client.GetID())
	// 		if err := h.Events.OnJSONMessage(data, h.dispatcher, ctx); err != nil {
	// 			h.log(logger.LogTypeError, logger.LogLevelDebug, "Error calling OnJSONMessage for client %s: %v", ctx.client.GetID(), err)
	// 			h.stats.IncrementErrors(err)
	// 			if h.Events.OnError != nil {
	// 				_ = h.Events.OnError(
	// 					errors.NewEventFailedError("OnJSONMessage", err),
	// 					h.dispatcher,
	// 					ctx,
	// 				)
	// 			}
	// 		}
	// 	}

	// default:
	// 	// encoder not known - ignore
	// }
}

func (h *Handler) initConnectionPool() {
	config := h.Config
	if config.ConnectionPoolConfig.MaxTotal > 0 {
		h.connectionPool = transport.NewConnectionPool(config.ConnectionPoolConfig)
		h.log(logger.LogTypeConnection, logger.LogLevelInfo,
			"Connection pool initialized: max_total=%d, max_per_ip=%d",
			config.ConnectionPoolConfig.MaxTotal, config.ConnectionPoolConfig.MaxPerIP)
	} else {
		h.log(logger.LogTypeConnection, logger.LogLevelWarn, "Connection pool disabled: MaxConnections is 0")
	}
}

func (h *Handler) cleanupClient(handlerCtx *Context) {
	client := handlerCtx.client
	defer func() {
		if r := recover(); r != nil {
			panicErr := fmt.Errorf("PANIC RECOVERED in cleanup for client %s: %v", client.GetID(), r)
			h.log(logger.LogTypeError, logger.LogLevelError,
				"%s\nStack trace:\n%s\n", panicErr.Error(), string(debug.Stack()))
			h.stats.IncrementErrors(panicErr)
		}
	}()

	h.stats.IncrementDisconnectedClients()

	if h.dispatcher != nil {
		h.log(logger.LogTypeClient, logger.LogLevelDebug, "Removing client %s from hub", client.GetID())
		err := h.dispatcher.UnregisterClient(client.GetID())
		if err != nil {
			h.log(logger.LogTypeError, logger.LogLevelError, "Error removing client %s from hub: %v", client.GetID(), err)
			h.stats.IncrementErrors(err)
			_ = h.Events.OnError(err, h.dispatcher, handlerCtx)
			return
		}
	}

	if client.Conn != nil {
		if conn, ok := client.Conn.(*websocket.Conn); ok {
			h.log(logger.LogTypeClient, logger.LogLevelDebug, "Closing client %s connection", client.GetID())
			_ = conn.Close()
		}
	}

	h.log(logger.LogTypeClient, logger.LogLevelDebug, "Calling OnDisconnect for client %s", client.GetID())
	_ = h.Events.OnDisconnect(h.dispatcher, handlerCtx)
}

func (h *Handler) with(options ...UniversalOption) (*Handler, error) {
	for _, o := range options {
		if err := o.applyHandler(h); err != nil {
			h.log(logger.LogTypeServer, logger.LogLevelError, "Failed to apply option: %s", err.Error())
			return nil, err
		}
	}

	return h, nil
}

func (h *Handler) log(t logger.LogType, l logger.LogLevel, msg string, args ...interface{}) {
	conf := h.Config.Logger
	if conf == nil {
		return
	}

	lvl, ok := conf.Level[t]
	if !ok {
		lvl = logger.LogLevelNone
	}

	if l <= lvl {
		conf.Logger.Log(t, l, msg, args...)
	}
}

func (o UniversalOptionFunc) applyHandler(h *Handler) error {
	if o.ApplyHandlerFn != nil {
		return o.ApplyHandlerFn(h)
	}
	return nil
}
