// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

/**
 * gosocket.go is the public facade of the GoSocket library.
 * It exposes the main entry points such as NewServer, NewHandler,
 * and other user-facing constructors.
 *
 * This file is responsible for translating public options into
 * internal runtime configuration and wiring the correct bootstrap
 * (server or handler) without exposing internal implementation details.
 *
 * This file must be the only public entry point for creating GoSocket
 * instances and should remain stable over time.
 *
 * MUST NOT contain business logic, state management, cluster logic,
 * dispatcher logic, or direct networking code.
 */

import (
	"context"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/cluster/backends"
	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/runtime"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/FilipeJohansson/gosocket/internal/transport/websocket"
)

// ===== TYPE ALIASES =====
// * Websocket
type Context = websocket.Context
type OnStartFunc = websocket.OnStartFunc
type OnBeforeConnectFunc = websocket.OnBeforeConnectFunc
type OnConnectFunc = websocket.OnConnectFunc
type OnDisconnectFunc = websocket.OnDisconnectFunc
type OnMessageFunc = websocket.OnMessageFunc
type OnRawMessageFunc = websocket.OnRawMessageFunc
type OnJSONMessageFunc = websocket.OnJSONMessageFunc
type OnErrorFunc = websocket.OnErrorFunc
type OnPingFunc = websocket.OnPingFunc
type OnPongFunc = websocket.OnPongFunc

// * Message
type Message = message.Message
type MessageType = message.MessageType
type EncodingType = message.EncodingType

const (
	TextMessage   = message.TextMessage
	BinaryMessage = message.BinaryMessage
	CloseMessage  = message.CloseMessage
	PingMessage   = message.PingMessage
	PongMessage   = message.PongMessage

	JSON = message.JSON
	Raw  = message.Raw
)

// * Hub
type Hub = hub.Hub
type Client = hub.Client
type Room = hub.Room

// * Logger
type LogType = logger.LogType
type LogLevel = logger.LogLevel

type Logger = logger.Logger
type LoggerConfig = logger.LoggerConfig

type DefaultLogger = logger.DefaultLogger
type NullLogger = logger.NullLogger

const (
	LogTypeServer     = logger.LogTypeServer
	LogTypeClient     = logger.LogTypeClient
	LogTypeAuth       = logger.LogTypeAuth
	LogTypeBroadcast  = logger.LogTypeBroadcast
	LogTypeConnection = logger.LogTypeConnection
	LogTypeMessage    = logger.LogTypeMessage
	LogTypeError      = logger.LogTypeError
	LogTypeRateLimit  = logger.LogTypeRateLimit
	LogTypeRoom       = logger.LogTypeRoom
	LogTypeOther      = logger.LogTypeOther
)

const (
	LogLevelNone  = logger.LogLevelNone
	LogLevelError = logger.LogLevelError
	LogLevelWarn  = logger.LogLevelWarn
	LogLevelInfo  = logger.LogLevelInfo
	LogLevelDebug = logger.LogLevelDebug
)

// * Connection Pool
type ConnectionPoolConfig = transport.ConnectionPoolConfig

func NewMessage(msgType MessageType, data interface{}) *Message {
	return message.NewMessage(msgType, data)
}

func NewMessageWithEncoding(msgType MessageType, data interface{}, encoding EncodingType) *Message {
	return message.NewMessageWithEncoding(msgType, data, encoding)
}

func NewRawMessage(msgType MessageType, rawData []byte) *Message {
	return message.NewRawMessage(msgType, rawData)
}

// * Cluster
type ClusterSubscription = cluster.Subscription
type ClusterConfig = cluster.ClusterConfig
type ClusterManager = cluster.Manager
type ClusterStateStore = store.StateStore
type ClusterEvent = cluster.Event
type ClusterEventType = cluster.EventType
type ClusterClientLocation = store.ClientLocation

const (
	ClusterEventSendToClient    = cluster.EventSendToClient
	ClusterEventBroadcast       = cluster.EventBroadcast
	ClusterEventBroadcastToRoom = cluster.EventBroadcastToRoom
)

// This should only be used for testing, examples and simulations
func NewTestMemoryManager() ClusterManager {
	return backends.NewTestMemoryManager()
}

// This should only be used for testing, examples and simulations
func NewTestRedisManager(addr string, topic string, nodeID string) (ClusterManager, error) {
	return backends.NewRedisManager(addr, topic, nodeID)
}

// This should only be used for testing, examples and simulations
func NewTestMemoryStateStore() ClusterStateStore {
	return store.NewMemoryStateStore()
}

// * Dispatcher
type Dispatcher = dispatcher.Dispatcher
type ClientDTO = dispatcher.ClientDTO
type RoomDTO = dispatcher.RoomDTO

// * Rate Limit
type RateLimiterConfig = transport.RateLimiterConfig

// ===== CONSTRUCTORS =====

func NewServer(opts ...websocket.UniversalOption) (*websocket.Server, error) {
	handler, err := websocket.NewHandler(opts...)
	if err != nil {
		return nil, err
	}

	runtime, err := runtime.NewRuntime(runtime.Config{
		Cluster: handler.Config.ClusterManager,
		State:   handler.Config.ClusterState,
		NodeID:  handler.Config.NodeID,
		HubConfig: &hub.HubConfig{
			Logger:             handler.Config.Logger,
			BackpressurePolicy: hub.DropNewest, // TODO: make configurable
		},
	})
	if err != nil {
		return nil, err
	}

	return websocket.NewServer(runtime, handler, opts...)
}

