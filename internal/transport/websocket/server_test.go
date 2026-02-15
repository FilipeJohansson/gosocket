package websocket

import (
	"testing"

	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/runtime"
	"github.com/stretchr/testify/assert"
)

func TestServer_NewServer(t *testing.T) {
	rt, err := runtime.NewRuntime(runtime.Config{
		HubConfig: hub.DefaultHubConfig(),
	})
	assert.NoError(t, err)

	handler, err := NewHandler()
	assert.NoError(t, err)

	server, err := NewServer(rt, handler)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, rt, server.runtime)
	assert.Equal(t, handler, server.handler)
	assert.NotNil(t, server.Config)
	assert.Equal(t, 8080, server.Config.Port)
	assert.Equal(t, "/ws", server.Config.Path)
}

func TestServer_NewServerNilRuntime(t *testing.T) {
	handler, err := NewHandler()
	assert.NoError(t, err)

	server, err := NewServer(nil, handler)
	assert.Error(t, err)
	assert.Nil(t, server)
}

func TestServer_NewServerNilHandler(t *testing.T) {
	rt, err := runtime.NewRuntime(runtime.Config{
		HubConfig: hub.DefaultHubConfig(),
	})
	assert.NoError(t, err)

	server, err := NewServer(rt, nil)
	assert.Error(t, err)
	assert.Nil(t, server)
}

func TestServer_DefaultServerConfig(t *testing.T) {
	defaultConfig := DefaultServerConfig()
	cfg := &ServerConfig{
		Port:       8080,
		Path:       "/ws",
		EnableCORS: true,
		EnableSSL:  false,
	}

	assert.Equal(t, defaultConfig, cfg)
}

func TestServer_Running(t *testing.T) {
	rt, err := runtime.NewRuntime(runtime.Config{
		HubConfig: hub.DefaultHubConfig(),
	})
	assert.NoError(t, err)

	handler, err := NewHandler()
	assert.NoError(t, err)

	server, err := NewServer(rt, handler)
	assert.NoError(t, err)

	// Initially should not be running
	assert.False(t, server.isRunning.Load())
}

func TestServer_Handler(t *testing.T) {
	rt, err := runtime.NewRuntime(runtime.Config{
		HubConfig: hub.DefaultHubConfig(),
	})
	assert.NoError(t, err)

	handler, err := NewHandler()
	assert.NoError(t, err)

	server, err := NewServer(rt, handler)
	assert.NoError(t, err)

	retrieved := server.Handler()
	assert.Equal(t, handler, retrieved)
}
