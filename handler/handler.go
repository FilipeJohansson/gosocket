// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/FilipeJohansson/gosocket"
	"github.com/gorilla/websocket"
)

// every events handlers
type Handlers struct {
	OnConnect         func(*gosocket.Client, *HandlerContext) error
	OnDisconnect      func(*gosocket.Client, *HandlerContext) error
	OnMessage         func(*gosocket.Client, *gosocket.Message, *HandlerContext) error // generic handler
	OnRawMessage      func(*gosocket.Client, []byte, *HandlerContext) error            // raw data handler
	OnJSONMessage     func(*gosocket.Client, interface{}, *HandlerContext) error       // JSON specific handler
	OnProtobufMessage func(*gosocket.Client, interface{}, *HandlerContext) error       // Protobuf specific handler
	OnError           func(*gosocket.Client, error, *HandlerContext) error
	OnPing            func(*gosocket.Client, *HandlerContext) error
	OnPong            func(*gosocket.Client, *HandlerContext) error
}

type HandlerContext struct {
	ctx       context.Context
	startTime time.Time
	connInfo  *gosocket.ConnectionInfo

	handler *Handler
	hub     gosocket.IHub
}

// NewHandlerContext creates a new context (for inner goroutines)
// It creates a new context with the given handler and hub, and the same
// connection info as the original context. The context is created with a
// new background context, and the start time is set to the current time.
func NewHandlerContext(handler *Handler) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo: &gosocket.ConnectionInfo{
			RequestID: generateRequestID(),
		},
	}
}

// NewHandlerContextFromRequest creates a new context from the given http request.
// It retrieves the client IP, user agent, origin, and headers from the request
// and uses them to create a new context.
func NewHandlerContextFromRequest(handler *Handler, r *http.Request) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo: &gosocket.ConnectionInfo{
			ClientIP:  getClientIPFromRequest(r),
			UserAgent: r.Header.Get("User-Agent"),
			Origin:    r.Header.Get("Origin"),
			Headers:   extractHeaders(r),
			RequestID: generateRequestID(),
		},
	}
}

// NewHandlerContextWithConnection creates a new context with the given connection info. This
// method is useful when you want to reuse the same connection info for multiple contexts.
// It returns a new context with the given connection info, and the same handler and hub as the
// original context. The context is created with a new background context, and the start time is
// set to the current time.
func NewHandlerContextWithConnection(handler *Handler, connInfo *gosocket.ConnectionInfo) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo:  connInfo,
	}
}

// Context returns the context associated with the handler context. The context
// is the parent context that was passed when creating the handler context. The
// context can be used to cancel the context, or to retrieve values from the
// context.
func (hc *HandlerContext) Context() context.Context {
	return hc.ctx
}

// WithContext returns a new context with the given context. The context is the parent context
// that was passed when creating the handler context. The context can be used to cancel the
// context, or to retrieve values from the context. The new context is a shallow copy of the
// original context, with the context replaced.
func (hc *HandlerContext) WithContext(ctx context.Context) *HandlerContext {
	newHC := *hc
	newHC.ctx = ctx
	return &newHC
}

// WithTimeout returns a new context with the given timeout, and a cancel function. The
// returned context is a shallow copy of the original context, with the timeout set to the
// given value. The cancel function can be used to cancel the context, or to retrieve values
// from the context. The context can be used to cancel the context, or to retrieve values
// from the context.
func (hc *HandlerContext) WithTimeout(timeout time.Duration) (*HandlerContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(hc.ctx, timeout)
	return hc.WithContext(ctx), cancel
}

// RequestID returns the request ID associated with the context. The request ID is a unique
// identifier for the connection, and can be used to identify the connection in logs and metrics.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) RequestID() string {
	if hc.connInfo != nil {
		return hc.connInfo.RequestID
	}
	return ""
}

// ClientIP returns the client IP address associated with the context.
// If the context is not associated with a connection, "unknown" is returned.
func (hc *HandlerContext) ClientIP() string {
	if hc.connInfo != nil {
		return hc.connInfo.ClientIP
	}
	return "unknown"
}

// UserAgent returns the user agent associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) UserAgent() string {
	if hc.connInfo != nil {
		return hc.connInfo.UserAgent
	}
	return ""
}

// Origin returns the origin associated with the context.
// The origin is the value of the Origin header of the request that
// established the connection. If the context is not associated with a
// connection, an empty string is returned.
func (hc *HandlerContext) Origin() string {
	if hc.connInfo != nil {
		return hc.connInfo.Origin
	}
	return ""
}

