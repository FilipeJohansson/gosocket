// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type OnBeforeConnectFunc func(r *http.Request, ctx *Context) error
type OnConnectFunc func(c *Client, ctx *Context) error
type OnDisconnectFunc func(c *Client, ctx *Context) error
type OnMessageFunc func(c *Client, m *Message, ctx *Context) error            // generic handler
type OnRawMessageFunc func(c *Client, m []byte, ctx *Context) error           // raw data handler
type OnJSONMessageFunc func(c *Client, m interface{}, ctx *Context) error     // JSON specific handler
type OnProtobufMessageFunc func(c *Client, m interface{}, ctx *Context) error // Protobuf specific handler
type OnErrorFunc func(c *Client, err error, ctx *Context) error
type OnPingFunc func(c *Client, ctx *Context) error
type OnPongFunc func(c *Client, ctx *Context) error

type Events struct {
	OnBeforeConnect   OnBeforeConnectFunc
	OnConnect         OnConnectFunc
	OnDisconnect      OnDisconnectFunc
	OnMessage         OnMessageFunc
	OnRawMessage      OnRawMessageFunc
	OnJSONMessage     OnJSONMessageFunc
	OnProtobufMessage OnProtobufMessageFunc
	OnError           OnErrorFunc
	OnPing            OnPingFunc
	OnPong            OnPongFunc
}

type Handler struct {
	hub         IHub
	config      *HandlerConfig
	events      *Events
	serializers map[EncodingType]Serializer

	authFunc          AuthFunc
	clientIdGenerator ClientIdGenerator

	upgrader    websocket.Upgrader
	hubRunning  sync.Once
	middlewares []Middleware
	mu          sync.RWMutex

	connectionPool *ConnectionPool
	logger         *LoggerConfig
	rateLimiter    *RateLimiterManager
}

// NewHandler returns a new instance of Handler with default configuration.
// The default configuration is to accept up to 1000 connections and to set the
// read and write buffers to 1024 bytes. The default encoding is JSON, but you can
// change it by calling the WithEncoding method.
func NewHandler(options ...UniversalOption) (*Handler, error) {
	rl := NewRateLimiterManager(DefaultRateLimiterConfig())
	l := DefaultLoggerConfig()

	h := &Handler{
		hub:         NewHub(l),
		config:      DefaultHandlerConfig(),
		events:      &Events{},
		serializers: make(map[EncodingType]Serializer),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		mu: sync.RWMutex{},

		logger:      l,
		rateLimiter: rl,
	}

	for _, o := range options {
		if err := o(h); err != nil {
			return nil, err
		}
	}

	h.initConnectionPool()

	return h, nil
}

func (h *Handler) Config() HandlerConfig {
	return *h.config
}

func (h *Handler) Hub() IHub {
	return h.hub
}

func (h *Handler) Serializers() map[EncodingType]Serializer {
	return h.serializers
}

func (h *Handler) Middlewares() []Middleware {
	return h.middlewares
}

func (h *Handler) Handlers() *Events {
	return h.events
}

func (h *Handler) AuthFunc() AuthFunc {
	return h.authFunc
}

func (h *Handler) ClientIdGenerator() ClientIdGenerator {
	return h.clientIdGenerator
}

func (h *Handler) AddSerializer(encoding EncodingType, serializer Serializer) {
	h.serializers[encoding] = serializer
}

func (h *Handler) SetHub(hub IHub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hub = hub
}

func (h *Handler) SetDefaultEncoding(encoding EncodingType) {
	h.config.DefaultEncoding = encoding
}

func (h *Handler) Handler() *Handler {
	return h
}

// ===== CONTROLLERS =====

// ServeHTTP implements the http.Handler interface.
// It is the entrypoint for handling all WebSocket requests.
// It first ensures that the hub is running, and then applies
// all configured middlewares to the request. Finally, it calls
// the configured WebSocket handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ensureHubRunning(context.Background())

	if len(h.middlewares) > 0 {
		wsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.HandleWebSocket(w, r)
		})

		// apply middlewares and run
		finalHandler := h.ApplyMiddlewares(wsHandler)
		finalHandler.ServeHTTP(w, r)
	} else {
		// no middlewares, just run directly
		h.HandleWebSocket(w, r)
	}
}

// ApplyMiddlewares applies all configured middlewares to the given handler.
// The middlewares are applied in reverse order of how they were added to the
// handler. If no middlewares are configured, the original handler is returned.
func (h *Handler) ApplyMiddlewares(handler http.Handler) http.Handler {
	if len(h.middlewares) == 0 {
		return handler
	}

	finalHandler := handler
	for i := len(h.middlewares) - 1; i >= 0; i-- {
		finalHandler = h.middlewares[i](finalHandler)
	}

	return finalHandler
}

