// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// every events handlers
type Handlers struct {
	OnConnect         func(*Client) error
	OnDisconnect      func(*Client) error
	OnMessage         func(*Client, *Message) error    // generic handler
	OnRawMessage      func(*Client, []byte) error      // raw data handler
	OnJSONMessage     func(*Client, interface{}) error // JSON specific handler
	OnProtobufMessage func(*Client, interface{}) error // Protobuf specific handler
	OnError           func(*Client, error) error
	OnPing            func(*Client) error
	OnPong            func(*Client) error
}

type Handler struct {
	hub         IHub
	config      *HandlerConfig
	handlers    *Handlers
	serializers map[EncodingType]Serializer
	authFunc    AuthFunc
	upgrader    websocket.Upgrader
	hubRunning  sync.Once
	middlewares []Middleware
	mu          sync.RWMutex
}

// NewHandler returns a new instance of Handler with default configuration.
// The default configuration is to accept up to 1000 connections and to set the
// read and write buffers to 1024 bytes. The default encoding is JSON, but you can
// change it by calling the WithEncoding method.
func NewHandler() *Handler {
	return &Handler{
		hub:         NewHub(),
		config:      DefaultHandlerConfig(),
		handlers:    &Handlers{},
		serializers: make(map[EncodingType]Serializer),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		mu: sync.RWMutex{},
	}
}

// ===== Fluent Interface =====

// WithMaxConnections sets the maximum number of connections the server should accept.
// If max <= 0, it is set to the default of 1000.
func (h *Handler) WithMaxConnections(max int) *Handler {
	if max <= 0 {
		fmt.Println("Warning: max connections must be greater than 0, setting to default 1000")
		max = 1000
	}

	h.config.MaxConnections = max
	return h
}

// WithMessageSize sets the maximum message size for the server. If the size is less than or equal to 0, the default of 1024 is used.
func (h *Handler) WithMessageSize(size int64) *Handler {
	if size <= 0 {
		fmt.Println("Warning: message size must be greater than 0, setting to default 1024")
		size = 1024
	}

	h.config.MessageSize = size
	return h
}

// WithTimeout sets the read and write timeouts for the server. If read is negative or write is negative,
// the timeouts are set to 0.
func (h *Handler) WithTimeout(read, write time.Duration) *Handler {
	if read < 0 || write < 0 {
		fmt.Println("Warning: timeouts must be non-negative, setting to default 0")
		read = 0
		write = 0
	}

	h.config.ReadTimeout = read
	h.config.WriteTimeout = write
	return h
}

// WithPingPong sets the ping and pong wait periods for the server. If pingPeriod or pongWait are
// negative, the timeouts are set to 0.
func (h *Handler) WithPingPong(pingPeriod, pongWait time.Duration) *Handler {
	if pingPeriod <= 0 || pongWait <= 0 {
		fmt.Println("Warning: ping period and pong wait must be greater than 0, setting to default 0")
		pingPeriod = 0
		pongWait = 0
	}

	h.config.PingPeriod = pingPeriod
	h.config.PongWait = pongWait
	return h
}

// WithAllowedOrigins sets the allowed origins for the server. If the slice is empty,
// the server will allow any origin. Otherwise, the server will only allow the specified
// origins in the Origin header of incoming requests.
func (h *Handler) WithAllowedOrigins(origins []string) *Handler {
	h.config.AllowedOrigins = origins
	return h
}

// WithEncoding sets the default encoding for the server. This encoding will be used
// to encode messages sent to clients if no encoding is specified. If the encoding
// is not supported, the server will not start.
func (h *Handler) WithEncoding(encoding EncodingType) *Handler {
	h.config.DefaultEncoding = encoding
	return h
}

// WithSerializer sets the serializer for the server to use with the given encoding.
// If the serializer is nil, the server will not use this encoding.
// The server will use the default encoding if no encoding is specified.
func (h *Handler) WithSerializer(encoding EncodingType, serializer Serializer) *Handler {
	if h.serializers == nil {
		h.serializers = make(map[EncodingType]Serializer)
	}
	h.serializers[encoding] = serializer
	return h
}

// WithJSONSerializer sets the JSON serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (h *Handler) WithJSONSerializer() *Handler {
	return h.WithSerializer(JSON, JSONSerializer{})
}

// WithProtobufSerializer sets the Protobuf serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (h *Handler) WithProtobufSerializer() *Handler {
	return h.WithSerializer(Protobuf, ProtobufSerializer{})
}

// WithRawSerializer sets the Raw serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (h *Handler) WithRawSerializer() *Handler {
	return h.WithSerializer(Raw, RawSerializer{})
}

// WithMiddleware adds a middleware to the handler. Middleware functions are executed
// before the OnConnect, OnMessage, and OnDisconnect handlers. They can be used to
// add authentication, logging, CORS support, or any other functionality that
// is needed.
func (h *Handler) WithMiddleware(middleware Middleware) *Handler {
	if h.middlewares == nil {
		h.middlewares = make([]Middleware, 0)
	}
	h.middlewares = append(h.middlewares, middleware)
	return h
}

// WithAuth sets the authentication function for the server. The authentication
// function is called for each new connection to the server. It should return a
// map of user data and an error. If the error is not nil, the connection is
// closed. The user data is stored in the client's User field and can be accessed
// using the client.User() method.
func (h *Handler) WithAuth(authFunc AuthFunc) *Handler {
	h.authFunc = authFunc
	return h
}

// ===== HANDLERS =====

// OnConnect sets the OnConnect handler for the server. This handler is
// called when a new client connects to the server. The handler should return an
// error if the connection should be closed. The handler is called after the
// authentication function has been called and the client has been added to the
// server's list of clients.
func (h *Handler) OnConnect(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnConnect = handler
	return h
}