// Header returns the value of the given header key associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) Header(key string) string {
	if hc.connInfo != nil && hc.connInfo.Headers != nil {
		return hc.connInfo.Headers[key]
	}
	return ""
}

// Headers returns the HTTP headers associated with the context.
// If the context is not associated with a connection, nil is returned.
func (hc *HandlerContext) Headers() map[string]string {
	if hc.connInfo != nil {
		return hc.connInfo.Headers
	}
	return nil
}

// ProcessingDuration returns the time elapsed since the context was created.
// The returned duration is the time elapsed between the context creation
// time and the current time. If the context is not associated with a
// connection, 0 is returned.
func (hc *HandlerContext) ProcessingDuration() time.Duration {
	return time.Since(hc.startTime)
}

// BroadcastToAll broadcasts a message to all connected clients.
//
// This function is a shortcut for calling hub.BroadcastMessage(message).
// If the context is not associated with a hub, an error is returned.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastToAll(message *gosocket.Message) error {
	if hc.hub == nil {
		return fmt.Errorf("hub is nil")
	}

	hc.hub.BroadcastMessage(message)
	return nil
}

// BroadcastToRoom broadcasts a message to all clients in the specified room.
//
// This function is a shortcut for calling hub.BroadcastToRoom(room, message).
// If the context is not associated with a hub, an error is returned.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastToRoom(room string, message *gosocket.Message) error {
	if hc.hub == nil {
		return fmt.Errorf("hub is nil")
	}

	message.Room = room
	hc.hub.BroadcastToRoom(room, message)
	return nil
}

// BroadcastJSONToAll broadcasts the given data as JSON to all connected clients.
//
// It wraps the given data in a Message and sets the Encoding to JSON, then
// calls BroadcastToAll to send the message.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastJSONToAll(data interface{}) error {
	message := gosocket.NewMessage(gosocket.TextMessage, data)
	message.Encoding = gosocket.JSON
	return hc.BroadcastToAll(message)
}

// BroadcastJSONToRoom broadcasts the given data as JSON to all clients in the specified room.
//
// It wraps the given data in a Message and sets the Encoding to JSON, then
// calls BroadcastToRoom to send the message.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastJSONToRoom(room string, data interface{}) error {
	message := gosocket.NewMessage(gosocket.TextMessage, data)
	message.Encoding = gosocket.JSON
	return hc.BroadcastToRoom(room, message)
}

// GetClientsInRoom returns a list of all clients in the given room. If the
// context is not associated with a hub, an empty slice is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetClientsInRoom(room string) []*gosocket.Client {
	if hc.hub == nil {
		return []*gosocket.Client{}
	}
	return hc.hub.GetRoomClients(room)
}

// GetAllClients returns a list of all clients connected to the hub.
//
// If the context is not associated with a hub, an empty slice is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetAllClients() []*gosocket.Client {
	if hc.hub == nil {
		return []*gosocket.Client{}
	}

	clients := []*gosocket.Client{}
	for client := range hc.hub.GetClients() {
		clients = append(clients, client)
	}
	return clients
}

// GetStats returns a map with statistics about the hub.
//
// If the context is not associated with a hub, an empty map is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetStats() map[string]interface{} {
	if hc.hub == nil {
		return map[string]interface{}{}
	}
	return hc.hub.GetStats()
}

// Handler returns the handler associated with the context. This can be used to
// access the underlying handler's configuration and functionality.
func (hc *HandlerContext) Handler() *Handler {
	return hc.handler
}

// Hub returns the hub associated with the context. This can be used to access the
// hub's methods and configuration. The returned hub is the same as the one passed
// when creating the HandlerContext. If the context is not associated with a hub,
// nil is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) Hub() gosocket.IHub {
	return hc.hub
}

type Handler struct {
	hub         gosocket.IHub
	config      *gosocket.HandlerConfig
	handlers    *Handlers
	serializers map[gosocket.EncodingType]gosocket.Serializer
	authFunc    gosocket.AuthFunc
	upgrader    websocket.Upgrader
	hubRunning  sync.Once
	middlewares []gosocket.Middleware
	mu          sync.RWMutex
}

// NewHandler returns a new instance of Handler with default configuration.
// The default configuration is to accept up to 1000 connections and to set the
// read and write buffers to 1024 bytes. The default encoding is JSON, but you can
// change it by calling the WithEncoding method.
func NewHandler(options ...func(*Handler)) *Handler {
	handler := &Handler{
		hub:         gosocket.NewHub(),
		config:      gosocket.DefaultHandlerConfig(),
		handlers:    &Handlers{},
		serializers: make(map[gosocket.EncodingType]gosocket.Serializer),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		mu: sync.RWMutex{},
	}

	for _, o := range options {
		o(handler)
	}

	return handler
}