// HandleWebSocket is the HTTP handler that is registered for the WebSocket endpoint. It upgrades the connection to a WebSocket connection,
// authenticates the user using the authFunc, and then adds the client to the hub and starts the client's read and write goroutines.
// It also calls the OnConnect handler if it is set.
//
// This method is safe to call concurrently.
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	setupCtx := r.Context()

	if h.config.ConnectionTimeout > 0 {
		var cancel context.CancelFunc
		setupCtx, cancel = context.WithTimeout(r.Context(), h.config.ConnectionTimeout)
		defer cancel()
	}

	defer func() {
		if r := recover(); r != nil {
			h.log(LogTypeError, LogLevelError, "PANIC RECOVERED in HandleWebSocket: %v\nStack trace:\n%s\n", r, string(debug.Stack()))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}()

	clientIP := getClientIPFromRequest(r)

	if h.connectionPool != nil {
		if err := h.connectionPool.AcquireConnection(clientIP); err != nil {
			h.log(LogTypeConnection, LogLevelWarn, "Connection rejected for %s: %v", clientIP, err)
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
			return
		}

		defer h.connectionPool.ReleaseConnection(clientIP)
	}

	if h.upgrader.CheckOrigin == nil {
		h.upgrader.CheckOrigin = func(r *http.Request) bool {
			if len(h.config.AllowedOrigins) == 0 {
				return true
			}

			origin := r.Header.Get("Origin")
			for _, allowed := range h.config.AllowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		}
	}

	var userData map[string]interface{}
	if h.authFunc != nil {
		h.log(LogTypeAuth, LogLevelDebug, "Authenticating user: %s", r.RemoteAddr)
		var err error
		userData, err = h.authFunc(r)
		if err != nil {
			h.log(LogTypeAuth, LogLevelError, "Authentication failed: %v", err)
			http.Error(w, newAuthFailureError(err).Error(), http.StatusUnauthorized)
			return
		}
	}

	setupHandlerCtx := NewHandlerContextFromRequest(h, r.WithContext(setupCtx))
	setupHandlerCtx.connInfo.ClientIP = clientIP
	setupHandlerCtx.connInfo.RequestID = generateRequestID()

	if !h.rateLimiter.AllowIP(r.RemoteAddr) {
		http.Error(w, ErrTooManyRequests.Error(), http.StatusTooManyRequests)
		return
	}

	if h.events.OnBeforeConnect != nil {
		if err := h.events.OnBeforeConnect(r, setupHandlerCtx); err != nil {
			http.Error(w, fmt.Sprintf("connection rejected: %v", err), http.StatusBadRequest)
			return
		}
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.events.OnError != nil {
			_ = h.events.OnError(nil, newUpgradeFailedError(err), setupHandlerCtx)
		}
		return
	}

	var clientId string
	if h.clientIdGenerator != nil {
		if clientId, err = h.clientIdGenerator(r, userData); err != nil {
			h.log(LogTypeError, LogLevelError, "Custom client ID generation failed: %v", err)
			http.Error(w, newClientIdGeneratorError(err).Error(), http.StatusInternalServerError)
		}
	} else {
		clientId = GenerateClientID()
	}

	if strings.TrimSpace(clientId) == "" {
		h.log(LogTypeError, LogLevelError, "Client ID cannot be empty")
		http.Error(w, ErrInvalidClientId.Error(), http.StatusInternalServerError)
		return
	}

	client := NewClient(clientId, conn, h.hub, h.config.MessageChanBufSize)
	client.ConnInfo = setupHandlerCtx.connInfo

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(h.config.MessageSize)

	if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
		if h.events.OnError != nil {
			_ = h.events.OnError(nil, newSetReadDeadlineError(err), setupHandlerCtx)
		}
		_ = conn.Close()
		return
	}

	connectionHandlerCtx := NewHandlerContextFromRequest(h, r.WithContext(context.WithoutCancel(setupHandlerCtx.Context())))
	connectionHandlerCtx.connInfo = setupHandlerCtx.connInfo

	conn.SetPongHandler(func(string) error {
		if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
			return newSetReadDeadlineError(err)
		}
		if h.events.OnPong != nil {
			if err := h.events.OnPong(client, connectionHandlerCtx); err != nil {
				_ = err
			}
		}
		return nil
	})

	h.hub.AddClient(client)

	if h.events.OnConnect != nil {
		if err := h.events.OnConnect(client, connectionHandlerCtx); err != nil {
			h.log(LogTypeError, LogLevelError, "OnConnect handler error: %v", err)
			h.hub.RemoveClient(client)
			_ = conn.Close()
			connectionHandlerCtx.Cancel()
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnConnect", err), connectionHandlerCtx)
			}
			return
		}
		h.log(LogTypeConnection, LogLevelInfo, "%s connected", client.ID)
	}

	safeGoroutine("ClientWrite", func() {
		h.handleClientWrite(client, connectionHandlerCtx)
	})

	safeGoroutine("ClientRead", func() {
		h.handleClientRead(client, connectionHandlerCtx)
	})
}

