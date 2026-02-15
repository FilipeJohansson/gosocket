// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/transport"
	"github.com/FilipeJohansson/gosocket/internal/transport/websocket"
)

/**
 * options.go defines all public configuration options (WithX functions)
 * used to customize GoSocket behavior.
 *
 * These options are responsible for collecting user intent and translating
 * it into internal runtime configuration in a decoupled and extensible way.
 *
 * This file should only define option types, default values, and option
 * application logic.
 *
 * MUST NOT create runtime instances, perform validation that depends on
 * runtime state, or execute side effects.
 */

// ===== Global Options (Server & Handler) =====

// WithMaxConnections sets the maximum number of connections allowed for a handler.
// If the limit is exceeded, new connections will be rejected with an error.
// The limit must be greater than 0.
func WithMaxConnections(c ConnectionPoolConfig) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if c.MaxTotal <= 0 {
				return errors.ErrMaxConnectionsLessThanOne
			}
			if c.MaxPerIP <= 0 {
				return errors.ErrMaxConnectionsPerIPLessThanOne
			}

			cfg.ConnectionPoolConfig = c
			return nil
		},
	}
}

// WithMessageSize sets the maximum size of an incoming message in bytes. If the message size is exceeded, the connection will be closed with an error.
// The size must be greater than 0.
func WithMessageSize(size int64) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if size <= 0 {
				return errors.ErrMessageSizeLessThanOne
			}

			cfg.MessageSize = size
			return nil
		},
	}
}

// WithTimeout sets the read and write timeouts for a handler. If the read timeout is
// exceeded, the connection will be closed with an error. If the write timeout is
// exceeded, the write will be cancelled and the connection will be closed with an
// error. The timeouts must be greater than 0.
func WithTimeout(read, write time.Duration) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if read <= 0 || write <= 0 {
				return errors.ErrTimeoutsLessThanOne
			}

			cfg.ReadTimeout = read
			cfg.WriteTimeout = write
			return nil
		},
	}
}

// WithPingPong sets the ping and pong wait periods for a handler. The ping period
// is the interval at which the handler sends a ping message to a client. The pong
// wait is the maximum time allowed for a client to respond to a ping message.
// If the pong wait is exceeded, the connection will be closed with an error.
// The ping and pong wait periods must be greater than 0 and the pong wait must be
// greater than the ping period.
func WithPingPong(pingPeriod, pongWait time.Duration) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if pingPeriod <= 0 || pongWait <= 0 {
				return errors.ErrPingPongLessThanOne
			}

			if pingPeriod > pongWait {
				return errors.ErrPongWaitLessThanPing
			}

			cfg.PingPeriod = pingPeriod
			cfg.PongWait = pongWait
			return nil
		},
	}
}

// WithAllowedOrigins sets the allowed origins for a handler. If the origins are
// specified, the handler will only allow incoming requests from the specified
// origins. If the origins are empty, the handler will allow incoming requests from
// any origin. The origins must be in the format "scheme://host[:port]".
// func WithAllowedOrigins(origins []string) websocket.UniversalOption {
// 	return websocket.UniversalOptionFunc{
// ApplyHandlerFn: func(h *websocket.Handler) error {
// hcfg := t.GetHandler()
// if h == nil {
// nil
// }

// cfg :h.ConfigetConfig()
// 		cfg.AllowedOrigins = origins
// 		return nil
// 	})
// }
// TODO: change to CheckOriginFn

func WithCheckOrigin(f func(r *http.Request) bool) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.CheckOrigin = f
			return nil
		},
	}
}

// WithEncoding sets the default encoding for a handler. The encoding is used to
// serialize outgoing messages and deserialize incoming messages. The default
// encoding is JSON, but you can change it to any of the supported encodings
// (JSON, Raw). If the encoding is not supported, an error will be
// returned.
func WithEncoding(encoding message.EncodingType) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.DefaultEncoding = encoding
			return nil
		},
	}
}

