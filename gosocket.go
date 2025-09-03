package gosocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// every events handlers
type Handlers struct {
	OnConnect    func(*Client)
	OnDisconnect func(*Client)
	OnMessage    func(*Client, *Message)              // generic handler
	OnRawMessage func(*Client, []byte)                // raw data handler
	OnJSONMessage func(*Client, interface{})          // JSON specific handler
	OnProtobufMessage func(*Client, interface{})      // Protobuf specific handler
	OnError      func(*Client, error)
	OnPing       func(*Client)
	OnPong       func(*Client)
}

// main WebSocket struct
type Server struct {
	hub        *Hub
	config     *Config
	handlers   *Handlers
	server     *http.Server
	serializers map[EncodingType]Serializer
	isRunning   bool
	authFunc AuthFunc
}

func New() *Server {
	config := DefaultConfig()
	
	return &Server{
		hub:         NewHub(),
		config:      config,
		handlers:    &Handlers{},
		serializers: make(map[EncodingType]Serializer),
		isRunning:   false,
	}
}

// ===== Fluent Interface =====

func (s *Server) WithPort(port int) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.Port = port
	return s
}

func (s *Server) WithPath(path string) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.Path = path
	return s
}

func (s *Server) WithCORS(enabled bool) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.EnableCORS = enabled
	return s
}

func (s *Server) WithMaxConnections(max int) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.MaxConnections = max
	return s
}

func (s *Server) WithMessageSize(size int64) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.MessageSize = size
	return s
}

func (s *Server) WithTimeout(read, write time.Duration) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.ReadTimeout = read
	s.config.WriteTimeout = write
	return s
}

func (s *Server) WithPingPong(pingPeriod, pongWait time.Duration) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.PingPeriod = pingPeriod
	s.config.PongWait = pongWait
	return s
}

func (s *Server) WithSSL(certFile, keyFile string) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.EnableSSL = true
	s.config.CertFile = certFile
	s.config.KeyFile = keyFile
	return s
}

func (s *Server) WithAllowedOrigins(origins []string) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.AllowedOrigins = origins
	return s
}

func (s *Server) WithEncoding(encoding EncodingType) *Server {
	if s.config == nil {
		s.config = DefaultConfig()
	}
	s.config.DefaultEncoding = encoding
	return s
}

func (s *Server) WithSerializer(encoding EncodingType, serializer Serializer) *Server {
	if s.serializers == nil {
		s.serializers = make(map[EncodingType]Serializer)
	}
	s.serializers[encoding] = serializer
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
	return s
}

func (s *Server) WithAuth(authFunc AuthFunc) *Server {
	return s
}

// ===== HANDLERS =====

func (s *Server) OnConnect(handler func(*Client)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnConnect = handler
	return s
}

func (s *Server) OnDisconnect(handler func(*Client)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnDisconnect = handler
	return s
}

func (s *Server) OnMessage(handler func(*Client, *Message)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnMessage = handler
	return s
}

func (s *Server) OnRawMessage(handler func(*Client, []byte)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnRawMessage = handler
	return s
}

func (s *Server) OnJSONMessage(handler func(*Client, interface{})) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnJSONMessage = handler
	return s
}

func (s *Server) OnProtobufMessage(handler func(*Client, interface{})) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnProtobufMessage = handler
	return s
}

func (s *Server) OnError(handler func(*Client, error)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnError = handler
	return s
}

func (s *Server) OnPing(handler func(*Client)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnPing = handler
	return s
}

func (s *Server) OnPong(handler func(*Client)) *Server {
	if s.handlers == nil {
		s.handlers = &Handlers{}
	}
	s.handlers.OnPong = handler
	return s
}

// ===== CONTROLLERS =====

func (s *Server) Start() error {
	if s.isRunning {
		return fmt.Errorf("server is already running")
	}

	if s.config.Port <= 0 || s.config.Port > 65535 {
		return fmt.Errorf("invalid port: %d", s.config.Port)
	}

	if s.config.Path == "" {
		// ? maybe don't check this? maybe the developer wants this path?
		// s.config.Path = "/ws"
	}

	if len(s.serializers) <= 0 {
		s.WithJSONSerializer() // JSON as default serializer
	}

	go s.hub.Run()

	mux := http.NewServeMux()

	mux.HandleFunc(s.config.Path, s.handleWebSocket)

	var handler http.Handler = mux
	// TODO: apply in chain middlewares

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	s.isRunning = true

	fmt.Printf("GoSocket server starting on port %d, path %s\n", s.config.Port, s.config.Path)

	var err error
	// if s.config.EnableSSL {
	// 	if s.config.CertFile == "" || s.config.KeyFile == "" {
	// 		return fmt.Errorf("SSL enabled but cert/key files not provided")
	// 	}
	// 	err = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
	// } else {
	err = s.server.ListenAndServe()
	// }

	s.isRunning = false
	s.hub.Stop()

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
	return nil
}

func (s *Server) Stop() error {
	return nil
}

