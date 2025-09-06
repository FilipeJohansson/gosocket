package gosocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type IServer interface {
	Start() error
	StartWithContext(ctx context.Context) error
	Stop() error
	StopGracefully(timeout time.Duration) error

	Broadcast(message []byte) error
	BroadcastMessage(message *Message) error
	BroadcastData(data interface{}) error
	BroadcastDataWithEncoding(data interface{}, encoding EncodingType) error
	BroadcastJSON(data interface{}) error
	BroadcastProtobuf(data interface{}) error
	BroadcastToRoom(room string, message []byte) error
	BroadcastToRoomData(room string, data interface{}) error
	BroadcastToRoomJSON(room string, data interface{}) error
	BroadcastToRoomProtobuf(room string, data interface{}) error

	GetClients() []*Client
	GetClient(id string) *Client
	GetClientsInRoom(room string) []*Client
	GetClientCount() int
	DisconnectClient(id string) error

	CreateRoom(name string) error
	DeleteRoom(name string) error
	GetRooms() []string
	JoinRoom(clientID, room string) error
	LeaveRoom(clientID, room string) error

	// Configuration methods
	WithPort(port int) *Server
	WithPath(path string) *Server
	WithCORS(enabled bool) *Server
	WithSSL(certFile, keyFile string) *Server
	WithMaxConnections(max int) *Server
	WithMessageSize(size int64) *Server
	WithTimeout(read, write time.Duration) *Server
	WithPingPong(pingPeriod, pongWait time.Duration) *Server
	WithAllowedOrigins(origins []string) *Server
	WithEncoding(encoding EncodingType) *Server
	WithSerializer(encoding EncodingType, serializer Serializer) *Server
	WithJSONSerializer() *Server
	WithProtobufSerializer() *Server
	WithRawSerializer() *Server
	WithMiddleware(middleware Middleware) *Server
	WithAuth(authFunc AuthFunc) *Server

	// Event handlers
	OnConnect(handler func(*Client) error) *Server
	OnDisconnect(handler func(*Client) error) *Server
	OnMessage(handler func(*Client, *Message) error) *Server
	OnRawMessage(handler func(*Client, []byte) error) *Server
	OnJSONMessage(handler func(*Client, interface{}) error) *Server
	OnProtobufMessage(handler func(*Client, interface{}) error) *Server
	OnError(handler func(*Client, error) error) *Server
	OnPing(handler func(*Client) error) *Server
	OnPong(handler func(*Client) error) *Server
}

type Server struct {
	handler   *Handler
	config    *ServerConfig
	server    *http.Server
	isRunning bool
	mu        sync.RWMutex
}

func NewServer() *Server {
	return &Server{
		handler: NewHandler(),
		config:  DefaultServerConfig(),
		mu:      sync.RWMutex{},
	}
}

// ===== Fluent Interface =====

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

func (s *Server) WithCORS(enabled bool) *Server {
	if s.config == nil {
		s.config = DefaultServerConfig()
	}
	s.config.EnableCORS = enabled
	return s
}

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

func (s *Server) WithMaxConnections(max int) *Server {
	s.handler.WithMaxConnections(max)
	return s
}

func (s *Server) WithMessageSize(size int64) *Server {
	s.handler.WithMessageSize(size)
	return s
}

func (s *Server) WithTimeout(read, write time.Duration) *Server {
	s.handler.WithTimeout(read, write)
	return s
}

func (s *Server) WithPingPong(pingPeriod, pongWait time.Duration) *Server {
	s.handler.WithPingPong(pingPeriod, pongWait)
	return s
}

func (s *Server) WithAllowedOrigins(origins []string) *Server {
	s.handler.WithAllowedOrigins(origins)
	return s
}

func (s *Server) WithEncoding(encoding EncodingType) *Server {
	s.handler.WithEncoding(encoding)
	return s
}

func (s *Server) WithSerializer(encoding EncodingType, serializer Serializer) *Server {
	s.handler.WithSerializer(encoding, serializer)
	return s
}

