package hub

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/stretchr/testify/assert"
)

func startHub(t *testing.T, h *Hub) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	assert.Eventually(t, func() bool { return h.Running() }, 2*time.Second, 10*time.Millisecond, "hub did not start")
	return ctx, cancel
}

func waitClient(t *testing.T, h *Hub, clientID string) {
	assert.Eventually(t, func() bool { return h.GetClient(clientID) != nil }, 2*time.Second, 10*time.Millisecond, "client %s not available", clientID)
}

func waitNoClient(t *testing.T, h *Hub, clientID string) {
	assert.Eventually(t, func() bool { return h.GetClient(clientID) == nil }, 2*time.Second, 10*time.Millisecond, "client %s still present", clientID)
}

func TestHub_NewHub(t *testing.T) {
	tests := []struct {
		name     string
		config   *HubConfig
		expected func(*Hub)
	}{
		{
			name:   "creates hub with nil config",
			config: nil,
			expected: func(h *Hub) {
				assert.NotNil(t, h.Config)
				assert.NotNil(t, h.Config.Logger)
				assert.Equal(t, DropNewest, h.Config.BackpressurePolicy)
			},
		},
		{
			name: "creates hub with custom config",
			config: &HubConfig{
				Logger:             nil,
				BackpressurePolicy: DropOldest,
			},
			expected: func(h *Hub) {
				assert.NotNil(t, h.Config)
				assert.Equal(t, DropOldest, h.Config.BackpressurePolicy)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub(tt.config)
			assert.NotNil(t, hub)
			tt.expected(hub)
		})
	}
}

func TestHub_HubRun(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		hub.Run(ctx)
		close(done)
	}()

	assert.Eventually(t, func() bool { return hub.Running() }, 2*time.Second, 10*time.Millisecond, "hub should be running")

	cancel()
	<-done

	assert.False(t, hub.Running(), "Hub should not be running after context cancel")
}

func TestHub_AddClient(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	err := hub.AddClient(client)
	assert.NoError(t, err)

	waitClient(t, hub, "client-1")
	retrieved := hub.GetClient("client-1")
	assert.NotNil(t, retrieved)
	assert.Equal(t, "client-1", retrieved.GetID())
}

