package gosocket

import (
	"time"
)

// ===== Functional Options =====

// WithMaxConnections sets the maximum number of connections allowed for a handler.
// If the limit is exceeded, new connections will be rejected with an error.
// The limit must be greater than 0.
func WithMaxConnections(max int) UniversalOption {
	return func(h HasHandler) error {
		if max <= 0 {
			return ErrMaxConnectionsLessThanOne
		}

		h.Handler().config.MaxConnections = max
		return nil
	}
}

// WithMessageSize sets the maximum size of an incoming message in bytes. If the message size is exceeded, the connection will be closed with an error.
// The size must be greater than 0.
func WithMessageSize(size int64) UniversalOption {
	return func(h HasHandler) error {
		if size <= 0 {
			return ErrMessageSizeLessThanOne
		}

		h.Handler().config.MessageSize = size
		return nil
	}
}

// WithTimeout sets the read and write timeouts for a handler. If the read timeout is
// exceeded, the connection will be closed with an error. If the write timeout is
// exceeded, the write will be cancelled and the connection will be closed with an
// error. The timeouts must be greater than 0.
func WithTimeout(read, write time.Duration) UniversalOption {
	return func(h HasHandler) error {
		if read <= 0 || write <= 0 {
			return ErrTimeoutsLessThanOne
		}

		h.Handler().config.ReadTimeout = read
		h.Handler().config.WriteTimeout = write
		return nil
	}
}

// WithPingPong sets the ping and pong wait periods for a handler. The ping period
// is the interval at which the handler sends a ping message to a client. The pong
// wait is the maximum time allowed for a client to respond to a ping message.
// If the pong wait is exceeded, the connection will be closed with an error.
// The ping and pong wait periods must be greater than 0 and the pong wait must be
// greater than the ping period.
func WithPingPong(pingPeriod, pongWait time.Duration) UniversalOption {
	return func(h HasHandler) error {
		if pingPeriod <= 0 || pongWait <= 0 {
			return ErrPingPongLessThanOne
		}

		if pingPeriod > pongWait {
			return ErrPongWaitLessThanPing
		}

		h.Handler().config.PingPeriod = pingPeriod
		h.Handler().config.PongWait = pongWait
		return nil
	}
}

// WithAllowedOrigins sets the allowed origins for a handler. If the origins are
// specified, the handler will only allow incoming requests from the specified
// origins. If the origins are empty, the handler will allow incoming requests from
// any origin. The origins must be in the format "scheme://host[:port]".
func WithAllowedOrigins(origins []string) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().config.AllowedOrigins = origins
		return nil
	}
}

// WithEncoding sets the default encoding for a handler. The encoding is used to
// serialize outgoing messages and deserialize incoming messages. The default
// encoding is JSON, but you can change it to any of the supported encodings
// (JSON, Protobuf, Raw). If the encoding is not supported, an error will be
// returned.
func WithEncoding(encoding EncodingType) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().config.DefaultEncoding = encoding
		return nil
	}
}

// WithSerializer sets a custom serializer for the specified encoding type.
//
// The serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the specified encoding type. The encoding type must be
// one of the supported encoding types (JSON, Protobuf, Raw). If the encoding type
// is not supported, an error will be returned.
//
// The serializer will be used for all incoming and outgoing messages with the
// specified encoding type. If you want to use a different serializer for a
// specific message, you can use the WithEncoding option on the message.
func WithSerializer(encoding EncodingType, serializer Serializer) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().serializers[encoding] = serializer
		return nil
	}
}

// WithJSONSerializer sets the default JSON serializer for a handler. The
// serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the JSON encoding type. The default JSON serializer
// will be used if no other serializer is specified.
func WithJSONSerializer() UniversalOption {
	return WithSerializer(JSON, CreateSerializer(JSON, DefaultSerializerConfig()))
}

// WithProtobufSerializer sets the default Protobuf serializer for a handler. The
// serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the Protobuf encoding type. The default Protobuf
// serializer will be used if no other serializer is specified.
func WithProtobufSerializer() UniversalOption {
	return WithSerializer(Protobuf, CreateSerializer(Protobuf, DefaultSerializerConfig()))
}

// WithRawSerializer sets the default Raw serializer for a handler. The
// serializer will be used to serialize outgoing messages and deserialize
// incoming messages for the Raw encoding type. The default Raw serializer
// will be used if no other serializer is specified.
func WithRawSerializer() UniversalOption {
	return WithSerializer(Raw, CreateSerializer(Raw, DefaultSerializerConfig()))
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
func WithMiddleware(middleware Middleware) UniversalOption {
	return func(h HasHandler) error {
		if h.Handler().middlewares == nil {
			h.Handler().middlewares = make([]Middleware, 0)
		}

		h.Handler().middlewares = append(h.Handler().middlewares, middleware)
		return nil
	}
}

// WithAuth sets an authentication function for a handler. The authentication
// function will be called with the original request as an argument when a new
// client connects to the handler. If the authentication function returns an
// error, the client will be immediately disconnected. If the authentication
// function does not return an error, the client will be authenticated and
// connected to the handler. The authentication function can return a value to
// be associated with the client, which can be accessed later in the
// OnConnect, OnDisconnect, OnMessage, OnRawMessage, OnJSONMessage, and
// OnProtobufMessage handlers. The authentication function can also return an
// error, which will be returned to the client. If the authentication function
// returns an error, the client will not be connected to the handler. If the
// authentication function does not return an error, the client will be
// connected to the handler. The authentication function is called before the
// OnConnect handler is called.
func WithAuth(authFunc AuthFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().authFunc = authFunc
		return nil
	}
}

