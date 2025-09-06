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

func (h *Handler) WithMaxConnections(max int) *Handler {
	if max <= 0 {
		fmt.Println("Warning: max connections must be greater than 0, setting to default 1000")
		max = 1000
	}

	h.config.MaxConnections = max
	return h
}

func (h *Handler) WithMessageSize(size int64) *Handler {
	if size <= 0 {
		fmt.Println("Warning: message size must be greater than 0, setting to default 1024")
		size = 1024
	}

	h.config.MessageSize = size
	return h
}

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

func (h *Handler) WithAllowedOrigins(origins []string) *Handler {
	h.config.AllowedOrigins = origins
	return h
}

func (h *Handler) WithEncoding(encoding EncodingType) *Handler {
	h.config.DefaultEncoding = encoding
	return h
}

func (h *Handler) WithSerializer(encoding EncodingType, serializer Serializer) *Handler {
	if h.serializers == nil {
		h.serializers = make(map[EncodingType]Serializer)
	}
	h.serializers[encoding] = serializer
	return h
}

func (h *Handler) WithJSONSerializer() *Handler {
	return h.WithSerializer(JSON, JSONSerializer{})
}

func (h *Handler) WithProtobufSerializer() *Handler {
	return h.WithSerializer(Protobuf, ProtobufSerializer{})
}

func (h *Handler) WithRawSerializer() *Handler {
	return h.WithSerializer(Raw, RawSerializer{})
}

func (h *Handler) WithMiddleware(middleware Middleware) *Handler {
	if h.middlewares == nil {
		h.middlewares = make([]Middleware, 0)
	}
	h.middlewares = append(h.middlewares, middleware)
	return h
}

func (h *Handler) WithAuth(authFunc AuthFunc) *Handler {
	h.authFunc = authFunc
	return h
}

// ===== HANDLERS =====

func (h *Handler) OnConnect(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnConnect = handler
	return h
}

func (h *Handler) OnDisconnect(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnDisconnect = handler
	return h
}

func (h *Handler) OnMessage(handler func(*Client, *Message) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnMessage = handler
	return h
}

func (h *Handler) OnRawMessage(handler func(*Client, []byte) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnRawMessage = handler
	return h
}

func (h *Handler) OnJSONMessage(handler func(*Client, interface{}) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnJSONMessage = handler
	return h
}

func (h *Handler) OnProtobufMessage(handler func(*Client, interface{}) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnProtobufMessage = handler
	return h
}

func (h *Handler) OnError(handler func(*Client, error) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnError = handler
	return h
}

func (h *Handler) OnPing(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnPing = handler
	return h
}

func (h *Handler) OnPong(handler func(*Client) error) *Handler {
	if h.handlers == nil {
		h.handlers = &Handlers{}
	}
	h.handlers.OnPong = handler
	return h
}

// ===== CONTROLLERS =====

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

func (h *Handler) ensureHubRunning() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hubRunning.Do(func() {
		go h.hub.Run()
	})
}