func TestHub_GetClient(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("test-client", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "test-client")

	tests := []struct {
		name     string
		clientID string
		expected *Client
	}{
		{"existing client", "test-client", client},
		{"nonexistent client", "nonexistent", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hub.GetClient(tt.clientID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHub_GetClients(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client1 := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	client2 := NewClient("client-2", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	_ = hub.AddClient(client1)
	_ = hub.AddClient(client2)
	assert.Eventually(t, func() bool { return len(hub.GetClients()) == 2 }, 2*time.Second, 10*time.Millisecond)

	clients := hub.GetClients()
	assert.Equal(t, 2, len(clients))
	assert.NotNil(t, clients["client-1"])
	assert.NotNil(t, clients["client-2"])
}

func TestHub_RemoveClient(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	assert.NotNil(t, hub.GetClient("client-1"))

	err := hub.RemoveClient("client-1")
	assert.NoError(t, err)
	waitNoClient(t, hub, "client-1")
}

func TestHub_SendToClient(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	msg := &message.Message{
		Type:    message.TextMessage,
		RawData: []byte("test message"),
	}

	err := hub.SendToClient("client-1", msg)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool { return len(client.SendChan) > 0 }, 2*time.Second, 10*time.Millisecond)
	select {
	case received := <-client.SendChan:
		assert.Equal(t, msg, received)
	default:
		t.Fatal("Message not delivered")
	}
}

func TestHub_SendToClientNotFound(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	msg := &message.Message{
		Type:    message.TextMessage,
		RawData: []byte("test"),
	}

	err := hub.SendToClient("nonexistent", msg)
	assert.Error(t, err)
	assert.Equal(t, errors.ErrClientNotFound, err)
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client1 := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	client2 := NewClient("client-2", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	_ = hub.AddClient(client1)
	_ = hub.AddClient(client2)
	assert.Eventually(t, func() bool { return len(hub.GetClients()) == 2 }, 2*time.Second, 10*time.Millisecond)

	msg := &message.Message{
		Type:    message.TextMessage,
		RawData: []byte("broadcast"),
	}

	err := hub.Broadcast(msg)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool { return len(client1.SendChan) > 0 && len(client2.SendChan) > 0 }, 2*time.Second, 10*time.Millisecond)

	select {
	case <-client1.SendChan:
	default:
		t.Fatal("Client1 didn't receive broadcast")
	}

	select {
	case <-client2.SendChan:
	default:
		t.Fatal("Client2 didn't receive broadcast")
	}
}

func TestHub_CreateRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	room, err := hub.CreateRoom("owner", "test-room")
	assert.NoError(t, err)
	assert.NotNil(t, room)
	assert.Equal(t, "test-room", room.Name())
}

func TestHub_CreateRoomEmptyName(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	_, err := hub.CreateRoom("owner", "")
	assert.Error(t, err)
	assert.Equal(t, errors.ErrRoomNameEmpty, err)
}

func TestHub_JoinRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	room, _ := hub.CreateRoom("owner", "test-room")
	err := hub.JoinRoom("client-1", "test-room")
	assert.NoError(t, err)

	assert.Eventually(t, func() bool {
		clients := hub.GetClientsInRoom(room.ID())
		return len(clients) == 1 && clients["client-1"] != nil
	}, 2*time.Second, 10*time.Millisecond)
}

func TestHub_LeaveRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	room, _ := hub.CreateRoom("owner", "test-room")
	_ = hub.JoinRoom("client-1", "test-room")
	assert.Eventually(t, func() bool { return len(hub.GetClientsInRoom(room.ID())) == 1 }, 2*time.Second, 10*time.Millisecond)

	err := hub.LeaveRoom("client-1", "test-room")
	assert.NoError(t, err)

	assert.Eventually(t, func() bool { return len(hub.GetClientsInRoom(room.ID())) == 0 }, 2*time.Second, 10*time.Millisecond)
}

func TestHub_BroadcastToRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client1 := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	client2 := NewClient("client-2", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	_ = hub.AddClient(client1)
	_ = hub.AddClient(client2)
	assert.Eventually(t, func() bool { return len(hub.GetClients()) == 2 }, 2*time.Second, 10*time.Millisecond)

	room, _ := hub.CreateRoom("owner", "test-room")
	_ = hub.JoinRoom("client-1", "test-room")
	assert.Eventually(t, func() bool { return len(hub.GetClientsInRoom(room.ID())) == 1 }, 2*time.Second, 10*time.Millisecond)

	msg := &message.Message{
		Type:    message.TextMessage,
		RawData: []byte("room broadcast"),
	}

	err := hub.BroadcastToRoom("test-room", msg)
	assert.NoError(t, err)

	assert.Eventually(t, func() bool { return len(client1.SendChan) > 0 }, 2*time.Second, 10*time.Millisecond)

	select {
	case <-client1.SendChan:
	default:
		t.Fatal("Client1 didn't receive room broadcast")
	}

	select {
	case <-client2.SendChan:
		t.Fatal("Client2 should not receive room broadcast")
	default:
	}
}

func TestHub_DeleteRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	room, _ := hub.CreateRoom("owner", "test-room")
	err := hub.DeleteRoom(room.ID())
	assert.NoError(t, err)

	_, err = hub.GetRoom(room.ID())
	assert.Error(t, err)
}

func TestHub_GetRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	created, _ := hub.CreateRoom("owner", "test-room")
	retrieved, err := hub.GetRoom(created.ID())

	assert.NoError(t, err)
	assert.Equal(t, created.ID(), retrieved.ID())
}

func TestHub_DeleteEmptyRooms(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	_, _ = hub.CreateRoom("owner", "room1")
	_, _ = hub.CreateRoom("owner", "room2")
	_, _ = hub.CreateRoom("owner", "room3")

	deleted := hub.DeleteEmptyRooms()
	assert.Equal(t, 3, len(deleted))

	rooms := hub.GetRooms()
	assert.Equal(t, 0, len(rooms))
}

