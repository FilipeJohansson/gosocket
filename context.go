// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"net/http"
	"time"
)

type Context struct {
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	connInfo  *ConnectionInfo

	handler *Handler
	hub     IHub
}

// NewHandlerContext creates a new context (for inner goroutines)
// It creates a new context with the given handler and hub, and the same
// connection info as the original context. The context is created with a
// new background context, and the start time is set to the current time.
func NewHandlerContext(handler *Handler) *Context {
	ctx, cancel := context.WithCancel(context.Background())
	return &Context{
		handler:   handler,
		hub:       handler.hub,
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
		connInfo: &ConnectionInfo{
			RequestID: generateRequestID(),
		},
	}
}

// NewHandlerContextFromRequest creates a new context from the given http request.
// It retrieves the client IP, user agent, origin, and headers from the request
// and uses them to create a new context.
func NewHandlerContextFromRequest(handler *Handler, r *http.Request) *Context {
	ctx, cancel := context.WithCancel(r.Context())
	return &Context{
		handler:   handler,
		hub:       handler.hub,
		ctx:       ctx,
		cancel:    cancel,
		startTime: time.Now(),
		connInfo: &ConnectionInfo{
			ClientIP:  getClientIPFromRequest(r),
			UserAgent: r.Header.Get("User-Agent"),
			Origin:    r.Header.Get("Origin"),
			Headers:   extractHeaders(r, handler.config.RelevantHeaders...),
			RequestID: generateRequestID(),
		},
	}
}

// Context returns the context associated with the handler context. The context
// is the parent context that was passed when creating the handler context. The
// context can be used to cancel the context, or to retrieve values from the
// context.
func (hc *Context) Context() context.Context {
	return hc.ctx
}

// Cancel cancels the context associated with the handler context. If the
// context is cancelled, any blocked calls to the context's Done method will
// return immediately. If the context is already cancelled, this method does
// nothing. The context is cancelled regardless of whether the handler context
// is associated with a connection or not.
func (hc *Context) Cancel() {
	if hc.cancel != nil {
		hc.cancel()
	}
}

// WithContext returns a new context with the given context. The context is the parent context
// that was passed when creating the handler context. The context can be used to cancel the
// context, or to retrieve values from the context. The new context is a shallow copy of the
// original context, with the context replaced.
func (hc *Context) WithContext(ctx context.Context) *Context {
	newHC := *hc
	newHC.ctx = ctx
	return &newHC
}

// WithTimeout returns a new context with the given timeout, and a cancel function. The
// returned context is a shallow copy of the original context, with the timeout set to the
// given value. The cancel function can be used to cancel the context, or to retrieve values
// from the context. The context can be used to cancel the context, or to retrieve values
// from the context.
func (hc *Context) WithTimeout(timeout time.Duration) (*Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(hc.ctx, timeout)
	return hc.WithContext(ctx), cancel
}

// RequestID returns the request ID associated with the context. The request ID is a unique
// identifier for the connection, and can be used to identify the connection in logs and metrics.
// If the context is not associated with a connection, an empty string is returned.
func (hc *Context) RequestID() string {
	if hc.connInfo != nil {
		return hc.connInfo.RequestID
	}
	return ""
}

// ClientIP returns the client IP address associated with the context.
// If the context is not associated with a connection, "unknown" is returned.
func (hc *Context) ClientIP() string {
	if hc.connInfo != nil {
		return hc.connInfo.ClientIP
	}
	return "unknown"
}

// UserAgent returns the user agent associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *Context) UserAgent() string {
	if hc.connInfo != nil {
		return hc.connInfo.UserAgent
	}
	return ""
}

// Origin returns the origin associated with the context.
// The origin is the value of the Origin header of the request that
// established the connection. If the context is not associated with a
// connection, an empty string is returned.
func (hc *Context) Origin() string {
	if hc.connInfo != nil {
		return hc.connInfo.Origin
	}
	return ""
}

// Header returns the value of the given header key associated with the context.
// If the context is not associated with a connection, an empty string is returned.
func (hc *Context) Header(key string) string {
	if hc.connInfo != nil && hc.connInfo.Headers != nil {
		return hc.connInfo.Headers[key]
	}
	return ""
}

// Headers returns the HTTP headers associated with the context.
// If the context is not associated with a connection, nil is returned.
func (hc *Context) Headers() map[string]string {
	if hc.connInfo != nil {
		return hc.connInfo.Headers
	}
	return nil
}

// ProcessingDuration returns the time elapsed since the context was created.
// The returned duration is the time elapsed between the context creation
// time and the current time. If the context is not associated with a
// connection, 0 is returned.
func (hc *Context) ProcessingDuration() time.Duration {
	return time.Since(hc.startTime)
}

// Handler returns the handler associated with the context. This can be used to
// access the underlying handler's configuration and functionality.
func (hc *Context) Handler() *Handler {
	return hc.handler
}

// Hub returns the hub associated with the context. This can be used to access the
// hub's methods and configuration. The returned hub is the same as the one passed
// when creating the HandlerContext. If the context is not associated with a hub,
// nil is returned.
//
// This method is safe to call concurrently.
func (hc *Context) Hub() IHub {
	return hc.hub
}