func (s *Server) StopGracefully(timeout time.Duration) error {
	return nil
}

// ===== BROADCASTING =====

func (s *Server) Broadcast(message []byte) error {
	return nil
}

func (s *Server) BroadcastMessage(message *Message) error {
	return nil
}

func (s *Server) BroadcastData(data interface{}) error {
	return nil
}

func (s *Server) BroadcastDataWithEncoding(data interface{}, encoding EncodingType) error {
	return nil
}

func (s *Server) BroadcastJSON(data interface{}) error {
	return nil
}

func (s *Server) BroadcastProtobuf(data interface{}) error {
	return nil
}

func (s *Server) BroadcastToRoom(room string, message []byte) error {
	return nil
}

func (s *Server) BroadcastToRoomData(room string, data interface{}) error {
	return nil
}

func (s *Server) BroadcastToRoomJSON(room string, data interface{}) error {
	return nil
}

func (s *Server) BroadcastToRoomProtobuf(room string, data interface{}) error {
	return nil
}

// ===== CLIENT MANAGEMENT =====

func (s *Server) GetClients() []*Client {
	return nil
}

func (s *Server) GetClient(id string) *Client {
	return nil
}

func (s *Server) GetClientsInRoom(room string) []*Client {
	return nil
}

func (s *Server) GetClientCount() int {
	return 0
}

func (s *Server) DisconnectClient(id string) error {
	return nil
}

// ===== ROOM MANAGEMENT =====

func (s *Server) CreateRoom(name string) error {
	return nil
}

func (s *Server) DeleteRoom(name string) error {
	return nil
}

func (s *Server) GetRooms() []string {
	return nil
}

func (s *Server) JoinRoom(clientID, room string) error {
	return nil
}

func (s *Server) LeaveRoom(clientID, room string) error {
	return nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		ReadBufferSize: 1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if len(s.config.AllowedOrigins) == 0 {
				return s.config.EnableCORS
			}

			origin := r.Header.Get("Origin")
			for _, allowed := range s.config.AllowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
	}

	var userData map[string]interface{}
	if s.authFunc != nil {
		var err error
		userData, err = s.authFunc(r)
		if err != nil {
			http.Error(w, "Authentication failed: " + err.Error(), http.StatusUnauthorized)
			return
		}
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if s.handlers.OnError != nil {
			s.handlers.OnError(nil, fmt.Errorf("websocket upgrade failed: %w", err))
		}
		return
	}

	clientId := generateClientID()
	client := NewClient(clientId, conn, s.hub)

	for key, value := range userData {
		client.SetUserData(key, value)
	}

	conn.SetReadLimit(s.config.MessageSize)
	conn.SetReadDeadline(time.Now().Add(s.config.PongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(s.config.PongWait))
		if s.handlers.OnPong != nil {
			s.handlers.OnPong(client)
		}
		return nil
	})

	s.hub.Register <- client

	if s.handlers.OnConnect != nil {
		s.handlers.OnConnect(client)
	}

	go s.handleClientWrite(client)
	go s.handleClientRead(client)
}

func (s *Server) handleClientWrite(client *Client) {
	ticker := time.NewTicker(s.config.PingPeriod)
	defer func()  {
		ticker.Stop()
		client.Conn.(*websocket.Conn).Close()
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		select {
		case message, ok := <-client.MessageChan:
			conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				if s.handlers.OnError != nil {
					s.handlers.OnError(client, err)
				}
				return
			}
		
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

			if s.handlers.OnPing != nil {
				s.handlers.OnPing(client)
			}
		}
	}
}

func (s *Server) handleClientRead(client *Client) {
	defer func()  {
		s.hub.Unregister <- client
		client.Conn.(*websocket.Conn).Close()

		if s.handlers.OnDisconnect != nil {
			s.handlers.OnDisconnect(client)
		}
	}()

	conn := client.Conn.(*websocket.Conn)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if s.handlers.OnError != nil {
					s.handlers.OnError(client, err)
				}
			}
			break
		}

		conn.SetReadDeadline(time.Now().Add(s.config.PongWait)) // reset read deadline

		message := &Message{
			Type: MessageType(messageType),
			RawData: data,
			From: client.ID,
			Created: time.Now(),
		}

		s.processMessage(client, message)
	}
}

func (s *Server) processMessage(client *Client, message *Message) {
	if s.handlers.OnMessage != nil {
		s.handlers.OnMessage(client, message)
	}

	if s.handlers.OnRawMessage != nil {
		s.handlers.OnRawMessage(client, message.RawData)
	}

	if s.handlers.OnJSONMessage != nil {
		var jsonData interface{}
		if err := json.Unmarshal(message.RawData, &jsonData); err == nil {
			message.Data = jsonData
			message.Encoding = JSON
			s.handlers.OnJSONMessage(client, jsonData)
		}
	}

	s.hub.BroadcastMessage(message) //! just to test, remove this!

	// TODO: add Protobuf and other formats
}

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}