func (h *Handler) Config() gosocket.HandlerConfig {
	return *h.config
}

func (h *Handler) Hub() gosocket.IHub {
	return h.hub
}

func (h *Handler) Serializers() map[gosocket.EncodingType]gosocket.Serializer {
	return h.serializers
}

func (h *Handler) Middlewares() []gosocket.Middleware {
	return h.middlewares
}

func (h *Handler) Handlers() *Handlers {
	return h.handlers
}

func (h *Handler) AuthFunc() gosocket.AuthFunc {
	return h.authFunc
}

func (h *Handler) AddSerializer(encoding gosocket.EncodingType, serializer gosocket.Serializer) {
	if h.serializers == nil {
		h.serializers = make(map[gosocket.EncodingType]gosocket.Serializer)
	}
	h.serializers[encoding] = serializer
}

func (h *Handler) SetHub(hub gosocket.IHub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hub = hub
}

func (h *Handler) SetDefaultEncoding(encoding gosocket.EncodingType) {
	h.config.DefaultEncoding = encoding
}

// ===== CONTROLLERS =====

// ServeHTTP implements the http.Handler interface.
// It is the entrypoint for handling all WebSocket requests.
// It first ensures that the hub is running, and then applies
// all configured middlewares to the request. Finally, it calls
// the configured WebSocket handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ensureHubRunning()

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
		var err error
		userData, err = h.authFunc(r)
		if err != nil {
			http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	handlerCtx := NewHandlerContextFromRequest(h, r)
	handlerCtx.connInfo.ClientIP = getClientIPFromRequest(r)
	handlerCtx.connInfo.RequestID = generateRequestID()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.handlers.OnError != nil {
			h.handlers.OnError(nil, fmt.Errorf("websocket upgrade failed: %w", err), handlerCtx)
		}
		return
	}

	clientId := gosocket.GenerateClientID()
	client := gosocket.NewClient(clientId, conn, h.hub)

	client.ConnInfo = handlerCtx.connInfo

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(h.config.MessageSize)
	conn.SetReadDeadline(time.Now().Add(h.config.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(h.config.PongWait))
		if h.handlers.OnPong != nil {
			h.handlers.OnPong(client, handlerCtx)
		}
		return nil
	})

	h.hub.AddClient(client)

	if h.handlers.OnConnect != nil {
		h.handlers.OnConnect(client, handlerCtx)
	}

	go h.handleClientWrite(client)
	go h.handleClientRead(client)
}

// handleClientWrite is a goroutine that writes messages to a client. It reads
// from the client's MessageChan and writes to the client's websocket connection.
// If the MessageChan is closed, it sends a CloseMessage to the client and
// returns. It also sends a PingMessage every h.config.PingPeriod to the
// client. If the client's websocket connection is closed or an error occurs when
// writing to the client, it calls h.handlers.OnError if it is not nil.
func (h *Handler) handleClientWrite(client *gosocket.Client) {
	ticker := time.NewTicker(h.config.PingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.(*websocket.Conn).Close()
	}()

	conn := client.Conn.(*websocket.Conn)

	handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)

	for {
		select {
		case message, ok := <-client.MessageChan:
			conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				if h.handlers.OnError != nil {
					h.handlers.OnError(client, err, handlerCtx)
				}
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

			if h.handlers.OnPing != nil {
				h.handlers.OnPing(client, handlerCtx)
			}
		}
	}
}

// handleClientRead is a goroutine that reads messages from a client. It reads
// from the client's websocket connection and processes the messages. If the
// client's websocket connection is closed or an error occurs when reading from
// the client, it calls h.handlers.OnError if it is not nil and
// h.handlers.OnDisconnect when the client is done.
func (h *Handler) handleClientRead(client *gosocket.Client) {
	defer func() {
		h.hub.RemoveClient(client)
		client.Conn.(*websocket.Conn).Close()

		if h.handlers.OnDisconnect != nil {
			handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)
			h.handlers.OnDisconnect(client, handlerCtx)
		}
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if h.handlers.OnError != nil {
					handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)
					h.handlers.OnError(client, err, handlerCtx)
				}
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(h.config.PongWait)) // reset read deadline

		message := &gosocket.Message{
			Type:    gosocket.MessageType(messageType),
			RawData: data,
			From:    client.ID,
			Created: time.Now(),
		}

		h.processMessage(client, message)
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
func (h *Handler) processMessage(client *gosocket.Client, message *gosocket.Message) {
	handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)

	if h.handlers.OnMessage != nil {
		if err := h.handlers.OnMessage(client, message, handlerCtx); err != nil {
			if h.handlers.OnError != nil {
				h.handlers.OnError(client, err, handlerCtx)
			}
		}
	}

	if h.handlers.OnRawMessage != nil {
		if err := h.handlers.OnRawMessage(client, message.RawData, handlerCtx); err != nil {
			if h.handlers.OnError != nil {
				h.handlers.OnError(client, err, handlerCtx)
			}
		}
	}

	if h.handlers.OnJSONMessage != nil {
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); err == nil {
			message.Data = jsonData
			message.Encoding = gosocket.JSON
			if err := h.handlers.OnJSONMessage(client, jsonData, handlerCtx); err != nil {
				if h.handlers.OnError != nil {
					h.handlers.OnError(client, err, handlerCtx)
				}
			}
		}
	}
	// TODO: add Protobuf and other formats
}

