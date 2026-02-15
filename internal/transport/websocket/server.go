// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package websocket

/**
 * server.go implements a standalone WebSocket server that hosts a GoSocket
 * Runtime.
 *
 * It is responsible for opening network listeners, accepting connections,
 * and adapting them into the Runtime.
 *
 * This file exists only to provide a standalone deployment option.
 *
 * MUST NOT contain cluster logic, business rules, or Runtime internals.
 */

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	gsErrors "github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/runtime"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type ServerConfig struct {
	Port       int
	Path       string
	EnableCORS bool
	EnableSSL  bool
	CertFile   string
	KeyFile    string
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:       8080,
		Path:       "/ws",
		EnableCORS: true,
		EnableSSL:  false,
	}
}

type Server struct {
	Config *ServerConfig

	handler *Handler
	runtime *runtime.Runtime

	httpServer *http.Server

	isRunning atomic.Bool
}

func NewServer(rt *runtime.Runtime, handler *Handler, options ...UniversalOption) (*Server, error) {
	if rt == nil {
		return nil, errors.New("runtime is required")
	}

	if handler == nil {
		return nil, errors.New("handler is required")
	}

	s := &Server{
		handler: handler,
		runtime: rt,
		Config:  DefaultServerConfig(),
	}

	// filter only those that are ServerOption
	return s.With(options...)
}

// ===== Lifecycle =====

func (s *Server) Start() error {
	return s.StartWithContext(context.Background())
}

func (s *Server) StartWithContext(ctx context.Context) error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return gsErrors.ErrServerAlreadyRunning
	}

	ctx, cancel := context.WithCancel(ctx)
	if err := s.runtime.Start(s.handler.Config.Serializers, ctx, cancel); err != nil {
		s.isRunning.Store(false)
		return err
	}

	s.handler.AttachRuntime(s.runtime)

	mux := http.NewServeMux()
	mux.Handle(s.Config.Path, s.handler)

	var httpHandler http.Handler = mux
	httpHandler = s.handler.ApplyMiddlewares(httpHandler)

	s.httpServer = s.buildHTTPServer(httpHandler)

	errChan := make(chan error, 1)

	utils.SafeGoroutine("ws-serve", func() {
		s.handler.log(logger.LogTypeServer, logger.LogLevelInfo, "Starting server on port %d, path %s...", s.Config.Port, s.Config.Path)
		var err error
		if s.Config.EnableSSL {
			s.handler.log(logger.LogTypeServer, logger.LogLevelInfo, "Enabling SSL...")
			s.httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			err = s.httpServer.ListenAndServeTLS(s.Config.CertFile, s.Config.KeyFile)
		} else {
			s.handler.log(logger.LogTypeServer, logger.LogLevelInfo, "SSL disabled")
			err = s.httpServer.ListenAndServe()
		}

		errChan <- err
	})

	select {
	case <-ctx.Done():
		_ = s.StopGracefully(5 * time.Second)
		return ctx.Err()

	case err := <-errChan:
		defer close(errChan)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) Stop() error {
	if !s.isRunning.Load() || s.httpServer == nil {
		return gsErrors.ErrServerNotRunning
	}
	s.isRunning.Store(false)

	if s.runtime != nil {
		_ = s.handler.Stop()
		_ = s.runtime.Stop()
	}

	s.handler.log(logger.LogTypeServer, logger.LogLevelInfo, "Shutting down server...")
	return s.httpServer.Close()
}

func (s *Server) StopGracefully(timeout time.Duration) error {
	if !s.isRunning.Load() || s.httpServer == nil {
		return gsErrors.ErrServerNotRunning
	}
	s.isRunning.Store(false)
	srv := s.httpServer

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if s.runtime != nil {
		_ = s.handler.Stop()
		_ = s.runtime.Stop()
	}

	s.handler.log(logger.LogTypeServer, logger.LogLevelInfo, "Shutting down server...")
	return srv.Shutdown(ctx)
}

// With applies the given options to the server. The options are applied in the
// order they are given, and if an option returns an error, the server will not
// be modified and the error will be returned. If no options are given, this
// function will return the server as is, with no error.
func (s *Server) With(options ...UniversalOption) (*Server, error) {
	for _, o := range options {
		if err := o.applyServer(s); err != nil {
			s.handler.log(logger.LogTypeServer, logger.LogLevelError, "Failed to apply option: %s", err.Error())
			return nil, err
		}
	}

	return s, nil
}

// ===== Accessors =====

func (s *Server) Handler() *Handler { return s.handler }

func (s *Server) Dispatcher() dispatcher.Dispatcher { return s.handler.Dispatcher() } // so server.Dispatcher().Broadcast(...) can be done

// ===== Internal =====

func (s *Server) buildHTTPServer(handler http.Handler) *http.Server {
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.Config.Port),
		Handler:      handler,
		ReadTimeout:  s.handler.Config.ReadTimeout,
		WriteTimeout: s.handler.Config.WriteTimeout,
	}

	if s.Config.EnableSSL {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return server
}

func (o UniversalOptionFunc) applyServer(s *Server) error {
	if o.ApplyServerFn != nil {
		return o.ApplyServerFn(s)
	}
	return nil
}
