// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
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
}

type Server struct {
	handler   *Handler
	config    *ServerConfig
	server    *http.Server
	isRunning bool
	mu        sync.RWMutex
}

// New returns a new Server instance with default configuration.
func NewServer(options ...UniversalOption) (*Server, error) {
	h, err := NewHandler()
	if err != nil {
		return nil, err
	}

	s := &Server{
		handler: h,
		config:  DefaultServerConfig(),
		mu:      sync.RWMutex{},
	}

	return s.With(options...)
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
func (s *Server) Start() (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("PANIC RECOVERED in Server.Start: %v\nStack trace:\n%s\n",
				r, string(debug.Stack()))
			err = fmt.Errorf("server start panic: %v", r)
		}

		if err != nil {
			err = newServerStoppedError(err)
		}
	}()

	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return ErrServerAlreadyRunning
	}

	if len(s.handler.Serializers()) <= 0 {
		s.handler.AddSerializer(JSON, JSONSerializer{}) // JSON as default serializer
	}

	hubCtx, hubCancel := context.WithCancel(context.Background())

	defer hubCancel()

	safeGoroutine("HubRun", func() {
		s.handler.Hub().Run(hubCtx)
	})

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handler.HandleWebSocket)

	var httpHandler http.Handler = mux
	httpHandler = s.handler.ApplyMiddlewares(httpHandler)

	s.server = s.buildHttpServer(httpHandler)
	s.isRunning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()

		hubCancel()

		if s.handler != nil && s.handler.Hub() != nil {
			s.handler.Hub().Stop()
		}
	}()

	fmt.Printf("GoSocket server starting on port %d, path %s\n", s.config.Port, s.config.Path)
	if s.config.EnableSSL {
		s.server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		err = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
	} else {
		err = s.server.ListenAndServe()
	}

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Println("GoSocket server stopped gracefully")
		return nil
	}

	return err
}

// StartWithContext starts the GoSocket server and returns an error. It will start listening on the configured
// port and path, and will begin accepting connections. If the server is already running, this function will
// return an error. If the port is invalid, this function will return an error. If the path is empty, this
// function will use the default path of "/ws". If there are no serializers configured, this function will use
// the JSON serializer by default. The server will be stopped using the Stop function, or by calling the Close
// method on the underlying net.Listener. If the server is stopped, this function will return an error. If the
// server encounters an error, this function will return an error. This function will also return an error if the
// provided context is canceled.
func (s *Server) StartWithContext(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			err = newServerStoppedError(err)
		}
	}()

	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return ErrServerAlreadyRunning
	}

	if len(s.handler.Serializers()) <= 0 {
		s.handler.AddSerializer(JSON, JSONSerializer{}) // JSON as default serializer
	}

	hubCtx, hubCancel := context.WithCancel(ctx)
	defer hubCancel()

	go s.handler.Hub().Run(hubCtx)

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handler.HandleWebSocket)

	var handler http.Handler = mux
	handler = s.handler.ApplyMiddlewares(handler)

	s.server = s.buildHttpServer(handler)
	s.isRunning = true
	s.mu.Unlock()

	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)

		fmt.Printf("GoSocket server starting on port %d, path %s\n", s.config.Port, s.config.Path)
		var serverErr error
		if s.config.EnableSSL {
			s.server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			serverErr = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else {
			serverErr = s.server.ListenAndServe()
		}

		errChan <- serverErr
	}()

	defer s.performGracefulShutdown(5*time.Second, hubCancel)

	select {
	case <-ctx.Done():
		// context canceled, shutdown graceful
		return ctx.Err()

	case err := <-errChan:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
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
		return ErrServerNotRunning
	}
	s.mu.RUnlock()

	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()

	if s.handler != nil && s.handler.Hub() != nil {
		s.handler.Hub().Stop()
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
		return ErrServerNotRunning
	}
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()

	if s.handler != nil && s.handler.Hub() != nil {
		s.handler.Hub().Stop()
	}

	return s.server.Shutdown(ctx)
}