// ensureHubRunning starts the hub if it is not already running.
// It is safe to call concurrently.
func (h *Handler) ensureHubRunning() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hubRunning.Do(func() {
		go h.hub.Run()
	})
}

// ===== Functional Options =====

// WithMaxConnections sets the maximum number of connections the server should accept.
// If max <= 0, it is set to the default of 1000.
func WithMaxConnections(max int) func(*Handler) {
	return func(h *Handler) {
		if max <= 0 {
			fmt.Println("Warning: max connections must be greater than 0, setting to default 1000")
			max = 1000
		}

		h.config.MaxConnections = max
	}
}

// WithMessageSize sets the maximum message size for the server. If the size is less than or equal to 0, the default of 1024 is used.
func WithMessageSize(size int64) func(*Handler) {
	return func(h *Handler) {
		if size <= 0 {
			fmt.Println("Warning: message size must be greater than 0, setting to default 1024")
			size = 1024
		}

		h.config.MessageSize = size
	}
}

// WithTimeout sets the read and write timeouts for the server. If read is negative or write is negative,
// the timeouts are set to 0.
func WithTimeout(read, write time.Duration) func(*Handler) {
	return func(h *Handler) {
		if read < 0 || write < 0 {
			fmt.Println("Warning: timeouts must be non-negative, setting to default 0")
			read = 0
			write = 0
		}

		h.config.ReadTimeout = read
		h.config.WriteTimeout = write
	}
}

// WithPingPong sets the ping and pong wait periods for the server. If pingPeriod or pongWait are
// negative, the timeouts are set to 0.
func WithPingPong(pingPeriod, pongWait time.Duration) func(*Handler) {
	return func(h *Handler) {
		if pingPeriod <= 0 || pongWait <= 0 {
			fmt.Println("Warning: ping period and pong wait must be greater than 0, setting to default 0")
			pingPeriod = 0
			pongWait = 0
		}

		h.config.PingPeriod = pingPeriod
		h.config.PongWait = pongWait
	}
}

// WithAllowedOrigins sets the allowed origins for the server. If the slice is empty,
// the server will allow any origin. Otherwise, the server will only allow the specified
// origins in the Origin header of incoming requests.
func WithAllowedOrigins(origins []string) func(*Handler) {
	return func(h *Handler) {
		h.config.AllowedOrigins = origins
	}
}

// WithEncoding sets the default encoding for the server. This encoding will be used
// to encode messages sent to clients if no encoding is specified. If the encoding
// is not supported, the server will not start.
func WithEncoding(encoding gosocket.EncodingType) func(*Handler) {
	return func(h *Handler) {
		h.config.DefaultEncoding = encoding
	}
}

// WithSerializer sets the serializer for the server to use with the given encoding.
// If the serializer is nil, the server will not use this encoding.
// The server will use the default encoding if no encoding is specified.
func WithSerializer(encoding gosocket.EncodingType, serializer gosocket.Serializer) func(*Handler) {
	return func(h *Handler) {
		if h.serializers == nil {
			h.serializers = make(map[gosocket.EncodingType]gosocket.Serializer)
		}
		h.serializers[encoding] = serializer
	}
}

// WithJSONSerializer sets the JSON serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func WithJSONSerializer() func(*Handler) {
	return WithSerializer(gosocket.JSON, gosocket.JSONSerializer{})
}

// WithProtobufSerializer sets the Protobuf serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func WithProtobufSerializer() func(*Handler) {
	return WithSerializer(gosocket.Protobuf, gosocket.ProtobufSerializer{})
}

// WithRawSerializer sets the Raw serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func WithRawSerializer() func(*Handler) {
	return WithSerializer(gosocket.Raw, gosocket.RawSerializer{})
}

