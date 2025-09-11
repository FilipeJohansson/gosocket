package gosocket

import (
	"context"
	"net/http"
	"time"
)

type HandlerContext struct {
	ctx       context.Context
	startTime time.Time
	connInfo  *ConnectionInfo

	handler *Handler
	hub     IHub
}

// NewHandlerContext creates a new context (for inner goroutines)
// It creates a new context with the given handler and hub, and the same
// connection info as the original context. The context is created with a
// new background context, and the start time is set to the current time.
func NewHandlerContext(handler *Handler) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo: &ConnectionInfo{
			RequestID: generateRequestID(),
		},
	}
}

// NewHandlerContextFromRequest creates a new context from the given http request.
// It retrieves the client IP, user agent, origin, and headers from the request
// and uses them to create a new context.
func NewHandlerContextFromRequest(handler *Handler, r *http.Request) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo: &ConnectionInfo{
			ClientIP:  getClientIPFromRequest(r),
			UserAgent: r.Header.Get("User-Agent"),
			Origin:    r.Header.Get("Origin"),
			Headers:   extractHeaders(r),
			RequestID: generateRequestID(),
		},
	}
}

// NewHandlerContextWithConnection creates a new context with the given connection info. This
// method is useful when you want to reuse the same connection info for multiple contexts.
// It returns a new context with the given connection info, and the same handler and hub as the
// original context. The context is created with a new background context, and the start time is
// set to the current time.
func NewHandlerContextWithConnection(handler *Handler, connInfo *ConnectionInfo) *HandlerContext {
	return &HandlerContext{
		handler:   handler,
		hub:       handler.hub,
		ctx:       context.Background(),
		startTime: time.Now(),
		connInfo:  connInfo,
	}
}

// Context returns the context associated with the handler context. The context
// is the parent context that was passed when creating the handler context. The
// context can be used to cancel the context, or to retrieve values from the
// context.
func (hc *HandlerContext) Context() context.Context {
	return hc.ctx
}

// WithContext returns a new context with the given context. The context is the parent context
// that was passed when creating the handler context. The context can be used to cancel the
// context, or to retrieve values from the context. The new context is a shallow copy of the
// original context, with the context replaced.
func (hc *HandlerContext) WithContext(ctx context.Context) *HandlerContext {
	newHC := *hc
	newHC.ctx = ctx
	return &newHC
}

// WithTimeout returns a new context with the given timeout, and a cancel function. The
// returned context is a shallow copy of the original context, with the timeout set to the
// given value. The cancel function can be used to cancel the context, or to retrieve values
// from the context. The context can be used to cancel the context, or to retrieve values
// from the context.
func (hc *HandlerContext) WithTimeout(timeout time.Duration) (*HandlerContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(hc.ctx, timeout)
	return hc.WithContext(ctx), cancel
}

// RequestID returns the request ID associated with the context. The request ID is a unique
// identifier for the connection, and can be used to identify the connection in logs and metrics.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) RequestID() string {
	if hc.connInfo != nil {
		return hc.connInfo.RequestID
	}
	return ""
}

// ClientIP returns the client IP address associated with the context.
// If the context is not associated with a connection, "unknown" is returned.
func (hc *HandlerContext) ClientIP() string {
	if hc.connInfo != nil {
		return hc.connInfo.ClientIP
	}
	return "unknown"
}

// UserAgent returns the user agent associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) UserAgent() string {
	if hc.connInfo != nil {
		return hc.connInfo.UserAgent
	}
	return ""
}

// Origin returns the origin associated with the context.
// The origin is the value of the Origin header of the request that
// established the connection. If the context is not associated with a
// connection, an empty string is returned.
func (hc *HandlerContext) Origin() string {
	if hc.connInfo != nil {
		return hc.connInfo.Origin
	}
	return ""
}

// Header returns the value of the given header key associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *HandlerContext) Header(key string) string {
	if hc.connInfo != nil && hc.connInfo.Headers != nil {
		return hc.connInfo.Headers[key]
	}
	return ""
}

// Headers returns the HTTP headers associated with the context.
// If the context is not associated with a connection, nil is returned.
func (hc *HandlerContext) Headers() map[string]string {
	if hc.connInfo != nil {
		return hc.connInfo.Headers
	}
	return nil
}

// ProcessingDuration returns the time elapsed since the context was created.
// The returned duration is the time elapsed between the context creation
// time and the current time. If the context is not associated with a
// connection, 0 is returned.
func (hc *HandlerContext) ProcessingDuration() time.Duration {
	return time.Since(hc.startTime)
}

// BroadcastToAll broadcasts a message to all connected clients.
//
// This function is a shortcut for calling hub.BroadcastMessage(message).
// If the context is not associated with a hub, an error is returned.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastToAll(message *Message) error {
	if hc.hub == nil {
		return ErrHubIsNil
	}

	hc.hub.BroadcastMessage(message)
	return nil
}

// BroadcastToRoom broadcasts a message to all clients in the specified room.
//
// This function is a shortcut for calling hub.BroadcastToRoom(room, message).
// If the context is not associated with a hub, an error is returned.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastToRoom(room string, message *Message) error {
	if hc.hub == nil {
		return ErrHubIsNil
	}

	message.Room = room
	hc.hub.BroadcastToRoom(room, message)
	return nil
}

// BroadcastJSONToAll broadcasts the given data as JSON to all connected clients.
//
// It wraps the given data in a Message and sets the Encoding to JSON, then
// calls BroadcastToAll to send the message.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastJSONToAll(data interface{}) error {
	message := NewMessage(TextMessage, data)
	message.Encoding = JSON
	return hc.BroadcastToAll(message)
}

// BroadcastJSONToRoom broadcasts the given data as JSON to all clients in the specified room.
//
// It wraps the given data in a Message and sets the Encoding to JSON, then
// calls BroadcastToRoom to send the message.
//
// It returns an error if the hub is not properly set.
func (hc *HandlerContext) BroadcastJSONToRoom(room string, data interface{}) error {
	message := NewMessage(TextMessage, data)
	message.Encoding = JSON
	return hc.BroadcastToRoom(room, message)
}

// GetClientsInRoom returns a list of all clients in the given room. If the
// context is not associated with a hub, an empty slice is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetClientsInRoom(room string) []*Client {
	if hc.hub == nil {
		return []*Client{}
	}
	return hc.hub.GetRoomClients(room)
}

// GetAllClients returns a list of all clients connected to the hub.
//
// If the context is not associated with a hub, an empty slice is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetAllClients() []*Client {
	if hc.hub == nil {
		return []*Client{}
	}

	clients := []*Client{}
	for client := range hc.hub.GetClients() {
		clients = append(clients, client)
	}
	return clients
}

// GetStats returns a map with statistics about the hub.
//
// If the context is not associated with a hub, an empty map is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) GetStats() map[string]interface{} {
	if hc.hub == nil {
		return map[string]interface{}{}
	}
	return hc.hub.GetStats()
}

// Handler returns the handler associated with the context. This can be used to
// access the underlying handler's configuration and functionality.
func (hc *HandlerContext) Handler() *Handler {
	return hc.handler
}

// Hub returns the hub associated with the context. This can be used to access the
// hub's methods and configuration. The returned hub is the same as the one passed
// when creating the HandlerContext. If the context is not associated with a hub,
// nil is returned.
//
// This method is safe to call concurrently.
func (hc *HandlerContext) Hub() IHub {
	return hc.hub
}