// OnDisconnect sets the OnDisconnect handler for the server. This handler is
// called when a client disconnects from the server. The handler should return an
// error if the disconnection should be treated as an error. The handler is called
// after the client has been removed from the server's list of clients.
func (h *Handler) OnDisconnect(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnDisconnect = handler
	return h
}

// OnMessage sets the OnMessage handler for the server. This handler is called when
// a new message is received from a client. The handler should return an error if
// the message should be treated as an error. The handler is called after the
// message has been decoded and deserialized.
func (h *Handler) OnMessage(handler func(*Client, *Message) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnMessage = handler
	return h
}

// OnRawMessage sets the OnRawMessage handler for the server. This handler is
// called when a new raw message is received from a client. The handler should
// return an error if the message should be treated as an error. The handler is
// called after the message has been decoded, but before it has been deserialized.
func (h *Handler) OnRawMessage(handler func(*Client, []byte) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnRawMessage = handler
	return h
}

// OnJSONMessage sets the OnJSONMessage handler for the server. This handler is
// called when a new JSON message is received from a client. The handler should
// return an error if the message should be treated as an error. The handler is
// called after the message has been decoded and parsed as JSON.
func (h *Handler) OnJSONMessage(handler func(*Client, interface{}) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnJSONMessage = handler
	return h
}

// OnProtobufMessage sets the OnProtobufMessage handler for the server. This handler
// is called when a new Protobuf message is received from a client. The handler
// should return an error if the message should be treated as an error. The handler
// is called after the message has been decoded and parsed as Protobuf.
func (h *Handler) OnProtobufMessage(handler func(*Client, interface{}) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnProtobufMessage = handler
	return h
}

// OnError sets the OnError handler for the server. This handler is
// called when an error occurs. The handler should return an error if the
// error should be treated as an error. The handler is called with the
// client that caused the error and the error itself. The handler is called
// after the error has been logged.
func (h *Handler) OnError(handler func(*Client, error) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnError = handler
	return h
}

// OnPing sets the OnPing handler for the server. This handler is
// called when a ping message is sent by a client. The handler should
// return an error if the ping should be treated as an error. The handler
// is called with the client that sent the ping.
func (h *Handler) OnPing(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnPing = handler
	return h
}

// OnPong sets the OnPong handler for the server. This handler is called when a
// pong message is sent by a client. The handler should return an error if the
// pong should be treated as an error. The handler is called with the client that
// sent the pong.
func (h *Handler) OnPong(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnPong = handler
	return h
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
			h.handleWebSocket(w, r)
		})

		// apply middlewares and run
		finalHandler := h.ApplyMiddlewares(wsHandler)
		finalHandler.ServeHTTP(w, r)
	} else {
		// no middlewares, just run directly
		h.handleWebSocket(w, r)
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

// handleWebSocket is the HTTP handler that is registered for the WebSocket endpoint. It upgrades the connection to a WebSocket connection,
// authenticates the user using the authFunc, and then adds the client to the hub and starts the client's read and write goroutines.
// It also calls the OnConnect handler if it is set.
//
// This method is safe to call concurrently.
func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
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

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.handlers.OnError != nil {
			h.handlers.OnError(nil, fmt.Errorf("websocket upgrade failed: %w", err))
		}
		return
	}

	clientId := generateClientID()
	client := NewClient(clientId, conn, h.hub)

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(h.config.MessageSize)
	conn.SetReadDeadline(time.Now().Add(h.config.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(h.config.PongWait))
		if h.handlers.OnPong != nil {
			h.handlers.OnPong(client)
		}
		return nil
	})

	h.hub.AddClient(client)

	if h.handlers.OnConnect != nil {
		h.handlers.OnConnect(client)
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
func (h *Handler) handleClientWrite(client *Client) {
	ticker := time.NewTicker(h.config.PingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.(*websocket.Conn).Close()
	}()

	conn := client.Conn.(*websocket.Conn)

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
					h.handlers.OnError(client, err)
				}
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

			if h.handlers.OnPing != nil {
				h.handlers.OnPing(client)
			}
		}
	}
}

// handleClientRead is a goroutine that reads messages from a client. It reads
// from the client's websocket connection and processes the messages. If the
// client's websocket connection is closed or an error occurs when reading from
// the client, it calls h.handlers.OnError if it is not nil and
// h.handlers.OnDisconnect when the client is done.
func (h *Handler) handleClientRead(client *Client) {
	defer func() {
		h.hub.RemoveClient(client)
		client.Conn.(*websocket.Conn).Close()

		if h.handlers.OnDisconnect != nil {
			h.handlers.OnDisconnect(client)
		}
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if h.handlers.OnError != nil {
					h.handlers.OnError(client, err)
				}
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(h.config.PongWait)) // reset read deadline

		message := &Message{
			Type:    MessageType(messageType),
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
func (h *Handler) processMessage(client *Client, message *Message) {
	if h.handlers.OnMessage != nil {
		if err := h.handlers.OnMessage(client, message); err != nil {
			if h.handlers.OnError != nil {
				h.handlers.OnError(client, err)
			}
		}
	}

	if h.handlers.OnRawMessage != nil {
		if err := h.handlers.OnRawMessage(client, message.RawData); err != nil {
			if h.handlers.OnError != nil {
				h.handlers.OnError(client, err)
			}
		}
	}

	if h.handlers.OnJSONMessage != nil {
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); err == nil {
			message.Data = jsonData
			message.Encoding = JSON
			if err := h.handlers.OnJSONMessage(client, jsonData); err != nil {
				if h.handlers.OnError != nil {
					h.handlers.OnError(client, err)
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