func (s *Server) WithJSONSerializer() *Server {
	return s.WithSerializer(JSON, JSONSerializer{})
}

func (s *Server) WithProtobufSerializer() *Server {
	return s.WithSerializer(Protobuf, ProtobufSerializer{})
}

func (s *Server) WithRawSerializer() *Server {
	return s.WithSerializer(Raw, RawSerializer{})
}

func (s *Server) WithMiddleware(middleware Middleware) *Server {
	s.handler.WithMiddleware(middleware)
	return s
}

func (s *Server) WithAuth(authFunc AuthFunc) *Server {
	s.handler.WithAuth(authFunc)
	return s
}

// ===== HANDLERS =====

func (s *Server) OnConnect(handler func(*Client) error) *Server {
	s.handler.OnConnect(handler)
	return s
}

func (s *Server) OnDisconnect(handler func(*Client) error) *Server {
	s.handler.OnDisconnect(handler)
	return s
}

func (s *Server) OnMessage(handler func(*Client, *Message) error) *Server {
	s.handler.OnMessage(handler)
	return s
}

func (s *Server) OnRawMessage(handler func(*Client, []byte) error) *Server {
	s.handler.OnRawMessage(handler)
	return s
}

func (s *Server) OnJSONMessage(handler func(*Client, interface{}) error) *Server {
	s.handler.OnJSONMessage(handler)
	return s
}

func (s *Server) OnProtobufMessage(handler func(*Client, interface{}) error) *Server {
	s.handler.OnProtobufMessage(handler)
	return s
}

func (s *Server) OnError(handler func(*Client, error) error) *Server {
	s.handler.OnError(handler)
	return s
}

func (s *Server) OnPing(handler func(*Client) error) *Server {
	s.handler.OnPing(handler)
	return s
}

func (s *Server) OnPong(handler func(*Client) error) *Server {
	s.handler.OnPong(handler)
	return s
}

// ===== CONTROLLERS =====

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

	mux.HandleFunc(s.config.Path, s.handleWebSocket)

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

	mux.HandleFunc(s.config.Path, s.handleWebSocket)

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

func (s *Server) Broadcast(message []byte) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	msg := NewRawMessage(TextMessage, message)
	s.BroadcastMessage(msg)

	return nil
}

func (s *Server) BroadcastMessage(message *Message) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	s.handler.hub.BroadcastMessage(message)
	return nil
}

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

func (s *Server) BroadcastJSON(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return s.Broadcast(jsonData)
}

func (s *Server) BroadcastProtobuf(data interface{}) error {
	return nil
}

func (s *Server) BroadcastToRoom(room string, message []byte) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	msg := NewRawMessage(TextMessage, message)
	msg.Room = room
	s.handler.hub.BroadcastToRoom(room, msg)

	return nil
}

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

func (s *Server) BroadcastToRoomJSON(room string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return s.BroadcastToRoom(room, jsonData)
}

func (s *Server) BroadcastToRoomProtobuf(room string, data interface{}) error {
	return nil
}

// ===== CLIENT MANAGEMENT =====

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

func (s *Server) GetClient(id string) *Client {
	clients := s.GetClients()
	for _, client := range clients {
		if client.ID == id {
			return client
		}
	}
	return nil
}

func (s *Server) GetClientsInRoom(room string) []*Client {
	if s.handler == nil || s.handler.hub == nil {
		return []*Client{}
	}

	return s.handler.hub.GetRoomClients(room)
}

func (s *Server) GetClientCount() int {
	if s.handler == nil || s.handler.hub == nil {
		return 0
	}

	return len(s.handler.hub.GetClients())
}

func (s *Server) DisconnectClient(id string) error {
	client := s.GetClient(id)
	if client == nil {
		return fmt.Errorf("client not found: %s", id)
	}

	return client.Disconnect()
}

// ===== ROOM MANAGEMENT =====

func (s *Server) CreateRoom(name string) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	return s.handler.hub.CreateRoom(name)
}

