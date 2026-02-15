package errors

import (
	errorspkg "errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorConstructors(t *testing.T) {
	root := errorspkg.New("root")

	tests := []struct {
		name    string
		err     error
		target  error
		contain string
	}{
		{"serialize", NewSerializeError(root), ErrSerializeData, "root"},
		{"room_not_found", NewRoomNotFoundError("roomA"), ErrRoomNotFound, "roomA"},
		{"set_write", NewSetWriteDeadlineError(root), ErrSetWriteDeadline, "root"},
		{"set_read", NewSetReadDeadlineError(root), ErrSetReadDeadline, "root"},
		{"upgrade", NewUpgradeFailedError(root), ErrUpgradeFailed, "root"},
		{"send", NewSendMessageError(root), ErrSendMessage, "root"},
		{"event", NewEventFailedError("OnConnect", root), ErrEventFailed, "OnConnect"},
		{"server_stopped", NewServerStoppedError(root), ErrServerStopped, "root"},
		{"client_not_found", NewClientNotFoundError("id-1"), ErrClientNotFound, "id-1"},
		{"with_only_server", NewWithOnlyServerError("type"), ErrWithOnlyServer, "type"},
		{"invalid_port", NewInvalidPortError(70000), ErrInvalidPort, "70000"},
		{"auth", NewAuthFailureError(root), ErrAuthFailure, "root"},
		{"client_id_gen", NewClientIdGeneratorError(root), ErrClientIdGenerator, "root"},
		{"max_depth", NewMaxDepthExceededError(3), ErrMaxDepthExceeded, "3"},
		{"max_key", NewMaxKeyLengthExceededError(4), ErrMaxKeyLengthExceeded, "4"},
		{"max_elements", NewMaxElementsExceededError(5), ErrMaxElementsExceeded, "5"},
		{"max_string", NewMaxStringLengthExceededError(6), ErrMaxStringLengthExceeded, "6"},
		{"type_not_allowed", NewTypeNotAllowedError("chan int"), ErrTypeNotAllowed, "chan int"},
		{"invalid_value", NewInvalidValueError("bad"), ErroInvalidValue, "bad"},
		{"invalid_json", NewInvalidJsonError(root), ErrInvalidJSON, "root"},
		{"invalid_struct", NewInvalidStructError(root), ErrInvalidStruct, "root"},
		{"data_too_long", NewDataTooLongError(100), ErrDataTooLong, "100"},
		{"max_conn_per_ip", NewMaxConnPerIpReachedError("127.0.0.1"), ErrMaxConnPerIpReached, "127.0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.err, tt.target)
			assert.Contains(t, tt.err.Error(), tt.contain)
		})
	}
}
