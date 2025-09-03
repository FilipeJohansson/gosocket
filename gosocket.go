package gosocket

import (
	"context"
	"net/http"
	"time"
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
