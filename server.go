// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// IServer defines the interface for a server that can manage client connections,
// broadcast messages, and handle various events.
type IServer interface {
	// Start starts the server.
	Start() error

	// StartWithContext starts the server with the given context.
	StartWithContext(ctx context.Context) error

	// Stop stops the server.
	Stop() error

	// StopGracefully stops the server gracefully with the given timeout.
	StopGracefully(timeout time.Duration) error

	// Broadcast methods

	// Broadcast sends a message to all connected clients.
	Broadcast(message []byte) error

	// BroadcastMessage sends a message to all connected clients.
	BroadcastMessage(message *Message) error

	// BroadcastData sends data to all connected clients.
	BroadcastData(data interface{}) error

	// BroadcastDataWithEncoding sends data to all connected clients with the given encoding.
	BroadcastDataWithEncoding(data interface{}, encoding EncodingType) error

	// BroadcastJSON sends JSON data to all connected clients.
	BroadcastJSON(data interface{}) error

	// BroadcastProtobuf sends Protobuf data to all connected clients.
	BroadcastProtobuf(data interface{}) error

	// BroadcastToRoom sends a message to all clients in the given room.
	BroadcastToRoom(room string, message []byte) error

	// BroadcastToRoomData sends data to all clients in the given room.
	BroadcastToRoomData(room string, data interface{}) error

	// BroadcastToRoomJSON sends JSON data to all clients in the given room.
	BroadcastToRoomJSON(room string, data interface{}) error

	// BroadcastToRoomProtobuf sends Protobuf data to all clients in the given room.
	BroadcastToRoomProtobuf(room string, data interface{}) error

	// Client management methods

	// GetClients returns a list of all connected clients.
	GetClients() []*Client

	// GetClient returns the client with the given ID.
	GetClient(id string) *Client

	// GetClientsInRoom returns a list of all clients in the given room.
	GetClientsInRoom(room string) []*Client

	// GetClientCount returns the number of connected clients.
	GetClientCount() int

	// DisconnectClient disconnects the client with the given ID.
	DisconnectClient(id string) error

	// Room management methods

	// CreateRoom creates a new room with the given name.
	CreateRoom(name string) error

	// DeleteRoom deletes the room with the given name.
	DeleteRoom(name string) error

	// GetRooms returns a list of all rooms.
	GetRooms() []string

	// JoinRoom joins the client with the given ID to the room with the given name.
	JoinRoom(clientID, room string) error

	// LeaveRoom leaves the client with the given ID from the room with the given name.
	LeaveRoom(clientID, room string) error

	// Configuration methods

	// WithPort sets the port number for the server.
	WithPort(port int) *Server

	// WithPath sets the path for the server.
	WithPath(path string) *Server

	// WithCORS enables or disables CORS for the server.
	WithCORS(enabled bool) *Server

	// WithSSL sets the SSL certificate and key files for the server.
	WithSSL(certFile, keyFile string) *Server

	// WithMaxConnections sets the maximum number of connections for the server.
	WithMaxConnections(max int) *Server

	// WithMessageSize sets the maximum message size for the server.
	WithMessageSize(size int64) *Server

	// WithTimeout sets the read and write timeouts for the server.
	WithTimeout(read, write time.Duration) *Server

	// WithPingPong sets the ping and pong periods for the server.
	WithPingPong(pingPeriod, pongWait time.Duration) *Server

	// WithAllowedOrigins sets the allowed origins for the server.
	WithAllowedOrigins(origins []string) *Server

	// WithEncoding sets the encoding for the server.
	WithEncoding(encoding EncodingType) *Server

	// WithSerializer sets the serializer for the server.
	WithSerializer(encoding EncodingType, serializer Serializer) *Server

	// WithJSONSerializer sets the JSON serializer for the server.
	WithJSONSerializer() *Server

	// WithProtobufSerializer sets the Protobuf serializer for the server.
	WithProtobufSerializer() *Server

	// WithRawSerializer sets the raw serializer for the server.
	WithRawSerializer() *Server

	// WithMiddleware sets the middleware for the server.
	WithMiddleware(middleware Middleware) *Server

	// WithAuth sets the authentication function for the server.
	WithAuth(authFunc AuthFunc) *Server

	// Event handlers

	// OnConnect sets the connect handler for the server.
	OnConnect(handler func(*Client, *HandlerContext) error) *Server

	// OnDisconnect sets the disconnect handler for the server.
	OnDisconnect(handler func(*Client, *HandlerContext) error) *Server

	// OnMessage sets the message handler for the server.
	OnMessage(handler func(*Client, *Message, *HandlerContext) error) *Server

	// OnRawMessage sets the raw message handler for the server.
	OnRawMessage(handler func(*Client, []byte, *HandlerContext) error) *Server

	// OnJSONMessage sets the JSON message handler for the server.
	OnJSONMessage(handler func(*Client, interface{}, *HandlerContext) error) *Server

	// OnProtobufMessage sets the Protobuf message handler for the server.
	OnProtobufMessage(handler func(*Client, interface{}, *HandlerContext) error) *Server

	// OnError sets the error handler for the server.
	OnError(handler func(*Client, error, *HandlerContext) error) *Server

	// OnPing sets the ping handler for the server.
	OnPing(handler func(*Client, *HandlerContext) error) *Server

	// OnPong sets the pong handler for the server.
	OnPong(handler func(*Client, *HandlerContext) error) *Server
}

