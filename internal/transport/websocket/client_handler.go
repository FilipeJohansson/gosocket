// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package websocket

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/utils"
	"github.com/gorilla/websocket"
)

type ClientCallbacks struct {
	OnMessage func(messageType message.MessageType, rawBytes []byte)

	OnPing  func()
	OnError func(err error)
	OnClose func(err error)

	OnLog func(logType logger.LogType, level logger.LogLevel, msg string, args ...interface{})
}

type ClientHandler struct {
	conn      *websocket.Conn
	client    *hub.Client
	handler   *Handler
	callbacks ClientCallbacks

	readTimeout  time.Duration
	writeTimeout time.Duration
	pingPeriod   time.Duration
	pongWait     time.Duration
}

type ClientHandlerConfig struct {
	Client *hub.Client
	Conn   *websocket.Conn

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PingPeriod   time.Duration
	PongWait     time.Duration

	Callbacks ClientCallbacks
}

func NewClientHandler(handler *Handler, cfg *ClientHandlerConfig) *ClientHandler {
	if cfg == nil {
		panic("websocket.ClientHandler: nil config")
	}
	if cfg.Client == nil {
		panic("websocket.ClientHandler: Client is required")
	}
	if cfg.Conn == nil {
		panic("websocket.ClientHandler: Conn is required")
	}

	return &ClientHandler{
		handler:      handler,
		client:       cfg.Client,
		conn:         cfg.Conn,
		callbacks:    cfg.Callbacks,
		readTimeout:  cfg.ReadTimeout,
		writeTimeout: cfg.WriteTimeout,
		pingPeriod:   cfg.PingPeriod,
		pongWait:     cfg.PongWait,
	}
}

func (c *ClientHandler) Start(ctx *Context) {
	utils.SafeGoroutine("ws-read-loop", func() { c.readLoop(ctx) })
	utils.SafeGoroutine("ws-write-loop", func() { c.writeLoop(ctx) })
}

func (c *ClientHandler) readLoop(handlerCtx *Context) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("PANIC in ws readLoop (client %s): %v", c.client.GetID(), r)
			c.log(logger.LogTypeError, logger.LogLevelError, "%s\nStack trace:\n%s\n", err.Error(), string(debug.Stack()))
			c.fireError(err)
		}
		c.fireClose(nil)
	}()

	ctx := handlerCtx.Context()

	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	})

	var violations int
	maxViolations := 0
	if c.handler.Config.RateLimiter != nil {
		maxViolations = c.handler.Config.RateLimiter.MaxRateLimitViolations()
	}

	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.readTimeout)); err != nil {
			c.log(logger.LogTypeError, logger.LogLevelError, "failed to set read deadline: %s", err.Error())
			c.fireError(err)
			return
		}

		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			c.log(logger.LogTypeError, logger.LogLevelError, "failed to read message: %s", err.Error())
			c.fireError(err)
			return
		}

		select {
		case <-ctx.Done():
			c.log(logger.LogTypeMessage, logger.LogLevelInfo, "read loop stopped (client %s)", c.client.GetID())
			return
		default:
		}

		// rate limit (client + ip)
		if rl := c.handler.Config.RateLimiter; rl != nil {
			c.log(logger.LogTypeRateLimit, logger.LogLevelDebug, "checking rate limit (client %s, ip %s)", c.client.GetID(), utils.ExtractIP(c.conn.RemoteAddr()))
			remote := utils.ExtractIP(c.conn.RemoteAddr())
			if !rl.AllowClient(c.client.GetID()) || !rl.AllowIP(remote) {
				violations++
				c.log(logger.LogTypeRateLimit, logger.LogLevelInfo,
					"rate limit violation (client %s, ip %s) (%d/%d)",
					c.client.GetID(), remote, violations, maxViolations,
				)

				if violations >= maxViolations {
					c.log(logger.LogTypeRateLimit, logger.LogLevelError,
						"rate limit exceeded (client %s, ip %s) (%d/%d) - closing connection",
						c.client.GetID(), remote, violations, maxViolations,
					)
					_ = c.writeClose(
						websocket.CloseTryAgainLater,
						errors.ErrRateLimitExceeded.Error(),
					)
					c.fireError(errors.ErrRateLimitExceeded)
					return
				}

				continue
			}
		}

		c.log(logger.LogTypeMessage, logger.LogLevelDebug, "received message (client %s): %s", c.client.GetID(), string(data))
		c.handler.stats.IncrementMessagesReceived(uint64(len(data)))

		switch msgType {
		case websocket.TextMessage:
			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "received text message (client %s): %s", c.client.GetID(), string(data))
			if c.callbacks.OnMessage != nil {
				c.callbacks.OnMessage(message.TextMessage, data)
			}
		case websocket.BinaryMessage:
			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "received binary message (client %s): %s", c.client.GetID(), string(data))
			if c.callbacks.OnMessage != nil {
				c.callbacks.OnMessage(message.BinaryMessage, data)
			}
		case websocket.PingMessage:
			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "received ping message (client %s)", c.client.GetID())
		case websocket.CloseMessage:
			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "received close message (client %s)", c.client.GetID())
			return
		}
	}
}

