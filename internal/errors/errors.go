// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package errors

import (
	"errors"
	"fmt"
)

var (
	// Server/Handler configuration errors
	ErrMaxConnectionsLessThanOne      = errors.New("max connections must be greater than 0")
	ErrMaxConnectionsPerIPLessThanOne = errors.New("max connections per IP must be greater than 0")
	ErrMessageSizeLessThanOne         = errors.New("message size must be greater than 0")
	ErrTimeoutsLessThanOne            = errors.New("read and write timeouts must be greater than 0")
	ErrPingPongLessThanOne            = errors.New("ping and pong wait periods must be greater than 0")
	ErrPongWaitLessThanPing           = errors.New("pong wait must be greater than ping period")
	ErrSSLFilesRequired               = errors.New("certFile and keyFile is required")
	ErrWithOnlyServer                 = errors.New("can only be set on server")
	ErrInvalidPort                    = errors.New("invalid port")
	ErrDepthLessThanOne               = errors.New("depth must be greater than 0")
	ErrMaxKeyLengthLessThanOne        = errors.New("max key length must be greater than 0")
	ErrMaxElementsLessThanOne         = errors.New("max elements must be greater than 0")
	ErrMaxBinarySizeLessThanOne       = errors.New("max binary size must be greater than 0")

	// Client errors
	ErrClientConnNil   = errors.New("client connection is nil")
	ErrNoDataToSend    = errors.New("message has no data to send")
	ErrClientFull      = errors.New("client message channel is full")
	ErrClientNotFound  = errors.New("client not found")
	ErrInvalidClientId = errors.New("invalid client id")
	ErrClientClosed    = errors.New("client is closed")

	// Message errors
	ErrNilMessage = errors.New("message is nil")

	// Server errors
	ErrServerAlreadyRunning = errors.New("server is already running")
	ErrServerNotRunning     = errors.New("server is not running")
	ErrServerNotInitialized = errors.New("server not properly initialized")
	ErrServerStopped        = errors.New("server stopped with error")
	ErrServerShutdown       = errors.New("server shutdown")

	// Handler errors
	ErrHandlerNotAvailable = errors.New("handler is not available")
	ErrSetWriteDeadline    = errors.New("failed to set write deadline")
	ErrSetReadDeadline     = errors.New("failed to set read deadline")
	ErrSendMessage         = errors.New("failed to send message")
	ErrEventFailed         = errors.New("event failed")
	ErrAuthFailure         = errors.New("authentication failed")
	ErrClientIdGenerator   = errors.New("client id generator failed")
	ErrTooManyRequests     = errors.New("too many requests")
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")

	// Hub errors
	ErrHubIsNil                      = errors.New("hub is nil")
	ErrHubNotRunning                 = errors.New("hub is not running")
	ErrHubStopped                    = errors.New("hub stopped")
	ErrUpgradeFailed                 = errors.New("websocket upgrade failed")
	ErrBroadcastFull                 = errors.New("broadcast channel is full")
	ErrBroadcastToRoomSomeClientFull = errors.New("some client channel is full during room broadcast")

	// Dispatcher errors
	ErrDispatcherNotAvailable = errors.New("dispatcher is not available")

	// Cluster errors
	ErrClusterManagerNotProvided = errors.New("cluster manager not provided")
	ErrClusterStateNotConfigured = errors.New("cluster state is not configured")

	// Room errors
	ErrRoomNameEmpty       = errors.New("room name cannot be empty")
	ErrRoomNotFound        = errors.New("room not found")
	ErrRoomAlreadyExists   = errors.New("room already exists")
	ErrClientAlreadyInRoom = errors.New("client already in room")

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

	// Generic errors
	ErrUnexpectedError  = errors.New("unexpected error")
	ErrInvalidArguments = errors.New("invalid arguments")
	ErrSendBufferFull   = errors.New("send buffer is full")
	ErrNilPayload       = errors.New("payload is nil")
)

func NewSerializeError(err error) error {
	return fmt.Errorf("%w: %w", ErrSerializeData, err)
}

func NewRoomNotFoundError(name string) error {
	return fmt.Errorf("%w: %s", ErrRoomNotFound, name)
}

func NewSetWriteDeadlineError(err error) error {
	return fmt.Errorf("%w: %w", ErrSetWriteDeadline, err)
}

func NewSetReadDeadlineError(err error) error {
	return fmt.Errorf("%w: %w", ErrSetReadDeadline, err)
}

func NewUpgradeFailedError(err error) error {
	return fmt.Errorf("%w: %w", ErrUpgradeFailed, err)
}

func NewSendMessageError(err error) error {
	return fmt.Errorf("%w: %w", ErrSendMessage, err)
}

func NewEventFailedError(event string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrEventFailed, event, err)
}

func NewServerStoppedError(err error) error {
	return fmt.Errorf("%w: %w", ErrServerStopped, err)
}

func NewClientNotFoundError(id string) error {
	return fmt.Errorf("%w: %s", ErrClientNotFound, id)
}

func NewWithOnlyServerError(t string) error {
	return fmt.Errorf("%s %w", t, ErrWithOnlyServer)
}

func NewInvalidPortError(port int) error {
	return fmt.Errorf("%w: %d", ErrInvalidPort, port)
}

func NewAuthFailureError(err error) error {
	return fmt.Errorf("%w: %w", ErrAuthFailure, err)
}

func NewClientIdGeneratorError(err error) error {
	return fmt.Errorf("%w: %w", ErrClientIdGenerator, err)
}

func NewMaxDepthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxDepthExceeded, val)
}

func NewMaxKeyLengthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxKeyLengthExceeded, val)
}

func NewMaxElementsExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxElementsExceeded, val)
}

func NewMaxStringLengthExceededError(val int) error {
	return fmt.Errorf("%w: %d", ErrMaxStringLengthExceeded, val)
}

func NewTypeNotAllowedError(t string) error {
	return fmt.Errorf("%w: %s", ErrTypeNotAllowed, t)
}

func NewInvalidValueError(val string) error {
	return fmt.Errorf("%w: %s", ErroInvalidValue, val)
}

func NewInvalidJsonError(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
}

func NewInvalidStructError(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidStruct, err)
}

func NewDataTooLongError(val int) error {
	return fmt.Errorf("%w: %d", ErrDataTooLong, val)
}

func NewMaxConnPerIpReachedError(ip string) error {
	return fmt.Errorf("%w: %s", ErrMaxConnPerIpReached, ip)
}