func WithMaxDepth(depth int) UniversalOption {
	return func(h HasHandler) error {
		if depth < 1 {
			return ErrDepthLessThanOne
		}

		handler := h.Handler()
		handler.config.Serialization.MaxDepth = depth

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithMaxKeys(keys int) UniversalOption {
	return func(h HasHandler) error {
		if keys < 1 {
			return ErrMaxKeyLengthLessThanOne
		}

		handler := h.Handler()
		handler.config.Serialization.MaxKeys = keys

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithMaxElements(elements int) UniversalOption {
	return func(h HasHandler) error {
		if elements < 1 {
			return ErrMaxElementsLessThanOne
		}

		handler := h.Handler()
		handler.config.Serialization.MaxElements = elements

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithDisallowedTypes(types []string) UniversalOption {
	return func(h HasHandler) error {
		handler := h.Handler()
		handler.config.Serialization.DisallowedTypes = types

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithStrictSerialization(enabled bool) UniversalOption {
	return func(h HasHandler) error {
		handler := h.Handler()
		handler.config.Serialization.EnableStrict = enabled

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithMaxBinarySize(size int64) UniversalOption {
	return func(h HasHandler) error {
		if size <= 0 {
			return ErrMaxBinarySizeLessThanOne
		}

		handler := h.Handler()
		handler.config.Serialization.MaxBinarySize = size

		for encoding, serializer := range handler.serializers {
			serializer.Configure(handler.config.Serialization)
			handler.serializers[encoding] = serializer
		}

		return nil
	}
}

func WithRelevantHeaders(headers []string) UniversalOption {
	return func(h HasHandler) error {
		handler := h.Handler()
		handler.config.RelevantHeaders = headers
		return nil
	}
}

func WithMessageBufferSize(size int) UniversalOption {
	return func(h HasHandler) error {
		handler := h.Handler()
		handler.config.MessageChanBufSize = size
		return nil
	}
}

func WithRateLimit(config RateLimiterConfig) UniversalOption {
	return func(h HasHandler) error {
		handler := h.Handler()
		handler.rateLimiter = NewRateLimiterManager(config)
		return nil
	}
}

// ===== EVENTS HANDLERS =====

// OnConnect sets a handler for the OnConnect event. The OnConnect event is
// called when a new client connects to the handler. The handler is called with
// the client that connected and a context object as arguments. The context
// object contains information about the client, such as the client's ID and the
// request that the client used to connect to the handler. The handler can
// return an error, which will be returned to the client. If the handler does not
// return an error, the client will be connected to the handler. The OnConnect
// handler can also be used to set the client's name and rooms. The OnConnect
// handler is called before the OnMessage handler is called.
func OnConnect(handler OnConnectFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnConnect = handler
		return nil
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
func OnDisconnect(handler OnDisconnectFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnDisconnect = handler
		return nil
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
func OnMessage(handler OnMessageFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnMessage = handler
		return nil
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
func OnRawMessage(handler OnRawMessageFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnRawMessage = handler
		return nil
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
func OnJSONMessage(handler OnJSONMessageFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnJSONMessage = handler
		return nil
	}
}

// OnProtobufMessage sets a handler for the OnProtobufMessage event. The
// OnProtobufMessage event is called when a client sends a Protobuf message to
// the handler. The handler is called with the client that sent the message, the
// unmarshaled Protobuf data, and a context object as arguments. The context
// object contains information about the client, such as the client's ID and the
// request that the client used to connect to the handler. The handler can return
// an error, which will be returned to the client. If the handler does not return
// an error, the message will be processed as usual. The OnProtobufMessage
// handler is called after the OnRawMessage handler is called. The
// OnProtobufMessage handler can also be used to modify the Protobuf data before
// it is processed by the handler.
func OnProtobufMessage(handler OnProtobufMessageFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnProtobufMessage = handler
		return nil
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
func OnError(handler OnErrorFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnError = handler
		return nil
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
func OnPing(handler OnPingFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnPing = handler
		return nil
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
func OnPong(handler OnPongFunc) UniversalOption {
	return func(h HasHandler) error {
		h.Handler().events.OnPong = handler
		return nil
	}
}
