package logger

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	require.NoError(t, w.Close())
	os.Stdout = old
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

func TestDefaultLoggerConfig(t *testing.T) {
	l, levels := DefaultLoggerConfig()
	assert.IsType(t, &DefaultLogger{}, l)
	assert.Equal(t, LogLevelInfo, levels[LogTypeServer])
	assert.Equal(t, LogLevelError, levels[LogTypeError])
}

func TestDefaultLogger_Log(t *testing.T) {
	l := &DefaultLogger{}
	output := captureStdout(t, func() {
		l.Log(LogTypeServer, LogLevelError, "error %d", 1)
		l.Log(LogTypeServer, LogLevelInfo, "info")
		l.Log(LogTypeServer, LogLevelDebug, "debug")
	})

	assert.Contains(t, output, "[ERROR] [server] error 1")
	assert.Contains(t, output, "[INFO] [server] info")
	assert.Contains(t, output, "[DEBUG] [server] debug")
}

func TestNullLoggerConfigAndLog(t *testing.T) {
	l, levels := NullLoggerConfig()
	assert.IsType(t, &NullLogger{}, l)
	assert.Empty(t, levels)

	output := captureStdout(t, func() {
		l.Log(LogTypeOther, LogLevelInfo, "ignored")
	})
	assert.Empty(t, output)
}
