package gosocket

import (
	"testing"

	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigs(t *testing.T) {
	logImpl, logLevels := DefaultLoggerConfig()
	require.NotNil(t, logImpl)
	require.NotNil(t, logLevels)
	assert.Equal(t, logger.LogLevelInfo, logLevels[logger.LogTypeServer])

	rate := DefaultRateLimiterConfig()
	require.NotNil(t, rate)
	assert.Positive(t, rate.PerClientRate)
	assert.Positive(t, rate.PerIPRate)

	ser := DefaultSerializerConfig()
	assert.True(t, ser.EnableStrict)
	assert.Positive(t, ser.MaxDepth)
	assert.Positive(t, ser.MaxBinarySize)

	handlerCfg := DefaultHandlerConfig()
	require.NotNil(t, handlerCfg)
	assert.NotNil(t, handlerCfg.CheckOrigin)
	assert.NotNil(t, handlerCfg.Serializers)

	serverCfg := DefaultServerConfig()
	require.NotNil(t, serverCfg)
	assert.Equal(t, 8080, serverCfg.Port)
	assert.Equal(t, "/ws", serverCfg.Path)

	hubCfg := DefaultHubConfig()
	require.NotNil(t, hubCfg)
	require.NotNil(t, hubCfg.Logger)
	assert.NotNil(t, hubCfg.Logger.Logger)
	assert.NotNil(t, hubCfg.Logger.Level)
}
