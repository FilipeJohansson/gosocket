// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package websocket

import (
	"context"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/ids"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type HandlerOption interface {
	applyHandler(*Handler) error
}

type ServerOption interface {
	applyServer(*Server) error
}

type UniversalOption interface {
	HandlerOption
	ServerOption
}

type UniversalOptionFunc struct {
	ApplyHandlerFn func(*Handler) error
	ApplyServerFn  func(*Server) error
}

type Context struct {
	client  *hub.Client
	handler *Handler

	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	connInfo  *hub.ConnectionInfo
}

// NewInternalContext creates a new context (for inner goroutines)
// It creates a new context with the given handler and hub, and the same
// connection info as the original context. The context is created with a
// new background context, and the start time is set to the current time.
// Intended only for internal goroutines.
func NewInternalContext(client *hub.Client, handler *Handler) *Context {
	ctx, cancel := context.WithCancel(context.Background())
	return &Context{
		client:    client,
		handler:   handler,
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
		connInfo: &hub.ConnectionInfo{
			RequestID: ids.GenerateRequestID(),
		},
	}
}

// NewHandlerContextFromRequest creates a new context from the given http request.
// It retrieves the client IP, user agent, origin, and headers from the request
// and uses them to create a new context.
func NewHandlerContextFromRequest(client *hub.Client, handler *Handler, r *http.Request) *Context {
	ctx, cancel := context.WithCancel(r.Context())
	return &Context{
		client:    client,
		handler:   handler,
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
		connInfo: &hub.ConnectionInfo{
			ClientIP:  utils.GetIPFromRequest(r),
			UserAgent: r.Header.Get("User-Agent"),
			Origin:    r.Header.Get("Origin"),
			Headers:   utils.ExtractHeaders(r, handler.Config.RelevantHeaders...),
			RequestID: ids.GenerateRequestID(),
		},
	}
}

// ===== Client =====

// Client returns the client associated with the context. This can be used to access the
// client's methods and configuration.
// Returns true as the second return value if the client is not nil.
// If the context is not associated with a client, nil is returned.
func (c *Context) Client() (*hub.Client, bool) {
	if c.client == nil {
		return nil, false
	}
	return c.client, true
}

// func (c *Context) GetClientRooms() []string {
// 	if c.client == nil {
// 		return nil
// 	}
// 	return c.handler.room
// }

// ===== Room =====

func (c *Context) CreateRoom(clientID, roomName string) (*hub.Room, error) {
	if c.handler == nil {
		return nil, errors.ErrHandlerNotAvailable
	}
	return c.handler.createRoom(clientID, roomName)
}

func (c *Context) JoinRoom(clientID, roomName string) error {
	if c.handler == nil {
		return errors.ErrHandlerNotAvailable
	}
	return c.handler.joinRoom(clientID, roomName)
}

func (c *Context) LeaveRoom(clientID, roomName string) error {
	if c.handler == nil {
		return errors.ErrHandlerNotAvailable
	}
	return c.handler.leaveRoom(clientID, roomName)
}

func (c *Context) GetClientsInRoom(roomName string) map[string]dispatcher.ClientDTO {
	if c.handler == nil || c.handler.dispatcher == nil {
		return map[string]dispatcher.ClientDTO{}
	}
	return c.handler.dispatcher.GetClientsInRoom(roomName)
}

func (c *Context) GetRooms() map[string]dispatcher.RoomDTO {
	if c.handler == nil || c.handler.dispatcher == nil {
		return map[string]dispatcher.RoomDTO{}
	}
	return c.handler.dispatcher.GetRooms()
}

func (c *Context) GetClients() map[string]dispatcher.ClientDTO {
	if c.handler == nil || c.handler.dispatcher == nil {
		return map[string]dispatcher.ClientDTO{}
	}
	return c.handler.dispatcher.GetClients()
}

func (c *Context) GetClientsGlobal() ([]store.ClientLocation, error) {
	if c.handler == nil || c.handler.dispatcher == nil {
		return nil, errors.ErrDispatcherNotAvailable
	}
	return c.handler.dispatcher.GetClientsGlobal(c.ctx)
}

func (c *Context) GetRoomsGlobal() ([]store.RoomInfo, error) {
	if c.handler == nil || c.handler.dispatcher == nil {
		return nil, errors.ErrDispatcherNotAvailable
	}
	return c.handler.dispatcher.GetRoomsGlobal(c.ctx)
}

func (c *Context) GetClientsInRoomGlobal(roomName string) ([]store.ClientPresence, error) {
	if c.handler == nil || c.handler.dispatcher == nil {
		return nil, errors.ErrDispatcherNotAvailable
	}
	return c.handler.dispatcher.GetClientsInRoomGlobal(c.ctx, roomName)
}

// Context returns the context associated with the handler context. The context
// is the parent context that was passed when creating the handler context. The
// context can be used to cancel the context, or to retrieve values from the
// context.
func (c *Context) Context() context.Context {
	return c.ctx
}

// WithContext returns a new context with the given context. The context is the parent context
// that was passed when creating the handler context. The context can be used to cancel the
// context, or to retrieve values from the context. The new context is a shallow copy of the
// original context, with the context replaced.
func (c *Context) WithContext(ctx context.Context) *Context {
	newContext := *c
	newContext.ctx = ctx
	return &newContext
}

// WithTimeout returns a new context with the given timeout, and a cancel function. The
// returned context is a shallow copy of the original context, with the timeout set to the
// given value. The cancel function can be used to cancel the context, or to retrieve values
// from the context. The context can be used to cancel the context, or to retrieve values
// from the context.
func (c *Context) WithTimeout(timeout time.Duration) (*Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	return c.WithContext(ctx), cancel
}

// RequestID returns the request ID associated with the context. The request ID is a unique
// identifier for the connection, and can be used to identify the connection in logs and metrics.
// If the context is not associated with a connection, an empty string is returned.
func (c *Context) RequestID() string {
	if c.connInfo != nil {
		return c.connInfo.RequestID
	}
	return ""
}

// ClientIP returns the client IP address associated with the context.
// If the context is not associated with a connection, "unknown" is returned.
func (c *Context) ClientIP() string {
	if c.connInfo != nil {
		return c.connInfo.ClientIP
	}
	return "unknown"
}

// UserAgent returns the user agent associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (c *Context) UserAgent() string {
	if c.connInfo != nil {
		return c.connInfo.UserAgent
	}
	return ""
}

// Origin returns the origin associated with the context.
// The origin is the value of the Origin header of the request that
// established the connection. If the context is not associated with a
// connection, an empty string is returned.
func (c *Context) Origin() string {
	if c.connInfo != nil {
		return c.connInfo.Origin
	}
	return ""
}

// Header returns the value of the given header key associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (c *Context) Header(key string) string {
	if c.connInfo != nil && c.connInfo.Headers != nil {
		return c.connInfo.Headers[key]
	}
	return ""
}

// Headers returns the HTTP headers associated with the context.
// If the context is not associated with a connection, nil is returned.
func (c *Context) Headers() map[string]string {
	if c.connInfo != nil {
		return c.connInfo.Headers
	}
	return nil
}

// ProcessingDuration returns the time elapsed since the context was created.
// The returned duration is the time elapsed between the context creation
// time and the current time. If the context is not associated with a
// connection, 0 is returned.
func (c *Context) ProcessingDuration() time.Duration {
	return time.Since(c.startTime)
}

// Handler returns the handler associated with the context. This can be used to
// access the underlying handler's configuration and functionality.
func (c *Context) Handler() *Handler {
	return c.handler
}