func (h *Handler) GetConnectionStats() (int, map[string]int) {
	if h.connectionPool == nil {
		return 0, nil
	}
	return h.connectionPool.GetStats()
}

// handleClientWrite is a goroutine that writes messages to a client. It reads
// from the client's MessageChan and writes to the client's websocket connection.
// If the MessageChan is closed, it sends a CloseMessage to the client and
// returns. It also sends a PingMessage every h.config.PingPeriod to the
// client. If the client's websocket connection is closed or an error occurs when
// writing to the client, it calls h.handlers.OnError if it is not nil.
func (h *Handler) handleClientWrite(client *Client, handlerCtx *Context) {
	defer func() {
		if r := recover(); r != nil {
			h.log(LogTypeError, LogLevelError, "PANIC RECOVERED in handleClientWrite (Client: %s): %v", client.ID, r)
		}
	}()

	conn, ok := client.Conn.(*websocket.Conn)
	if !ok {
		h.log(LogTypeError, LogLevelError, "Client %s connection is not a websocket connection", client.ID)
		return
	}

	ticker := time.NewTicker(h.config.PingPeriod)
	defer func() {
		ticker.Stop()
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.log(LogTypeError, LogLevelError, "PANIC RECOVERED closing connection for client %s: %v", client.ID, r)
				}
			}()

			if conn != nil {
				_ = conn.Close()
			}
		}()
	}()

	ctx := handlerCtx.Context()

	for {
		select {
		case <-ctx.Done():
			h.log(LogTypeMessage, LogLevelDebug, "%s write cancelled", client.ID)
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"), time.Now().Add(h.config.WriteTimeout))
			return

		case message, ok := <-client.MessageChan:
			if !ok {
				h.log(LogTypeMessage, LogLevelDebug, "%s write channel closed", client.ID)
				_ = conn.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(h.config.WriteTimeout))
				return
			}

			select {
			case <-ctx.Done():
				h.log(LogTypeMessage, LogLevelDebug, "%s write cancelled", client.ID)
				return
			default:
			}

			// Set write deadline - if this fails, connection is likely dead
			if err := conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout)); err != nil {
				h.log(LogTypeError, LogLevelDebug, "%s write deadline error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetWriteDeadlineError(err), handlerCtx)
				}
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				h.log(LogTypeMessage, LogLevelError, "%s write error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, err, handlerCtx)
				}
				return
			}
			h.log(LogTypeMessage, LogLevelDebug, "%s wrote: %s", client.ID, message)

		case <-ticker.C:
			select {
			case <-ctx.Done():
				h.log(LogTypeMessage, LogLevelDebug, "%s write cancelled for ping", client.ID)
				return
			default:
			}

			// Set write deadline for ping - if this fails, connection is likely dead
			if err := conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout)); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetWriteDeadlineError(err), handlerCtx)
				}
				return
			}

			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(h.config.WriteTimeout)); err != nil {
				h.log(LogTypeMessage, LogLevelError, "%s ping error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSendMessageError(err), handlerCtx)
				}
				return
			}
			h.log(LogTypeMessage, LogLevelDebug, "%s sent ping", client.ID)

			if h.events.OnPing != nil {
				if err := h.events.OnPing(client, handlerCtx); err != nil {
					_ = err
				}
			}
		}
	}
}

