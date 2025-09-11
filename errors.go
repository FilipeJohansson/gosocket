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

	// Handler errors
	ErrSetWriteDeadline = errors.New("failed to set write deadline")
	ErrSetReadDeadline  = errors.New("failed to set read deadline")
	ErrSendMessage      = errors.New("failed to send message")
	ErrEventFailed      = errors.New("event failed")
	ErrAuthFailure      = errors.New("authentication failed")

	// Hub errors
	ErrHubIsNil      = errors.New("hub is nil")
	ErrRoomNameEmpty = errors.New("room name cannot be empty")
	ErrRoomNotFound  = errors.New("room not found")
	ErrUpgradeFailed = errors.New("websocket upgrade failed")

	// Serializer errors
	ErrRawSerializer      = errors.New("raw serializer expects []byte")
	ErrRawSerializerPtr   = errors.New("raw serializer expects *[]byte")
	ErrSerializeData      = errors.New("failed to serialize data")
	ErrSerializerNotFound = errors.New("serializer not found for encoding")

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