func TestHub_GetClientsInRoom(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client1 := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	client2 := NewClient("client-2", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	_ = hub.AddClient(client1)
	_ = hub.AddClient(client2)
	assert.Eventually(t, func() bool { return len(hub.GetClients()) == 2 }, 2*time.Second, 10*time.Millisecond)

	room, _ := hub.CreateRoom("owner", "test-room")
	_ = hub.JoinRoom("client-1", "test-room")
	_ = hub.JoinRoom("client-2", "test-room")
	assert.Eventually(t, func() bool { return len(hub.GetClientsInRoom(room.ID())) == 2 }, 2*time.Second, 10*time.Millisecond)
}

func TestHub_BackpressureDropNewest(t *testing.T) {
	cfg := &HubConfig{
		Logger:             nil,
		BackpressurePolicy: DropNewest,
	}
	hub := NewHub(cfg)
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 2)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	// Fill channel
	_ = hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg1")})
	_ = hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg2")})

	// This should be dropped (DropNewest policy)
	err := hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg3")})
	assert.Error(t, err)
}

func TestHub_BackpressureDropOldest(t *testing.T) {
	cfg := &HubConfig{
		Logger:             nil,
		BackpressurePolicy: DropOldest,
	}
	hub := NewHub(cfg)
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 2)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	// Fill channel
	_ = hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg1")})
	_ = hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg2")})

	// This should succeed (DropOldest policy - drops msg1)
	err := hub.SendToClient("client-1", &message.Message{Type: message.TextMessage, RawData: []byte("msg3")})
	assert.NoError(t, err)

	dropped := hub.DroppedMessages()
	assert.Greater(t, dropped, uint64(0))
}

func TestHub_ConcurrentClientOperations(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	var wg sync.WaitGroup
	numClients := 50

	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()
			clientID := fmt.Sprintf("client-%d", id)
			client := NewClient(clientID, &MockWebSocketConn{}, &ConnectionInfo{}, 256)
			_ = hub.AddClient(client)
		}(i)
	}

	wg.Wait()
	assert.Eventually(t, func() bool { return len(hub.GetClients()) == numClients }, 2*time.Second, 10*time.Millisecond)
}

func TestHub_ConcurrentRoomOperations(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	var wg sync.WaitGroup
	numRooms := 50

	wg.Add(numRooms)
	for i := 0; i < numRooms; i++ {
		go func(id int) {
			defer wg.Done()
			roomName := fmt.Sprintf("room-%d", id)
			_, _ = hub.CreateRoom("owner", roomName)
		}(i)
	}

	wg.Wait()
	assert.Eventually(t, func() bool { return len(hub.GetRooms()) == numRooms }, 2*time.Second, 10*time.Millisecond)
}

func TestHub_GetStats(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	client := NewClient("client-1", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	_, _ = hub.CreateRoom("owner", "test-room")
	_ = hub.JoinRoom("client-1", "test-room")

	stats := hub.GetStats()
	assert.Equal(t, 1, stats.ActiveConnections)
	assert.Equal(t, 1, stats.TotalRooms)
}

func TestHub_HubRunning(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	assert.False(t, hub.Running())

	_, cancel := startHub(t, hub)

	assert.True(t, hub.Running())

	cancel()
	assert.Eventually(t, func() bool { return !hub.Running() }, 2*time.Second, 10*time.Millisecond)

	assert.False(t, hub.Running())
}

func TestHub_DisconnectClient(t *testing.T) {
	hub := NewHub(DefaultHubConfig())
	_, cancel := startHub(t, hub)
	defer cancel()

	mockConn := &MockWebSocketConn{}
	mockConn.On("Close").Return(nil)

	client := NewClient("client-1", mockConn, &ConnectionInfo{}, 256)
	_ = hub.AddClient(client)
	waitClient(t, hub, "client-1")

	err := hub.DisconnectClient("client-1")
	assert.NoError(t, err)

	waitNoClient(t, hub, "client-1")
}