// handleClientRead is a goroutine that reads messages from a client. It reads
// from the client's websocket connection and processes the messages. If the
// client's websocket connection is closed or an error occurs when reading from
// the client, it calls h.handlers.OnError if it is not nil and
// h.handlers.OnDisconnect when the client is done.
func (h *Handler) handleClientRead(client *Client, handlerCtx *Context) {
	defer func() {
		if r := recover(); r != nil {
			h.log(LogTypeError, LogLevelError, "PANIC RECOVERED in handleClientRead (Client: %s): %v", client.ID, r)
		}
		h.cleanupClient(client, handlerCtx)
	}()

	conn, ok := client.Conn.(*websocket.Conn)
	if !ok {
		h.log(LogTypeError, LogLevelError, "Client %s does not have a websocket connection", client.ID)
		return
	}

	ctx := handlerCtx.Context()

	messageChan := make(chan struct {
		messageType int
		data        []byte
		err         error
	}, 1)

	maxViolations := h.rateLimiter.config.MaxRateLimitViolations
	violations := 0

	go func() {
		defer close(messageChan)
		for {
			if h.rateLimiter != nil && (!h.rateLimiter.AllowClient(client.ID) || !h.rateLimiter.AllowNetAddr(conn.RemoteAddr())) {
				violations++

				h.log(LogTypeRateLimit, LogLevelInfo, "Rate limit exceeded for client %s (%d/%d violations)", client.ID, violations, maxViolations)

				if violations >= maxViolations {
					messageChan <- struct {
						messageType int
						data        []byte
						err         error
					}{0, nil, ErrRateLimitExceeded}

					h.log(LogTypeRateLimit, LogLevelError, "Cancelling read for client %s (%d/%d violations)", client.ID, violations, maxViolations)
					return
				}

				continue
			}

			if err := conn.SetReadDeadline(time.Now().Add(h.config.ReadTimeout)); err != nil {
				h.log(LogTypeError, LogLevelDebug, "%s set read deadline error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetReadDeadlineError(err), handlerCtx)
				}
				return
			}

			messageType, data, err := conn.ReadMessage()
			h.log(LogTypeMessage, LogLevelDebug, "%s read: %s", client.ID, string(data))
			select {
			case messageChan <- struct {
				messageType int
				data        []byte
				err         error
			}{messageType, data, err}:
			case <-ctx.Done():
				h.log(LogTypeMessage, LogLevelDebug, "%s read cancelled", client.ID)
				return
			}

			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			h.log(LogTypeMessage, LogLevelDebug, "%s read cancelled", client.ID)
			_ = conn.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, ErrServerShutdown.Error()),
				time.Now().Add(h.config.WriteTimeout),
			)
			return

		case msg, ok := <-messageChan:
			if !ok {
				h.log(LogTypeMessage, LogLevelDebug, "%s read channel closed", client.ID)
				return
			}

			if msg.err != nil {
				if errors.Is(msg.err, ErrRateLimitExceeded) {
					h.log(LogTypeRateLimit, LogLevelInfo, "Rate limit exceeded for client %s", client.ID)
					_ = conn.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseTryAgainLater, ErrRateLimitExceeded.Error()),
						time.Now().Add(h.config.WriteTimeout),
					)
				}

				if websocket.IsUnexpectedCloseError(msg.err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.log(LogTypeError, LogLevelDebug, "%s read error: %v", client.ID, msg.err)
					if h.events.OnError != nil {
						_ = h.events.OnError(client, msg.err, handlerCtx)
					}
				}
				return
			}

			// Reset read deadline - if this fails, connection might be in bad state
			if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
				h.log(LogTypeError, LogLevelDebug, "%s set read deadline error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetReadDeadlineError(err), handlerCtx)
				}
				return
			}

			message := &Message{
				Type:    MessageType(msg.messageType),
				RawData: msg.data,
				From:    client.ID,
				Created: time.Now(),
			}

			h.log(LogTypeMessage, LogLevelDebug, "%s read: %s", client.ID, string(message.RawData))
			h.processMessage(client, message, handlerCtx)
		}
	}
}

// processMessage is a helper function that processes a message from a client.
// It first calls the OnMessage handler if it is not nil. If the OnMessage handler
// returns an error, it calls the OnError handler if it is not nil. Then, it
// calls the OnRawMessage handler if it is not nil. If the OnRawMessage handler
// returns an error, it calls the OnError handler if it is not nil. Finally, it
// calls the OnJSONMessage handler if it is not nil, unmarshals the message data
// to JSON, and passes the unmarshaled data to the handler. If the OnJSONMessage
// handler returns an error, it calls the OnError handler if it is not nil.
func (h *Handler) processMessage(client *Client, message *Message, handlerCtx *Context) {
	if h.events.OnMessage != nil {
		h.log(LogTypeMessage, LogLevelDebug, "%s OnMessage: %s", client.ID, string(message.RawData))
		if err := h.events.OnMessage(client, message, handlerCtx); err != nil {
			h.log(LogTypeError, LogLevelDebug, "%s OnMessage error: %v", client.ID, err)
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnMessage", err), handlerCtx)
			}
		}
	}

	if h.events.OnRawMessage != nil {
		h.log(LogTypeMessage, LogLevelDebug, "%s OnRawMessage: %s", client.ID, string(message.RawData))
		if err := h.events.OnRawMessage(client, message.RawData, handlerCtx); err != nil {
			h.log(LogTypeError, LogLevelDebug, "%s OnRawMessage error: %v", client.ID, err)
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnRawMessage", err), handlerCtx)
			}
		}
	}

	if h.events.OnJSONMessage != nil {
		h.log(LogTypeMessage, LogLevelDebug, "%s OnJSONMessage: %s", client.ID, string(message.RawData))
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); errors.Is(err, nil) {
			message.Data = jsonData
			message.Encoding = JSON
			if err := h.events.OnJSONMessage(client, jsonData, handlerCtx); err != nil {
				h.log(LogTypeError, LogLevelDebug, "%s OnJSONMessage error: %v", client.ID, err)
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newEventFailedError("OnJSONMessage", err), handlerCtx)
				}
			}
		}
	}
	// TODO: add Protobuf and other formats
}