type Server struct {
	handler   *Handler
	config    *ServerConfig
	server    *http.Server
	isRunning bool
	mu        sync.RWMutex
}

// NewServer returns a new Server instance with default configuration.
func NewServer() *Server {
	return &Server{
		handler: NewHandler(),
		config:  DefaultServerConfig(),
		mu:      sync.RWMutex{},
	}
}

// ===== Fluent Interface =====

// WithPort sets the port number for the server to listen on. If the port is outside the valid range
// of 1-65535, a warning is printed to the console and the default port of 8080 is used instead.
func (s *Server) WithPort(port int) *Server {
	if s.config == nil {
		s.config = DefaultServerConfig()
	}

	if port <= 0 || port > 65535 {
		fmt.Printf("Warning: invalid port %d, using default 8080\n", port)
		port = 8080
	}

	s.config.Port = port
	return s
}

// WithPath sets the path for the server to listen on. If the path is empty, it defaults to "/". If the path does not start with a slash, it is prepended with one.
func (s *Server) WithPath(path string) *Server {
	if s.config == nil {
		s.config = DefaultServerConfig()
	}

	if path == "" {
		path = "/"
	}

	if path[0] != '/' {
		path = "/" + path
	}

	s.config.Path = path
	return s
}

// WithCORS sets whether the server should enable Cross-Origin Resource Sharing (CORS) for
// incoming requests. If enabled, the server will include the Access-Control-Allow-Origin
// header in all responses. Note that this is a simple implementation and does not handle
// preflighted requests or other advanced CORS features. See https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
// for more information.
func (s *Server) WithCORS(enabled bool) *Server {
	if s.config == nil {
		s.config = DefaultServerConfig()
	}
	s.config.EnableCORS = enabled
	return s
}

// WithSSL enables SSL/TLS for the server. The server will only serve requests
// over HTTPS if this method is called with a valid certificate and key file.
// If either the certificate or key file is empty, SSL/TLS will not be enabled.
// The certificate and key file should be in PEM format.
func (s *Server) WithSSL(certFile, keyFile string) *Server {
	if s.config == nil {
		s.config = DefaultServerConfig()
	}

	if certFile == "" || keyFile == "" {
		fmt.Println("Warning: certFile or keyFile is empty, SSL not enabled")
		return s
	}

	s.config.EnableSSL = true
	s.config.CertFile = certFile
	s.config.KeyFile = keyFile
	return s
}

