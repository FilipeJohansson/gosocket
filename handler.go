// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type OnConnectFunc func(c *Client, ctx *HandlerContext) error
type OnDisconnectFunc func(c *Client, ctx *HandlerContext) error
type OnMessageFunc func(c *Client, m *Message, ctx *HandlerContext) error            // generic handler
type OnRawMessageFunc func(c *Client, m []byte, ctx *HandlerContext) error           // raw data handler
type OnJSONMessageFunc func(c *Client, m interface{}, ctx *HandlerContext) error     // JSON specific handler
type OnProtobufMessageFunc func(c *Client, m interface{}, ctx *HandlerContext) error // Protobuf specific handler
type OnErrorFunc func(c *Client, err error, ctx *HandlerContext) error
type OnPingFunc func(c *Client, ctx *HandlerContext) error
type OnPongFunc func(c *Client, ctx *HandlerContext) error

type Events struct {
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
func NewHandler(options ...UniversalOption) (*Handler, error) {
	h := &Handler{
		hub:         NewHub(),
		config:      DefaultHandlerConfig(),
		events:      &Events{},
		serializers: make(map[EncodingType]Serializer),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		mu: sync.RWMutex{},
	}

	for _, o := range options {
		if err := o(h); err != nil {
			return nil, err
		}
	}

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
			http.Error(w, newAuthFailureError(err).Error(), http.StatusUnauthorized)
			return
		}
	}

	handlerCtx := NewHandlerContextFromRequest(h, r)
	handlerCtx.connInfo.ClientIP = getClientIPFromRequest(r)
	handlerCtx.connInfo.RequestID = generateRequestID()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		if h.events.OnError != nil {
			_ = h.events.OnError(nil, newUpgradeFailedError(err), handlerCtx)
		}
		return
	}

	clientId := GenerateClientID()
	client := NewClient(clientId, conn, h.hub)

	client.ConnInfo = handlerCtx.connInfo

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(h.config.MessageSize)

	if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
		if h.events.OnError != nil {
			_ = h.events.OnError(nil, newSetReadDeadlineError(err), handlerCtx)
		}
		conn.Close()
		return
	}

	conn.SetPongHandler(func(string) error {
		if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
			return newSetReadDeadlineError(err)
		}
		if h.events.OnPong != nil {
			if err := h.events.OnPong(client, handlerCtx); err != nil {
				_ = err
			}
		}
		return nil
	})

	h.hub.AddClient(client)

	if h.events.OnConnect != nil {
		if err := h.events.OnConnect(client, handlerCtx); err != nil {
			h.hub.RemoveClient(client)
			conn.Close()
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnConnect", err), handlerCtx)
			}
			return
		}
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
		_ = client.Conn.(*websocket.Conn).Close()
	}()

	conn := client.Conn.(*websocket.Conn)

	handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)

	for {
		select {
		case message, ok := <-client.MessageChan:
			// Set write deadline - if this fails, connection is likely dead
			if err := conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout)); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetWriteDeadlineError(err), handlerCtx)
				}
				return
			}

			if !ok {
				// Channel closed, send close message and return
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, err, handlerCtx)
				}
				return
			}

		case <-ticker.C:
			// Set write deadline for ping - if this fails, connection is likely dead
			if err := conn.SetWriteDeadline(time.Now().Add(h.config.WriteTimeout)); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSetWriteDeadlineError(err), handlerCtx)
				}
				return
			}

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newSendMessageError(err), handlerCtx)
				}
				return
			}

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
func (h *Handler) handleClientRead(client *Client) {
	defer func() {
		h.hub.RemoveClient(client)
		_ = client.Conn.(*websocket.Conn).Close()

		if h.events.OnDisconnect != nil {
			handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)
			// OnDisconnect might return an error, but we're in cleanup so just ignore it
			_ = h.events.OnDisconnect(client, handlerCtx)
		}
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if h.events.OnError != nil {
					handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)
					_ = h.events.OnError(client, err, handlerCtx)
				}
			}
			break
		}

		// Reset read deadline - if this fails, connection might be in bad state
		if err := conn.SetReadDeadline(time.Now().Add(h.config.PongWait)); err != nil {
			if h.events.OnError != nil {
				handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)
				_ = h.events.OnError(client, newSetReadDeadlineError(err), handlerCtx)
			}
			break
		}

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
	handlerCtx := NewHandlerContextWithConnection(h, client.ConnInfo)

	if h.events.OnMessage != nil {
		if err := h.events.OnMessage(client, message, handlerCtx); err != nil {
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnMessage", err), handlerCtx)
			}
		}
	}

	if h.events.OnRawMessage != nil {
		if err := h.events.OnRawMessage(client, message.RawData, handlerCtx); err != nil {
			if h.events.OnError != nil {
				_ = h.events.OnError(client, newEventFailedError("OnRawMessage", err), handlerCtx)
			}
		}
	}

	if h.events.OnJSONMessage != nil {
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); errors.Is(err, nil) {
			message.Data = jsonData
			message.Encoding = JSON
			if err := h.events.OnJSONMessage(client, jsonData, handlerCtx); err != nil {
				if h.events.OnError != nil {
					_ = h.events.OnError(client, newEventFailedError("OnJSONMessage", err), handlerCtx)
				}
			}
		}
	}
	// TODO: add Protobuf and other formats
}

// ensureHubRunning starts the hub if it is not already running.
// It is safe to call concurrently.
func (h *Handler) ensureHubRunning() {
	h.hubRunning.Do(func() {
		go h.hub.Run()
	})
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
		"X-Real-Ip",
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
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}
