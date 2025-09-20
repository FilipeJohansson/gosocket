// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"errors"
	"fmt"
)

var (
	// Server/Handler configuration errors
	ErrMaxConnectionsLessThanOne = errors.New("max connections must be greater than 0")
	ErrMessageSizeLessThanOne    = errors.New("message size must be greater than 0")
	ErrTimeoutsLessThanOne       = errors.New("read and write timeouts must be greater than 0")
	ErrPingPongLessThanOne       = errors.New("ping and pong wait periods must be greater than 0")
	ErrPongWaitLessThanPing      = errors.New("pong wait must be greater than ping period")
	ErrSSLFilesEmpty             = errors.New("certFile or keyFile is empty")
	ErrWithOnlyServer            = errors.New("can only be set on server")
	ErrInvalidPort               = errors.New("invalid port")
	ErrDepthLessThanOne          = errors.New("depth must be greater than 0")
	ErrMaxKeyLengthLessThanOne   = errors.New("max key length must be greater than 0")
	ErrMaxElementsLessThanOne    = errors.New("max elements must be greater than 0")
	ErrMaxBinarySizeLessThanOne  = errors.New("max binary size must be greater than 0")

	// Client errors
	ErrClientConnNil  = errors.New("client connection is nil")
	ErrNoDataToSend   = errors.New("message has no data to send")
	ErrClientFull     = errors.New("client message channel is full")
	ErrClientNotFound = errors.New("client not found")

	// Server errors
	ErrServerAlreadyRunning = errors.New("server is already running")
	ErrServerNotRunning     = errors.New("server is not running")
	ErrServerNotInitialized = errors.New("server not properly initialized")
	ErrServerStopped        = errors.New("server stopped with error")
	ErrServerShutdown       = errors.New("server shutdown")

	// Handler errors
	ErrSetWriteDeadline  = errors.New("failed to set write deadline")
	ErrSetReadDeadline   = errors.New("failed to set read deadline")
	ErrSendMessage       = errors.New("failed to send message")
	ErrEventFailed       = errors.New("event failed")
	ErrAuthFailure       = errors.New("authentication failed")
	ErrTooManyRequests   = errors.New("too many requests")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// Hub errors
	ErrHubIsNil      = errors.New("hub is nil")
	ErrRoomNameEmpty = errors.New("room name cannot be empty")
	ErrRoomNotFound  = errors.New("room not found")
	ErrUpgradeFailed = errors.New("websocket upgrade failed")

	// Serializer errors
	ErrRawSerializer           = errors.New("raw serializer expects []byte")
	ErrRawSerializerPtr        = errors.New("raw serializer expects *[]byte")
	ErrSerializeData           = errors.New("failed to serialize data")
	ErrSerializerNotFound      = errors.New("serializer not found for encoding")
	ErrMaxDepthExceeded        = errors.New("max depth exceeded")
	ErrMaxKeyLengthExceeded    = errors.New("max key length exceeded")
	ErrMaxElementsExceeded     = errors.New("max elements exceeded")
	ErrMaxStringLengthExceeded = errors.New("max string length exceeded")
	ErrTypeNotAllowed          = errors.New("type not allowed")
	ErroInvalidValue           = errors.New("invalid value")
	ErrEmptyData               = errors.New("empty data")
	ErrDataTooLong             = errors.New("data too long")
	ErrInvalidJSON             = errors.New("invalid JSON")
	ErrInvalidStruct           = errors.New("invalid struct")

	// Connection Pool errors
	ErrMaxConnReached      = errors.New("max connections reached")
	ErrMaxConnPerIpReached = errors.New("max connections per IP reached")

	// Encoding errors
	ErrRawEncoding         = errors.New("raw encoding expects []byte data")
	ErrUnsupportedEncoding = errors.New("unsupported encoding")
)

func newUnsupportedEncodingError(encoding EncodingType) error {
	return fmt.Errorf("%w: %d", ErrUnsupportedEncoding, encoding)
}

func newSerializeError(err error) error {
	return fmt.Errorf("%w: %w", ErrSerializeData, err)
}

func newRoomNotFoundError(name string) error {
	return fmt.Errorf("%w: %s", ErrRoomNotFound, name)
}

func newSetWriteDeadlineError(err error) error {
	return fmt.Errorf("%w: %w", ErrSetWriteDeadline, err)
}

func newSetReadDeadlineError(err error) error {
	return fmt.Errorf("%w: %w", ErrSetReadDeadline, err)
}

func newUpgradeFailedError(err error) error {
	return fmt.Errorf("%w: %w", ErrUpgradeFailed, err)
}

func newSendMessageError(err error) error {
	return fmt.Errorf("%w: %w", ErrSendMessage, err)
}

func newEventFailedError(event string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrEventFailed, event, err)
}

func newServerStoppedError(err error) error {
	return fmt.Errorf("%w: %w", ErrServerStopped, err)
}

func newSerializerNotFoundError(encoding EncodingType) error {
	return fmt.Errorf("%w: %d", ErrSerializerNotFound, encoding)
}

func newClientNotFoundError(id string) error {
	return fmt.Errorf("%w: %s", ErrClientNotFound, id)
}

func newWithOnlyServerError(t string, h HasHandler) error {
	return fmt.Errorf("%s %w, got %T", t, ErrWithOnlyServer, h)
}

func newInvalidPortError(port int) error {
	return fmt.Errorf("%w: %d", ErrInvalidPort, port)
}

func newAuthFailureError(err error) error {
	return fmt.Errorf("%w: %w", ErrAuthFailure, err)
}

func newMaxDepthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxDepthExceeded, val)
}

func newMaxKeyLengthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxKeyLengthExceeded, val)
}

func newMaxElementsExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxElementsExceeded, val)
}

func newMaxStringLengthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxStringLengthExceeded, val)
}

func newTypeNotAllowedError(t string) error {
	return fmt.Errorf("%w: %s", ErrTypeNotAllowed, t)
}

func newInvalidValueError(val string) error {
	return fmt.Errorf("%w: %s", ErroInvalidValue, val)
}

func newInvalidJsonError(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
}

func newInvalidStructError(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidStruct, err)
}

func newDataTooLongError(val int) error {
	return fmt.Errorf("%w: %d", ErrDataTooLong, val)
}

func newMaxConnPerIpReachedError(ip string) error {
	return fmt.Errorf("%w: %s", ErrMaxConnPerIpReached, ip)
}