// WithMaxConnections sets the maximum number of connections the server should accept.
// If max <= 0, it is set to the default of 1000.
func (s *Server) WithMaxConnections(max int) *Server {
	s.handler.WithMaxConnections(max)
	return s
}

// WithMessageSize sets the maximum message size for the server. If the size is less than or equal to 0, the default of 1024 is used.
func (s *Server) WithMessageSize(size int64) *Server {
	s.handler.WithMessageSize(size)
	return s
}

// WithTimeout sets the read and write timeouts for the server. If read is negative or write is negative,
// the timeouts are set to 0.
func (s *Server) WithTimeout(read, write time.Duration) *Server {
	s.handler.WithTimeout(read, write)
	return s
}

// WithPingPong sets the ping and pong wait periods for the server. If pingPeriod or pongWait are negative,
// the timeouts are set to 0.
func (s *Server) WithPingPong(pingPeriod, pongWait time.Duration) *Server {
	s.handler.WithPingPong(pingPeriod, pongWait)
	return s
}

// WithAllowedOrigins sets the allowed origins for the server. If the slice is empty,
// the server will allow any origin. Otherwise, the server will only allow the specified
// origins in the Origin header of incoming requests.
func (s *Server) WithAllowedOrigins(origins []string) *Server {
	s.handler.WithAllowedOrigins(origins)
	return s
}

// WithEncoding sets the default encoding for the server. This encoding will be used
// to encode messages sent to clients if no encoding is specified. If the encoding
// is not supported, the server will not start.
func (s *Server) WithEncoding(encoding EncodingType) *Server {
	s.handler.WithEncoding(encoding)
	return s
}

// WithSerializer sets the serializer for the server to use with the given encoding.
// If the serializer is nil, the server will not use this encoding.
// The server will use the default encoding if no encoding is specified.
func (s *Server) WithSerializer(encoding EncodingType, serializer Serializer) *Server {
	s.handler.WithSerializer(encoding, serializer)
	return s
}

// WithJSONSerializer sets the JSON serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (s *Server) WithJSONSerializer() *Server {
	return s.WithSerializer(JSON, JSONSerializer{})
}

// WithProtobufSerializer sets the Protobuf serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (s *Server) WithProtobufSerializer() *Server {
	return s.WithSerializer(Protobuf, ProtobufSerializer{})
}

// WithRawSerializer sets the Raw serializer for the server. This serializer
// is used to encode and decode messages sent to and from clients. If the
// serializer is nil, the server will not use this encoding.
func (s *Server) WithRawSerializer() *Server {
	return s.WithSerializer(Raw, RawSerializer{})
}

// WithMiddleware adds a middleware to the server. Middleware functions are executed
// before the OnConnect, OnMessage, and OnDisconnect handlers. They can be used to
// add authentication, logging, CORS support, or any other functionality that
// is needed.
func (s *Server) WithMiddleware(middleware Middleware) *Server {
	s.handler.WithMiddleware(middleware)
	return s
}

// WithAuth sets the authentication function for the server. The authentication
// function is called for each new connection to the server. It should return a
// map of user data and an error. If the error is not nil, the connection is
// closed. The user data is stored in the client's User field and can be accessed
// using the client.User() method.
func (s *Server) WithAuth(authFunc AuthFunc) *Server {
	s.handler.WithAuth(authFunc)
	return s
}

// ===== HANDLERS =====

// OnConnect sets the OnConnect handler for the server. This handler is called when
// a new client connects to the server. The handler should return an error if the
// connection should be closed. The handler is called after the authentication
// function has been called and the client has been added to the server's list of
// clients.
func (s *Server) OnConnect(handler func(*Client, *HandlerContext) error) *Server {
	s.handler.OnConnect(handler)
	return s
}

// OnDisconnect sets the OnDisconnect handler for the server. This handler is
// called when a client disconnects from the server. The handler should return an
// error if the disconnection should be treated as an error. The handler is called
// after the client has been removed from the server's list of clients.
func (s *Server) OnDisconnect(handler func(*Client, *HandlerContext) error) *Server {
	s.handler.OnDisconnect(handler)
	return s
}