// WithSerializer sets a custom serializer for the specified encoding type.
//
// The serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the specified encoding type. The encoding type must be
// one of the supported encoding types (JSON, Raw). If the encoding type
// is not supported, an error will be returned.
//
// The serializer will be used for all incoming and outgoing messages with the
// specified encoding type. If you want to use a different serializer for a
// specific message, you can use the WithEncoding option on the message.
func WithSerializer(encoding message.EncodingType, serializer message.Serializer) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.Serializers[encoding] = serializer
			return nil
		},
	}
}

// WithJSONSerializer sets the default JSON serializer for a handler. The
// serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the JSON encoding type. The default JSON serializer
// will be used if no other serializer is specified.
func WithJSONSerializer() websocket.UniversalOption {
	return WithSerializer(message.JSON, message.CreateSerializer(message.JSON, DefaultSerializerConfig()))
}

// WithRawSerializer sets the default Raw serializer for a handler. The
// serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the Raw encoding type. The default Raw serializer
// will be used if no other serializer is specified.
func WithRawSerializer() websocket.UniversalOption {
	return WithSerializer(message.Raw, message.CreateSerializer(message.Raw, DefaultSerializerConfig()))
}

// WithMiddleware adds a middleware to the handler. The middleware will be
// applied to the handler in the order it is specified. If no middlewares are
// specified, the handler will not apply any middlewares. The middleware will
// receive the original request and response as arguments, and can return a new
// request and response. If the middleware returns an error, the handler will
// return the error to the client. If the middleware does not return an error, the
// handler will call the next middleware in the chain. If the middleware chain
// returns an error, the handler will return the error to the client. If the
// middleware chain does not return an error, the handler will call the original
// handler with the modified request and response. If the original handler returns
// an error, the handler will return the error to the client. If the original
// handler does not return an error, the handler will return the result of the
// original handler to the client.
func WithMiddleware(middleware websocket.Middleware) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if cfg.Middlewares == nil {
				cfg.Middlewares = make([]websocket.Middleware, 0)
			}

			cfg.Middlewares = append(cfg.Middlewares, middleware)
			return nil
		},
	}
}

// WithAuth sets an authentication function for a handler. The authentication
// function will be called with the original request as an argument when a new
// client connects to the handler. If the authentication function returns an
// error, the client will be immediately disconnected. If the authentication
// function does not return an error, the client will be authenticated and
// connected to the handler. The authentication function can return a value to
// be associated with the client, which can be accessed later in the
// OnConnect, OnDisconnect, OnMessage, OnRawMessage, OnJSONMessage handlers.
// The authentication function can also return an error, which will be returned
// to the client. If the authentication function returns an error, the client
// will not be connected to the handler. If the authentication function does not
// return an error, the client will be connected to the handler. The authentication
// function is called before the OnConnect handler is called.
func WithAuth(authFunc websocket.AuthFunc) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.AuthFunc = authFunc
			return nil
		},
	}
}

// WithCustomClientID sets a custom client ID generator for a handler. The
// generator will be called with the original request and associated user data
// (from authentication, if any) when a new client connects to the handler.
// If the generator returns an error, the connection will be rejected with HTTP 500.
// If the generator returns an empty string, the connection will also be rejected.
// The generator is called after authentication but before the WebSocket upgrade
// and OnConnect handler, allowing the OnConnect handler to access the generated
// client ID through the Client object.
func WithCustomClientID(generator websocket.ClientIdGenerator) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.ClientIdGenerator = generator
			return nil
		},
	}
}