func (c *ClientHandler) writeLoop(handlerCtx *Context) {
	ticker := time.NewTicker(c.pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("PANIC in ws writeLoop (client %s): %v\n%s", c.client.GetID(), r, debug.Stack())
			c.log(logger.LogTypeError, logger.LogLevelError, "%s\nStack trace:\n%s\n", err.Error(), string(debug.Stack()))
			c.fireError(err)
		}
	}()

	ctx := handlerCtx.Context()

	for {
		select {
		case <-ctx.Done():
			c.log(logger.LogTypeMessage, logger.LogLevelInfo, "write loop stopped (client %s)", c.client.GetID())
			_ = c.writeClose(websocket.CloseGoingAway, "server shutdown")
			return

		case msg, ok := <-c.client.SendChan:
			if !ok {
				c.log(logger.LogTypeMessage, logger.LogLevelDebug, "send channel closed (client %s)", c.client.GetID())
				_ = c.writeClose(websocket.CloseNormalClosure, "")
				return
			}

			if msg.RawData == nil && msg.Data != nil {
				serialized, err := c.serializeMessageWithEncoding(msg)
				if err != nil {
					c.log(logger.LogTypeMessage, logger.LogLevelError, "failed to serialize message: %s", err.Error())
					c.fireError(errors.NewSerializeError(err))
					continue
				}

				msg.RawData = serialized
			}

			if msg.RawData == nil {
				c.log(logger.LogTypeMessage, logger.LogLevelError, "no data to send (client %s)", c.client.GetID())
				return
			}

			if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
				c.log(logger.LogTypeMessage, logger.LogLevelError, "failed to set write deadline: %s", err.Error())
				c.fireError(errors.NewSetWriteDeadlineError(err))
				return
			}

			if err := c.conn.WriteMessage(int(msg.Type), msg.RawData); err != nil {
				c.log(logger.LogTypeMessage, logger.LogLevelError, "failed to write message (type %d | data %s): %s", msg.Type, string(msg.RawData), err.Error())
				c.fireError(err)
				return
			}

			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "sent message (client %s): %s", c.client.GetID(), string(msg.RawData))
			c.handler.stats.IncrementMessagesSent(uint64(len(msg.RawData)))

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
				c.log(logger.LogTypeMessage, logger.LogLevelError, "failed to set write deadline: %s", err.Error())
				c.fireError(errors.NewSetWriteDeadlineError(err))
				return
			}
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.writeTimeout)); err != nil {
				c.fireError(errors.NewSendMessageError(err))
				return
			}

			c.log(logger.LogTypeMessage, logger.LogLevelDebug, "sent ping message (client %s)", c.client.GetID())
			if c.callbacks.OnPing != nil {
				c.callbacks.OnPing()
			}
		}
	}
}

func (c *ClientHandler) serializeMessageWithEncoding(msg *message.Message) ([]byte, error) {
	switch msg.Encoding {
	case message.Protobuf:
		if serializer := c.handler.Config.Serializers[message.Protobuf]; serializer != nil {
			if protoData, err := serializer.Marshal(msg.Data); err == nil {
				return protoData, nil
			} else {
				return nil, errors.NewSerializeError(err)
			}
		}
	case message.Raw:
		if rawData, ok := msg.Data.([]byte); ok {
			return rawData, nil
		} else {
			return nil, errors.ErrRawEncoding
		}
	case message.JSON:
		if serializer := c.handler.Config.Serializers[message.JSON]; serializer != nil {
			return serializer.Marshal(msg.Data)
		}
		fallthrough
	default:
		// fallback to JSON
		if jsonData, err := json.Marshal(msg.Data); err == nil {
			return jsonData, nil
		} else {
			return nil, errors.NewSerializeError(err)
		}
	}

	return nil, errors.ErrSerializeData
}

func (c *ClientHandler) fireError(err error) {
	if err != nil && c.callbacks.OnError != nil {
		c.callbacks.OnError(err)
	}
}

func (c *ClientHandler) fireClose(err error) {
	if c.callbacks.OnClose != nil {
		c.callbacks.OnClose(err)
	}
}

func (c *ClientHandler) writeClose(code int, reason string) error {
	return c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		time.Now().Add(c.writeTimeout),
	)
}

func (c *ClientHandler) log(t logger.LogType, l logger.LogLevel, msg string, args ...interface{}) {
	if c.callbacks.OnLog != nil {
		c.callbacks.OnLog(t, l, msg, args...)
	}
}