// OnMessage sets the OnMessage handler for the server. This handler is called when
// a new message is received from a client. The handler should return an error if
// the message should be treated as an error. The handler is called after the
// message has been decoded and deserialized.
func (s *Server) OnMessage(handler func(*Client, *Message, *HandlerContext) error) *Server {
	s.handler.OnMessage(handler)
	return s
}

// OnRawMessage sets the OnRawMessage handler for the server. This handler is called
// when a new message is received from a client. The handler should return an error
// if the message should be treated as an error. The handler is called after the
// message has been decoded, but before it has been deserialized.
func (s *Server) OnRawMessage(handler func(*Client, []byte, *HandlerContext) error) *Server {
	s.handler.OnRawMessage(handler)
	return s
}

// OnJSONMessage sets the OnJSONMessage handler for the server. This handler is
// called when a new JSON message is received from a client. The handler should
// return an error if the message should be treated as an error. The handler is
// called after the message has been decoded and parsed as JSON.
func (s *Server) OnJSONMessage(handler func(*Client, interface{}, *HandlerContext) error) *Server {
	s.handler.OnJSONMessage(handler)
	return s
}

// OnProtobufMessage sets the OnProtobufMessage handler for the server. This
// handler is called when a new Protobuf message is received from a client. The
// handler should return an error if the message should be treated as an error.
// The handler is called after the message has been decoded and parsed as
// Protobuf.
func (s *Server) OnProtobufMessage(handler func(*Client, interface{}, *HandlerContext) error) *Server {
	s.handler.OnProtobufMessage(handler)
	return s
}

// OnError sets the OnError handler for the server. This handler is called when an
// error occurs. The handler should return an error if the error should be
// treated as an error. The handler is called with the client that caused the
// error and the error itself. The handler is called after the error has been
// logged.
func (s *Server) OnError(handler func(*Client, error, *HandlerContext) error) *Server {
	s.handler.OnError(handler)
	return s
}

// OnPing sets the OnPing handler for the server. This handler is called when a
// ping message is sent by a client. The handler should return an error if the
// ping should be treated as an error. The handler is called with the client that
// sent the ping.
func (s *Server) OnPing(handler func(*Client, *HandlerContext) error) *Server {
	s.handler.OnPing(handler)
	return s
}

// OnPong sets the OnPong handler for the server. This handler is called when a pong message is sent by a client.
// The handler should return an error if the pong should be treated as an error. The handler is called with the client that
// sent the pong.
func (s *Server) OnPong(handler func(*Client, *HandlerContext) error) *Server {
	s.handler.OnPong(handler)
	return s
}

// ===== CONTROLLERS =====

