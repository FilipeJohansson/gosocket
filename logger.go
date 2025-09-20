// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import "fmt"

type LogType string

const (
	LogTypeServer     LogType = "server"     // for server events
	LogTypeClient     LogType = "client"     // for client events
	LogTypeAuth       LogType = "auth"       // for auth events
	LogTypeBroadcast  LogType = "broadcast"  // for messages sent to multiple clients/rooms
	LogTypeConnection LogType = "connection" // for connection events
	LogTypeMessage    LogType = "message"    // for messages sent and received
	LogTypeError      LogType = "error"      // for internal errors and connection errors
	LogTypeRateLimit  LogType = "ratelimit"  // for rate limit events
	LogTypeOther      LogType = "other"      // generic
)

type LogLevel int

const (
	LogLevelNone LogLevel = iota
	LogLevelError
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

type Logger interface {
	Log(logType LogType, level LogLevel, msg string, args ...interface{})
}

type DefaultLogger struct{}

func (l *DefaultLogger) Log(logType LogType, level LogLevel, msg string, args ...interface{}) {
	prefix := ""
	switch level {
	case LogLevelError:
		prefix = "[ERROR]"
	case LogLevelInfo:
		prefix = "[INFO]"
	case LogLevelDebug:
		prefix = "[DEBUG]"
	}
	fmt.Printf("%s [%s] %s\n", prefix, logType, fmt.Sprintf(msg, args...))
}

type NullLogger struct{}

func (l *NullLogger) Log(logType LogType, level LogLevel, msg string, args ...interface{}) {}