// WithMiddleware adds a middleware to the handler. Middleware functions are executed
// before the OnConnect, OnMessage, and OnDisconnect handlers. They can be used to
// add authentication, logging, CORS support, or any other functionality that
// is needed.
func WithMiddleware(middleware gosocket.Middleware) func(*Handler) {
	return func(h *Handler) {
		if h.middlewares == nil {
			h.middlewares = make([]gosocket.Middleware, 0)
		}
		h.middlewares = append(h.middlewares, middleware)
	}
}

// WithAuth sets the authentication function for the server. The authentication
// function is called for each new connection to the server. It should return a
// map of user data and an error. If the error is not nil, the connection is
// closed. The user data is stored in the client's User field and can be accessed
// using the client.User() method.
func WithAuth(authFunc gosocket.AuthFunc) func(*Handler) {
	return func(h *Handler) {
		h.authFunc = authFunc
	}
}

// ===== HANDLERS =====

// OnConnect sets the OnConnect handler for the server. This handler is
// called when a new client connects to the server. The handler should return an
// error if the connection should be closed. The handler is called after the
// authentication function has been called and the client has been added to the
// server's list of clients.
func OnConnect(handler func(*gosocket.Client, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnConnect = handler
	}
}

// OnDisconnect sets the OnDisconnect handler for the server. This handler is
// called when a client disconnects from the server. The handler should return an
// error if the disconnection should be treated as an error. The handler is called
// after the client has been removed from the server's list of clients.
func OnDisconnect(handler func(*gosocket.Client, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnDisconnect = handler
	}
}

// OnMessage sets the OnMessage handler for the server. This handler is called when
// a new message is received from a client. The handler should return an error if
// the message should be treated as an error. The handler is called after the
// message has been decoded and deserialized.
func OnMessage(handler func(*gosocket.Client, *gosocket.Message, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnMessage = handler
	}
}

// OnRawMessage sets the OnRawMessage handler for the server. This handler is
// called when a new raw message is received from a client. The handler should
// return an error if the message should be treated as an error. The handler is
// called after the message has been decoded, but before it has been deserialized.
func OnRawMessage(handler func(*gosocket.Client, []byte, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnRawMessage = handler
	}
}

// OnJSONMessage sets the OnJSONMessage handler for the server. This handler is
// called when a new JSON message is received from a client. The handler should
// return an error if the message should be treated as an error. The handler is
// called after the message has been decoded and parsed as JSON.
func OnJSONMessage(handler func(*gosocket.Client, interface{}, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnJSONMessage = handler
	}
}

// OnProtobufMessage sets the OnProtobufMessage handler for the server. This handler
// is called when a new Protobuf message is received from a client. The handler
// should return an error if the message should be treated as an error. The handler
// is called after the message has been decoded and parsed as Protobuf.
func OnProtobufMessage(handler func(*gosocket.Client, interface{}, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnProtobufMessage = handler
	}
}

// OnError sets the OnError handler for the server. This handler is
// called when an error occurs. The handler should return an error if the
// error should be treated as an error. The handler is called with the
// client that caused the error and the error itself. The handler is called
// after the error has been logged.
func OnError(handler func(*gosocket.Client, error, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnError = handler
	}
}

// OnPing sets the OnPing handler for the server. This handler is
// called when a ping message is sent by a client. The handler should
// return an error if the ping should be treated as an error. The handler
// is called with the client that sent the ping.
func OnPing(handler func(*gosocket.Client, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnPing = handler
	}
}

// OnPong sets the OnPong handler for the server. This handler is called when a
// pong message is sent by a client. The handler should return an error if the
// pong should be treated as an error. The handler is called with the client that
// sent the pong.
func OnPong(handler func(*gosocket.Client, *HandlerContext) error) func(*Handler) {
	return func(h *Handler) {
		if h.handlers == nil {
			h.handlers = &Handlers{}
		}
		h.handlers.OnPong = handler
	}
}

// extractHeaders extracts relevant headers from the given http request.
// It returns a map of header name to header value. Only the following
// headers are considered: Authorization, X-Forwarded-For, X-Real-IP,
// Accept, Accept-Language, and Accept-Encoding. If a header is not
// present in the request, it is not included in the returned map.
func extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)

	// common headers that might be useful in handlers
	relevantHeaders := []string{
		"Authorization",
		"X-Forwarded-For",
		"X-Real-IP",
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
	}

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
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

// generateRequestID returns a unique request ID as a string. The request ID
// is a concatenation of the current time in nanoseconds and a random number
// between 0 and 9999. The request ID is used to identify requests and is
// passed to the OnRequest handler if it is not nil. The request ID can be
// used to identify requests in logs, metrics, and other monitoring tools.
func generateRequestID() string {
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}