// Start starts the GoSocket server. It will start listening on the configured
// port and path, and will begin accepting connections. If the server is already
// running, this function will return an error. If the port is invalid, this
// function will return an error. If the path is empty, this function will use the
// default path of "/ws". If there are no serializers configured, this function
// will use the JSON serializer by default. The server will be stopped using the
// Stop function, or by calling the Close method on the underlying net.Listener.
// If the server is stopped, this function will return an error. If the server
// encounters an error, this function will return an error.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("server is already running")
	}

	if s.config.Port <= 0 || s.config.Port > 65535 {
		return fmt.Errorf("invalid port: %d", s.config.Port)
	}

	if s.config.Path == "" {
		// ? maybe don't check this? maybe the developer wants this path?
		// s.config.Path = "/ws"
	}

	if len(s.handler.serializers) <= 0 {
		s.WithJSONSerializer() // JSON as default serializer
	}

	go s.handler.hub.Run()

	mux := http.NewServeMux()

	mux.HandleFunc(s.config.Path, s.handler.HandleWebSocket)

	var handler http.Handler = mux
	handler = s.handler.ApplyMiddlewares(handler)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      handler,
		ReadTimeout:  s.handler.config.ReadTimeout,
		WriteTimeout: s.handler.config.WriteTimeout,
	}

	s.isRunning = true
	s.mu.Unlock()

	fmt.Printf("GoSocket server starting on port %d, path %s\n", s.config.Port, s.config.Path)

	var err error
	if s.config.EnableSSL {
		if s.config.CertFile == "" || s.config.KeyFile == "" {
			return fmt.Errorf("SSL enabled but cert/key files not provided")
		}
		err = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
	} else {
		err = s.server.ListenAndServe()
	}

	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()
	s.handler.hub.Stop()

	if err == http.ErrServerClosed {
		fmt.Println("GoSocket server stopped gracefully")
		return nil
	}

	if err != nil {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// StartWithContext starts the GoSocket server and returns an error. It will start listening on the configured
// port and path, and will begin accepting connections. If the server is already running, this function will
// return an error. If the port is invalid, this function will return an error. If the path is empty, this
// function will use the default path of "/ws". If there are no serializers configured, this function will use
// the JSON serializer by default. The server will be stopped using the Stop function, or by calling the Close
// method on the underlying net.Listener. If the server is stopped, this function will return an error. If the
// server encounters an error, this function will return an error. This function will also return an error if the
// provided context is canceled.
func (s *Server) StartWithContext(ctx context.Context) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("server is already running")
	}

	if s.config.Port <= 0 || s.config.Port > 65535 {
		return fmt.Errorf("invalid port: %d", s.config.Port)
	}

	if len(s.handler.serializers) <= 0 {
		s.WithJSONSerializer() // JSON as default serializer
	}

	go s.handler.hub.Run()

	mux := http.NewServeMux()

	mux.HandleFunc(s.config.Path, s.handler.HandleWebSocket)

	var handler http.Handler = mux
	handler = s.handler.ApplyMiddlewares(handler)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      handler,
		ReadTimeout:  s.handler.config.ReadTimeout,
		WriteTimeout: s.handler.config.WriteTimeout,
	}

	s.isRunning = true
	s.mu.Unlock()

	errChan := make(chan error, 1)

	go func() {
		fmt.Printf("GoSocket server starting on port %d, path %s\n", s.config.Port, s.config.Path)

		var err error
		if s.config.EnableSSL {
			if s.config.CertFile == "" || s.config.KeyFile == "" {
				errChan <- fmt.Errorf("SSL enabled but cert/key files not provided")
				return
			}
			err = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else {
			err = s.server.ListenAndServe()
		}

		if err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		// context canceled, shutdown graceful
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()

		if s.handler != nil && s.handler.hub != nil {
			s.handler.hub.Stop()
		}

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}

		fmt.Println("GoSocket server stopped by context")
		return ctx.Err()

	case err := <-errChan:
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
		s.handler.hub.Stop()

		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}
}

// Stop stops the GoSocket server and closes all active connections. This function will block until all active
// connections have been closed. If the server is not running, this function will return an error. If the server
// encounters an error while stopping, this function will return an error. This function will also return an error
// if the server's underlying net.Listener cannot be closed.
func (s *Server) Stop() error {
	s.mu.RLock()
	if !s.isRunning || s.server == nil {
		s.mu.RUnlock()
		return fmt.Errorf("server is not running")
	}
	s.mu.RUnlock()

	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()

	if s.handler != nil && s.handler.hub != nil {
		s.handler.hub.Stop()
	}

	return s.server.Close()
}

// StopGracefully stops the GoSocket server and closes all active connections gracefully. This function will block until the context
// times out or all active connections have been closed. If the server is not running, this function will return an error. If the server
// encounters an error while stopping, this function will return an error. This function will also return an error if the server's
// underlying net.Listener cannot be closed.
func (s *Server) StopGracefully(timeout time.Duration) error {
	s.mu.RLock()
	if !s.isRunning || s.server == nil {
		s.mu.RUnlock()
		return fmt.Errorf("server is not running")
	}
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()

	if s.handler != nil && s.handler.hub != nil {
		s.handler.hub.Stop()
	}

	return s.server.Shutdown(ctx)
}