// With applies the given options to the server. The options are applied in the
// order they are given, and if an option returns an error, the server will not
// be modified and the error will be returned. If no options are given, this
// function will return the server as is, with no error.
func (s *Server) With(options ...UniversalOption) (*Server, error) {
	for _, o := range options {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// ===== BROADCASTING =====

// Broadcast sends a raw message to all connected clients. The message is sent as a
// websocket.TextMessage. If the server is not properly initialized, this function
// will return an error.
func (s *Server) Broadcast(message []byte) error {
	if s.handler == nil || s.handler.Hub() == nil {
		return ErrServerNotInitialized
	}

	msg := NewRawMessage(TextMessage, message)
	return s.BroadcastMessage(msg)
}

// BroadcastMessage sends a Message to all connected clients. The message will be sent
// to each client according to the client's EncodingType. If the server is not properly
// initialized, this function will return an error.
func (s *Server) BroadcastMessage(message *Message) error {
	if s.handler == nil || s.handler.Hub() == nil {
		return ErrServerNotInitialized
	}

	s.handler.Hub().BroadcastMessage(message)
	return nil
}

// BroadcastData sends the given data to all connected clients. The data is serialized
// using the server's default encoding. If the default encoding is not set, the data
// will be sent as JSON. The data is sent as a websocket.TextMessage. If the server
// is not properly initialized, this function will return an error. If serialization
// fails, this function will return an error.
func (s *Server) BroadcastData(data interface{}) error {
	serializer := s.handler.Serializers()[s.handler.Config().DefaultEncoding]
	if serializer == nil {
		// JSON fallback
		return s.BroadcastJSON(data)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return newSerializeError(err)
	}

	return s.Broadcast(rawData)
}

// BroadcastDataWithEncoding sends the given data to all connected clients using the
// specified encoding type. The data is serialized using the matching serializer. If
// the serializer is not found, an error is returned. If serialization fails, an
// error is returned. The data is sent as a websocket.BinaryMessage. If the server
// is not properly initialized, this function will return an error.
func (s *Server) BroadcastDataWithEncoding(data interface{}, encoding EncodingType) error {
	serializer := s.handler.Serializers()[encoding]
	if serializer == nil {
		return newSerializerNotFoundError(encoding)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return newSerializeError(err)
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
		return newSerializeError(err)
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
	if s.handler == nil || s.handler.Hub() == nil {
		return ErrServerNotInitialized
	}

	msg := NewRawMessage(TextMessage, message)
	msg.Room = room
	s.handler.Hub().BroadcastToRoom(room, msg)

	return nil
}

// BroadcastToRoomData sends the given data to all clients in the specified room. The data
// is serialized using the server's default encoding. If the default encoding is not set,
// the data will be sent as JSON. The data is sent as a websocket.TextMessage. If the server
// is not properly initialized, this function will return an error. If serialization fails,
// this function will return an error.
func (s *Server) BroadcastToRoomData(room string, data interface{}) error {
	serializer := s.handler.Serializers()[s.handler.Config().DefaultEncoding]
	if serializer == nil {
		return s.BroadcastToRoomJSON(room, data)
	}

	rawData, err := serializer.Marshal(data)
	if err != nil {
		return newSerializeError(err)
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
		return newSerializeError(err)
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
	if s.handler == nil || s.handler.Hub() == nil {
		return []*Client{}
	}

	hubClients := s.handler.Hub().GetClients()
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
	if s.handler == nil || s.handler.Hub() == nil {
		return []*Client{}
	}

	return s.handler.Hub().GetRoomClients(room)
}

// GetClientCount returns the number of clients currently connected to the server. If the server is not properly
// initialized, 0 is returned. This method is safe to call concurrently.
func (s *Server) GetClientCount() int {
	if s.handler == nil || s.handler.Hub() == nil {
		return 0
	}

	return len(s.handler.Hub().GetClients())
}

// DisconnectClient removes the client with the specified ID from its hub and closes its connection. It will
// return nil if the client is not connected to a hub or if the connection is nil. Otherwise, it will return
// the error from closing the connection. If the client is not found, an error is returned.
//
// This method is safe to call concurrently.
func (s *Server) DisconnectClient(id string) error {
	client := s.GetClient(id)
	if client == nil {
		return newClientNotFoundError(id)
	}

	return client.Disconnect()
}

// ===== ROOM MANAGEMENT =====

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the server is not properly initialized, an error is returned.
//
// Thismethod is safe to call concurrently.
func (s *Server) CreateRoom(name string) error {
	if s.handler == nil || s.handler.Hub() == nil {
		return ErrServerNotInitialized
	}

	return s.handler.Hub().CreateRoom(name)
}

// DeleteRoom deletes a room with the given name. If the room does not exist, it will return
// an error. If the server is not properly initialized, an error is returned.
//
// This method is safe to call concurrently.
func (s *Server) DeleteRoom(name string) error {
	if s.handler == nil || s.handler.Hub() == nil {
		return ErrServerNotInitialized
	}

	return s.handler.Hub().DeleteRoom(name)
}

// GetRooms returns all rooms currently on the server. If the server is not properly
// initialized, an empty slice is returned.
//
// This method is safe to call concurrently.
func (s *Server) GetRooms() []string {
	if s.handler == nil || s.handler.Hub() == nil {
		return []string{}
	}

	hubRooms := s.handler.Hub().GetRooms()
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
		return newClientNotFoundError(clientID)
	}

	return client.JoinRoom(room)
}

// LeaveRoom removes the given client from the given room. It will return an error if the client does not exist.
//
// This method is safe to call concurrently.
func (s *Server) LeaveRoom(clientID, room string) error {
	client := s.GetClient(clientID)
	if client == nil {
		return newClientNotFoundError(clientID)
	}

	return client.LeaveRoom(room)
}

func (s *Server) Handler() *Handler {
	return s.handler
}

func (s *Server) buildHttpServer(httpHandler http.Handler) *http.Server {
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      httpHandler,
		ReadTimeout:  s.handler.Config().ReadTimeout,
		WriteTimeout: s.handler.Config().WriteTimeout,
	}

	if s.config.EnableSSL {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return server
}

func (s *Server) performGracefulShutdown(timeout time.Duration, hubCancel context.CancelFunc) {
	s.mu.Lock()
	wasRunning := s.isRunning
	s.isRunning = false
	s.mu.Unlock()

	if !wasRunning {
		return
	}

	hubCancel()

	if s.handler != nil && s.handler.Hub() != nil {
		s.notifyClientsShutdown()
	}

	if s.handler != nil && s.handler.Hub() != nil {
		s.handler.Hub().Stop()
	}

	if s.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := s.server.Shutdown(shutdownCtx); err != nil {
			_ = s.server.Close()
		}
	}
}

func (s *Server) notifyClientsShutdown() {
	if s.handler == nil || s.handler.Hub() == nil {
		return
	}

	clients := s.handler.Hub().GetClients()
	shutdownMessage := map[string]interface{}{
		"type":      "server_shutdown",
		"message":   "Server is shutting down",
		"timestamp": time.Now(),
	}

	msg := NewMessage(TextMessage, shutdownMessage)
	msg.Encoding = JSON

	for client := range clients {
		go func(c *Client) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			select {
			case c.MessageChan <- func() []byte {
				data, _ := json.Marshal(shutdownMessage)
				return data
			}():
			case <-ctx.Done():
				c.Disconnect()
			}
		}(client)
	}

	// give clients time to receive the shutdown message
	time.Sleep(500 * time.Millisecond)
}

// ===== Functional Options =====

// WithPort sets the port number for the server to listen on. If the port is outside the valid range
// of 1-65535, a warning is printed to the console and the default port of 8080 is used instead.
func WithPort(port int) UniversalOption {
	return func(h HasHandler) error {
		server, ok := h.(*Server)
		if !ok {
			return newWithOnlyServerError("WithPort", h)
		}

		if port <= 0 || port > 65535 {
			return newInvalidPortError(port)
		}

		server.config.Port = port
		return nil
	}
}

// WithPath sets the path for the server to listen on. If the path is empty, it defaults to "/". If the path does not start with a slash, it is prepended with one.
func WithPath(path string) UniversalOption {
	return func(h HasHandler) error {
		server, ok := h.(*Server)
		if !ok {
			return newWithOnlyServerError("WithPath", h)
		}

		if path == "" {
			path = "/"
		}

		if path[0] != '/' {
			path = "/" + path
		}

		server.config.Path = path
		return nil
	}
}

// WithCORS sets whether the server should enable Cross-Origin Resource Sharing (CORS) for
// incoming requests. If enabled, the server will include the Access-Control-Allow-Origin
// header in all responses. Note that this is a simple implementation and does not handle
// preflighted requests or other advanced CORS features. See https://developer.mozilla.org/en-US/docs/Web/HTTP/CORS
// for more information.
func WithCORS(enabled bool) UniversalOption {
	return func(h HasHandler) error {
		server, ok := h.(*Server)
		if !ok {
			return newWithOnlyServerError("WithCORS", h)
		}

		server.config.EnableCORS = enabled
		return nil
	}
}

// WithSSL enables SSL/TLS for the server. The server will only serve requests
// over HTTPS if this method is called with a valid certificate and key file.
// If either the certificate or key file is empty, SSL/TLS will not be enabled.
// The certificate and key file should be in PEM format.
func WithSSL(certFile, keyFile string) UniversalOption {
	return func(h HasHandler) error {
		server, ok := h.(*Server)
		if !ok {
			return newWithOnlyServerError("WithSSL", h)
		}

		if certFile == "" || keyFile == "" {
			return ErrSSLFilesEmpty
		}

		server.config.EnableSSL = true
		server.config.CertFile = certFile
		server.config.KeyFile = keyFile
		return nil
	}
}