func (s *Server) DeleteRoom(name string) error {
	if s.handler == nil || s.handler.hub == nil {
		return fmt.Errorf("server not properly initialized")
	}

	return s.handler.hub.DeleteRoom(name)
}

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

func (s *Server) JoinRoom(clientID, room string) error {
	client := s.GetClient(clientID)
	if client == nil {
		return fmt.Errorf("client not found: %s", clientID)
	}

	return client.JoinRoom(room)
}

func (s *Server) LeaveRoom(clientID, room string) error {
	client := s.GetClient(clientID)
	if client == nil {
		return fmt.Errorf("client not found: %s", clientID)
	}

	return client.LeaveRoom(room)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if len(s.handler.config.AllowedOrigins) == 0 {
				return s.config.EnableCORS
			}

			origin := r.Header.Get("Origin")
			for _, allowed := range s.handler.config.AllowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
	}

	var userData map[string]interface{}
	if s.handler.authFunc != nil {
		var err error
		userData, err = s.handler.authFunc(r)
		if err != nil {
			http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if s.handler.handlers.OnError != nil {
			s.handler.handlers.OnError(nil, fmt.Errorf("websocket upgrade failed: %w", err))
		}
		return
	}

	clientId := generateClientID()
	client := NewClient(clientId, conn, s.handler.hub)

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(s.handler.config.MessageSize)
	conn.SetReadDeadline(time.Now().Add(s.handler.config.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(s.handler.config.PongWait))
		if s.handler.handlers.OnPong != nil {
			s.handler.handlers.OnPong(client)
		}
		return nil
	})

	s.handler.hub.AddClient(client)

	if s.handler.handlers.OnConnect != nil {
		s.handler.handlers.OnConnect(client)
	}

	go s.handleClientWrite(client)
	go s.handleClientRead(client)
}

func (s *Server) handleClientWrite(client *Client) {
	ticker := time.NewTicker(s.handler.config.PingPeriod)
	defer func() {
		ticker.Stop()
		client.Conn.(*websocket.Conn).Close()
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		select {
		case message, ok := <-client.MessageChan:
			conn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				if s.handler.handlers.OnError != nil {
					s.handler.handlers.OnError(client, err)
				}
				return
			}

		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(s.handler.config.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

			if s.handler.handlers.OnPing != nil {
				s.handler.handlers.OnPing(client)
			}
		}
	}
}

func (s *Server) handleClientRead(client *Client) {
	defer func() {
		s.handler.hub.RemoveClient(client)
		client.Conn.(*websocket.Conn).Close()

		if s.handler.handlers.OnDisconnect != nil {
			s.handler.handlers.OnDisconnect(client)
		}
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if s.handler.handlers.OnError != nil {
					s.handler.handlers.OnError(client, err)
				}
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(s.handler.config.PongWait)) // reset read deadline

		message := &Message{
			Type:    MessageType(messageType),
			RawData: data,
			From:    client.ID,
			Created: time.Now(),
		}

		s.processMessage(client, message)
	}
}

func (s *Server) processMessage(client *Client, message *Message) {
	if s.handler.handlers.OnMessage != nil {
		if err := s.handler.handlers.OnMessage(client, message); err != nil {
			if s.handler.handlers.OnError != nil {
				s.handler.handlers.OnError(client, err)
			}
		}
	}

	if s.handler.handlers.OnRawMessage != nil {
		if err := s.handler.handlers.OnRawMessage(client, message.RawData); err != nil {
			if s.handler.handlers.OnError != nil {
				s.handler.handlers.OnError(client, err)
			}
		}
	}

	if s.handler.handlers.OnJSONMessage != nil {
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); err == nil {
			message.Data = jsonData
			message.Encoding = JSON
			if err := s.handler.handlers.OnJSONMessage(client, jsonData); err != nil {
				if s.handler.handlers.OnError != nil {
					s.handler.handlers.OnError(client, err)
				}
			}
		}
	}
	// TODO: add Protobuf and other formats
}