// ===== BROADCASTING =====

// Broadcast sends a raw message to all connected clients. The message is sent as a
// websocket.TextMessage. If the server is not properly initialized, this function
// will return an error.
func (s *Server) Broadcast(message []byte) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	msg := NewRawMessage(TextMessage, message)
	s.BroadcastMessage(msg)

	return nil
}

// BroadcastMessage sends a Message to all connected clients. The message will be sent
// to each client according to the client's EncodingType. If the server is not properly
// initialized, this function will return an error.
func (s *Server) BroadcastMessage(message *Message) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	s.handler.hub.BroadcastMessage(message)
	return nil
}

// BroadcastData sends the given data to all connected clients. The data is serialized
// using the server's default encoding. If the default encoding is not set, the data
// will be sent as JSON. The data is sent as a websocket.TextMessage. If the server
// is not properly initialized, this function will return an error. If serialization
// fails, this function will return an error.
func (s *Server) BroadcastData(data interface{}) error {
	serializer := s.handler.serializers[s.handler.config.DefaultEncoding]
	if serializer == nil {
		// JSON fallback
		return s.BroadcastJSON(data)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	return s.Broadcast(rawData)
}

// BroadcastDataWithEncoding sends the given data to all connected clients using the
// specified encoding type. The data is serialized using the matching serializer. If
// the serializer is not found, an error is returned. If serialization fails, an
// error is returned. The data is sent as a websocket.BinaryMessage. If the server
// is not properly initialized, this function will return an error.
func (s *Server) BroadcastDataWithEncoding(data interface{}, encoding EncodingType) error {
	serializer := s.handler.serializers[encoding]
	if serializer == nil {
		return fmt.Errorf("serializer not found for encoding: %d", encoding)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	return s.Broadcast(rawData)
}

// BroadcastJSON sends the given data to all connected clients as JSON. The data is
// marshaled to JSON and sent as a websocket.TextMessage. If the marshaling fails,
// an error is returned. If the server is not properly initialized, this function
// will return an error.
func (s *Server) BroadcastJSON(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return s.Broadcast(jsonData)
}

// BroadcastProtobuf sends the given data to all connected clients as Protobuf. The data is
// marshaled to Protobuf and sent as a websocket.BinaryMessage. If the marshaling fails,
// an error is returned. If the server is not properly initialized, this function
// will return an error.
func (s *Server) BroadcastProtobuf(data interface{}) error {
	return nil
}

// BroadcastToRoom sends the given message to all clients in the specified room. The message
// is sent as a websocket.TextMessage. If the server is not properly initialized, this
// function will return an error.
func (s *Server) BroadcastToRoom(room string, message []byte) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	msg := NewRawMessage(TextMessage, message)
	msg.Room = room
	s.handler.hub.BroadcastToRoom(room, msg)

	return nil
}

// BroadcastToRoomData sends the given data to all clients in the specified room. The data
// is serialized using the server's default encoding. If the default encoding is not set,
// the data will be sent as JSON. The data is sent as a websocket.TextMessage. If the server
// is not properly initialized, this function will return an error. If serialization fails,
// this function will return an error.
func (s *Server) BroadcastToRoomData(room string, data interface{}) error {
	serializer := s.handler.serializers[s.handler.config.DefaultEncoding]
	if serializer == nil {
		return s.BroadcastToRoomJSON(room, data)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	return s.BroadcastToRoom(room, rawData)
}

// BroadcastToRoomJSON sends the given data to all clients in the specified room as JSON.
// The data is marshaled to JSON and sent as a websocket.TextMessage. If the marshaling
// fails, an error is returned. If the server is not properly initialized, this function
// will return an error.
func (s *Server) BroadcastToRoomJSON(room string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return s.BroadcastToRoom(room, jsonData)
}

// BroadcastToRoomProtobuf sends the given data to all clients in the specified room as Protobuf. The data
// is marshaled to Protobuf and sent as a websocket.BinaryMessage. If the marshaling fails, an error is
// returned. If the server is not properly initialized, this function will return an error.
func (s *Server) BroadcastToRoomProtobuf(room string, data interface{}) error {
	return nil
}

// ===== CLIENT MANAGEMENT =====

// GetClients returns all clients currently connected to the server. If the server is not properly
// initialized, an empty slice is returned.
func (s *Server) GetClients() []*Client {
	if s.handler == nil || s.handler.hub == nil {
		return []*Client{}
	}

	hubClients := s.handler.hub.GetClients()
	clients := make([]*Client, 0, len(hubClients))
	for client := range hubClients {
		clients = append(clients, client)
	}

	return clients
}

// GetClient returns a client by its ID. If no client with the given ID exists,
// nil is returned. This method is safe to call concurrently.
func (s *Server) GetClient(id string) *Client {
	clients := s.GetClients()
	for _, client := range clients {
		if client.ID == id {
			return client
		}
	}
	return nil
}

// GetClientsInRoom returns all clients in the specified room. If the server is not properly
// initialized or the room does not exist, an empty slice is returned. This method is safe to call
// concurrently.
func (s *Server) GetClientsInRoom(room string) []*Client {
	if s.handler == nil || s.handler.hub == nil {
		return []*Client{}
	}

	return s.handler.hub.GetRoomClients(room)
}

// GetClientCount returns the number of clients currently connected to the server. If the server is not properly
// initialized, 0 is returned. This method is safe to call concurrently.
func (s *Server) GetClientCount() int {
	if s.handler == nil || s.handler.hub == nil {
		return 0
	}

	return len(s.handler.hub.GetClients())
}

// DisconnectClient removes the client with the specified ID from its hub and closes its connection. It will
// return nil if the client is not connected to a hub or if the connection is nil. Otherwise, it will return
// the error from closing the connection. If the client is not found, an error is returned.
//
// This method is safe to call concurrently.
func (s *Server) DisconnectClient(id string) error {
	client := s.GetClient(id)
	if client == nil {
		return fmt.Errorf("client not found: %s", id)
	}

	return client.Disconnect()
}

// ===== ROOM MANAGEMENT =====

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the server is not properly initialized, an error is returned.
//
// Thismethod is safe to call concurrently.
func (s *Server) CreateRoom(name string) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	return s.handler.hub.CreateRoom(name)
}

// DeleteRoom deletes a room with the given name. If the room does not exist, it will return
// an error. If the server is not properly initialized, an error is returned.
//
// This method is safe to call concurrently.
func (s *Server) DeleteRoom(name string) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	return s.handler.hub.DeleteRoom(name)
}

// GetRooms returns all rooms currently on the server. If the server is not properly
// initialized, an empty slice is returned.
//
// This method is safe to call concurrently.
func (s *Server) GetRooms() []string {
	if s.handler == nil || s.handler.hub == nil {
		return []string{}
	}

	hubRooms := s.handler.hub.GetRooms()
	rooms := make([]string, 0, len(hubRooms))
	for roomName := range hubRooms {
		rooms = append(rooms, roomName)
	}

	return rooms
}

// JoinRoom joins the given client to the given room. It will return an error if the client does not exist.
// If the client exists, it will call the JoinRoom method on the client.
//
// This method is safe to call concurrently.
func (s *Server) JoinRoom(clientID, room string) error {
	client := s.GetClient(clientID)
	if client == nil {
		return fmt.Errorf("client not found: %s", clientID)
	}

	return client.JoinRoom(room)
}

// LeaveRoom removes the given client from the given room. It will return an error if the client does not exist.
//
// This method is safe to call concurrently.
func (s *Server) LeaveRoom(clientID, room string) error {
	client := s.GetClient(clientID)
	if client == nil {
		return fmt.Errorf("client not found: %s", clientID)
	}

	return client.LeaveRoom(room)
}