func (h *Handler) initConnectionPool() {
	if h.config.MaxConnections > 0 {
		h.connectionPool = NewConnectionPool(
			h.config.MaxConnections,
			h.config.MaxConnectionsPerIP,
		)
		h.log(LogTypeConnection, LogLevelInfo,
			"Connection pool initialized: max_total=%d, max_per_ip=%d",
			h.config.MaxConnections, h.config.MaxConnectionsPerIP)
	} else {
		h.log(LogTypeConnection, LogLevelWarn, "Connection pool disabled: MaxConnections is 0")
	}
}

// ensureHubRunning starts the hub if it is not already running.
// It is safe to call concurrently.
func (h *Handler) ensureHubRunning(ctx context.Context) {
	h.hubRunning.Do(func() {
		go h.hub.Run(ctx)
	})
}

func (h *Handler) cleanupClient(client *Client, handlerCtx *Context) {
	defer func() {
		if r := recover(); r != nil {
			h.log(LogTypeError, LogLevelError, "PANIC RECOVERED in cleanup for client %s: %v", client.ID, r)
		}
	}()

	if h.hub != nil {
		h.log(LogTypeClient, LogLevelDebug, "Removing client %s from hub", client.ID)
		h.hub.RemoveClient(client)
	}

	if client.Conn != nil {
		if conn, ok := client.Conn.(*websocket.Conn); ok {
			h.log(LogTypeClient, LogLevelDebug, "Closing client %s connection", client.ID)
			_ = conn.Close()
		}
	}

	if h.events.OnDisconnect != nil {
		h.log(LogTypeClient, LogLevelDebug, "Calling OnDisconnect for client %s", client.ID)
		_ = h.events.OnDisconnect(client, handlerCtx)
	}
}

func (h *Handler) log(logType LogType, level LogLevel, msg string, args ...interface{}) {
	lvl, ok := h.logger.Level[logType]
	if !ok {
		lvl = LogLevelNone
	}

	if level <= lvl {
		h.logger.Logger.Log(logType, level, msg, args...)
	}
}

// extractHeaders extracts relevant headers from the given http request.
// It returns a map of header name to header value. Only the following
// headers are considered: Authorization, X-Forwarded-For, X-Real-IP,
// Accept, Accept-Language, and Accept-Encoding. If a header is not
// present in the request, it is not included in the returned map.
func extractHeaders(r *http.Request, extraHeaders ...string) map[string]string {
	headers := make(map[string]string)

	// common headers that might be useful in handlers
	relevantHeaders := []string{ // TODO: make configurable
		"Authorization",
		"X-Forwarded-For",
		"X-Real-Ip",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
	}
	relevantHeaders = append(relevantHeaders, extraHeaders...)

	for _, header := range relevantHeaders {
		if value := r.Header.Get(header); value != "" {
			headers[header] = value
		}
	}

	return headers
}

// getClientIPFromRequest returns the client's IP address from the given http request.
// If the request contains a valid X-Real-IP or X-Forwarded-For header, it returns the IP
// address from the header. Otherwise, it returns the IP address from the remote address.
// The IP address is returned as a string in the format "192.0.2.1".
func getClientIPFromRequest(r *http.Request) string {
	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		ip, _, _ := net.SplitHostPort(ip)
		return ip
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// generateRequestID returns a unique request ID as a string. The request ID
// is a concatenation of the current time in nanoseconds and a random number
// between 0 and 9999. The request ID is used to identify requests and is
// passed to the OnRequest handler if it is not nil. The request ID can be
// used to identify requests in logs, metrics, and other monitoring tools.
func generateRequestID() string {
	randN, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), randN)
}