func NewHandler(opts ...websocket.UniversalOption) (*websocket.Handler, error) {
	handler, err := websocket.NewHandler(opts...)
	if err != nil {
		return nil, err
	}

	runtime, err := runtime.NewRuntime(runtime.Config{
		Cluster: handler.Config.ClusterManager,
		State:   handler.Config.ClusterState,
		NodeID:  handler.Config.NodeID,
		HubConfig: &hub.HubConfig{
			Logger:             handler.Config.Logger,
			BackpressurePolicy: hub.DropNewest, // TODO: make configurable
		},
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err = runtime.Start(handler.Config.Serializers, ctx, cancel); err != nil {
		cancel()
		return nil, err
	}

	handler.AttachRuntime(runtime)
	return handler, nil
}

// ===== EVENTS HANDLERS =====

func OnStart(handler OnStartFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			h.Events.OnStart = handler
			return nil
		},
	}
}

func OnBeforeConnect(handler OnBeforeConnectFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnBeforeConnect = handler
			return nil
		},
	}
}

// OnConnect sets a handler for the OnConnect event. The OnConnect event is
// called when a new client connects to the handler. The handler is called with
// the client that connected and a context object as arguments. The context
// object contains information about the client, such as the client's ID and the
// request that the client used to connect to the handler. The handler can
// return an error, which will be returned to the client. If the handler does not
// return an error, the client will be connected to the handler. The OnConnect
// handler can also be used to set the client's name and rooms. The OnConnect
// handler is called before the OnMessage handler is called.
func OnConnect(handler OnConnectFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnConnect = handler
			return nil
		},
	}
}

// OnDisconnect sets a handler for the OnDisconnect event. The OnDisconnect
// event is called when a client disconnects from the handler. The handler is
// called with the client that disconnected and a context object as arguments.
// The context object contains information about the client, such as the client's
// ID and the request that the client used to connect to the handler. The
// handler can return an error, which will be ignored. If the handler does not
// return an error, the client will be disconnected from the handler. The
// OnDisconnect handler is called after the client has been disconnected from
// the handler. The OnDisconnect handler can also be used to clean up resources
// associated with the client.
func OnDisconnect(handler OnDisconnectFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnDisconnect = handler
			return nil
		},
	}
}

// OnMessage sets a handler for the OnMessage event. The OnMessage event is
// called when a client sends a message to the handler. The handler is called
// with the client that sent the message, the message that was sent, and a
// context object as arguments. The context object contains information about
// the client, such as the client's ID and the request that the client used to
// connect to the handler. The handler can return an error, which will be
// returned to the client. If the handler does not return an error, the message
// will be processed as usual. The OnMessage handler is called after the
// OnRawMessage handler is called. The OnMessage handler can also be used to
// modify the message before it is processed by the handler.
func OnMessage(handler OnMessageFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnMessage = handler
			return nil
		},
	}
}

// OnRawMessage sets a handler for the OnRawMessage event. The OnRawMessage event
// is called when a client sends a message to the handler. The handler is called
// with the client that sent the message, the message that was sent, and a
// context object as arguments. The context object contains information about
// the client, such as the client's ID and the request that the client used to
// connect to the handler. The handler can return an error, which will be
// returned to the client. If the handler does not return an error, the message
// will be processed as usual. The OnRawMessage handler is called before the
// OnMessage handler is called. The OnRawMessage handler can also be used to
// modify the message before it is processed by the OnMessage handler.
func OnRawMessage(handler OnRawMessageFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnRawMessage = handler
			return nil
		},
	}
}

// OnJSONMessage sets a handler for the OnJSONMessage event. The OnJSONMessage event
// is called when a client sends a JSON message to the handler. The handler is called
// with the client that sent the message, the unmarshaled JSON data, and a
// context object as arguments. The context object contains information about
// the client, such as the client's ID and the request that the client used to
// connect to the handler. The handler can return an error, which will be
// returned to the client. If the handler does not return an error, the message
// will be processed as usual. The OnJSONMessage handler is called after the
// OnRawMessage handler is called. The OnJSONMessage handler can also be used to
// modify the JSON data before it is processed by the handler.
func OnJSONMessage(handler OnJSONMessageFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnJSONMessage = handler
			return nil
		},
	}
}

// OnError sets a handler for the OnError event. The OnError event is called when
// the handler encounters an error while handling a client. The handler is
// called with the client that caused the error, the error that was encountered,
// and a context object as arguments. The context object contains information
// about the client, such as the client's ID and the request that the client used
// to connect to the handler. The handler can return an error, which will be
// ignored. If the handler does not return an error, the error will be logged.
// The OnError handler is called after the error has been logged. The OnError
// handler can also be used to clean up resources associated with the client.
func OnError(handler OnErrorFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnError = handler
			return nil
		},
	}
}

// OnPing sets a handler for the OnPing event. The OnPing event is called when
// the handler sends a ping message to a client. The handler is called with the
// client that received the ping message and a context object as arguments. The
// context object contains information about the client, such as the client's ID
// and the request that the client used to connect to the handler. The handler
// can return an error, which will be ignored. If the handler does not return an
// error, the ping message will be sent to the client as usual. The OnPing
// handler is called before the ping message is sent to the client. The OnPing
// handler can also be used to modify the ping message before it is sent to the
// client.
func OnPing(handler OnPingFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnPing = handler
			return nil
		},
	}
}

// OnPong sets a handler for the OnPong event. The OnPong event is called when a
// client responds to a ping message sent by the handler. The handler is called
// with the client that responded to the ping message and a context object as
// arguments. The context object contains information about the client, such as
// the client's ID and the request that the client used to connect to the
// handler. The handler can return an error, which will be ignored. If the
// handler does not return an error, the pong message will be processed as
// usual. The OnPong handler is called after the pong message is processed. The
// OnPong handler can also be used to clean up resources associated with the
// client.
func OnPong(handler OnPongFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}
			h.Events.OnPong = handler
			return nil
		},
	}
}