func WithMaxDepth(depth int) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if depth < 1 {
				return errors.ErrDepthLessThanOne
			}

			cfg.Serialization.MaxDepth = depth

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithMaxKeys(keys int) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if keys < 1 {
				return errors.ErrMaxKeyLengthLessThanOne
			}

			cfg.Serialization.MaxKeys = keys

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithMaxElements(elements int) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if elements < 1 {
				return errors.ErrMaxElementsLessThanOne
			}

			cfg.Serialization.MaxElements = elements

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithDisallowedTypes(types []string) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.Serialization.DisallowedTypes = types

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithStrictSerialization(enabled bool) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.Serialization.EnableStrict = enabled

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithMaxBinarySize(size int64) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			if size <= 0 {
				return errors.ErrMaxBinarySizeLessThanOne
			}

			cfg.Serialization.MaxBinarySize = size

			for encoding, serializer := range cfg.Serializers {
				serializer.Configure(cfg.Serialization)
				cfg.Serializers[encoding] = serializer
			}

			return nil
		},
	}
}

func WithRelevantHeaders(headers []string) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.RelevantHeaders = headers
			return nil
		},
	}
}

func WithMessageBufferSize(size int) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.SendChanBufSize = size
			return nil
		},
	}
}

func WithRateLimit(config *transport.RateLimiterConfig) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.RateLimiter = transport.NewRateLimiterManager(config)
			return nil
		},
	}
}

func WithLogger(l logger.Logger, levels map[logger.LogType]logger.LogLevel) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			cfg.Logger = &logger.LoggerConfig{
				Logger: l,
				Level:  levels,
			}
			return nil
		},
	}
}

func WithCluster(clusterCfg cluster.ClusterConfig) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			if clusterCfg.Manager == nil {
				return errors.ErrClusterManagerNotProvided
			}

			cfg := h.Config
			cfg.ClusterManager = clusterCfg.Manager
			cfg.ClusterState = clusterCfg.State
			cfg.NodeID = clusterCfg.NodeID
			return nil
		},
	}
}

// WithDebugLogger configures a logger where all log types are set to LogLevelDebug
func WithDebugLogger() websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyHandlerFn: func(h *websocket.Handler) error {
			if h == nil {
				return nil
			}

			cfg := h.Config
			var levels = map[LogType]LogLevel{
				LogTypeServer:     LogLevelDebug,
				LogTypeClient:     LogLevelDebug,
				LogTypeAuth:       LogLevelDebug,
				LogTypeBroadcast:  LogLevelDebug,
				LogTypeConnection: LogLevelDebug,
				LogTypeMessage:    LogLevelDebug,
				LogTypeError:      LogLevelError,
				LogTypeRateLimit:  LogLevelDebug,
				LogTypeRoom:       LogLevelDebug,
				LogTypeOther:      LogLevelDebug,
			}

			cfg.Logger = &LoggerConfig{
				Logger: &DefaultLogger{},
				Level:  levels,
			}
			return nil
		},
	}
}

// TODO: add hub backpressure config
// TODO: add broadcast chan buffer size config

// ===== Server Options =====

func WithPort(port int) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyServerFn: func(s *websocket.Server) error {
			cfg := s.Config
			if cfg == nil {
				return nil
			}

			if port <= 0 || port > 65535 {
				return errors.NewInvalidPortError(port)
			}

			cfg.Port = port
			return nil
		},
	}
}

func WithPath(path string) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyServerFn: func(s *websocket.Server) error {
			cfg := s.Config
			if cfg == nil {
				return nil
			}

			if path == "" {
				path = "/ws"
			}
			if path[0] != '/' {
				path = "/" + path
			}
			cfg.Path = path
			return nil
		},
	}
}

func WithCORS(enabled bool) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyServerFn: func(s *websocket.Server) error {
			cfg := s.Config
			if cfg == nil {
				return nil
			}

			cfg.EnableCORS = enabled
			return nil
		},
	}
}

func WithSSL(certFile, keyFile string) websocket.UniversalOption {
	return websocket.UniversalOptionFunc{
		ApplyServerFn: func(s *websocket.Server) error {
			cfg := s.Config
			if cfg == nil {
				return nil
			}

			if certFile == "" || keyFile == "" {
				return errors.ErrSSLFilesRequired
			}
			cfg.EnableSSL = true
			cfg.CertFile = certFile
			cfg.KeyFile = keyFile
			return nil
		},
	}
}
