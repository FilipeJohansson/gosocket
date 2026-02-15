package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/stretchr/testify/require"
)

func startHubForDispatcherTest(t *testing.T, h *hub.Hub) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	require.Eventually(t, func() bool { return h.Running() }, 2*time.Second, 10*time.Millisecond)
	return cancel
}

func waitClientPresence(t *testing.T, h *hub.Hub, id string, shouldExist bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		c := h.GetClient(id)
		if shouldExist {
			return c != nil
		}
		return c == nil
	}, 2*time.Second, 10*time.Millisecond)
}

func testSerializers() map[message.EncodingType]message.Serializer {
	cfg := message.DefaultSerializerConfig()
	return map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(cfg),
		message.Raw:  message.NewRawSerializer(cfg),
	}
}

func TestLocalDispatcher_LifecycleAndValidation(t *testing.T) {
	h := hub.NewHub(hub.DefaultHubConfig())
	d := NewLocalDispatcher(h, testSerializers())

	require.NoError(t, d.Start(context.Background()))
	require.NoError(t, d.Stop())
	require.Equal(t, Stats{}, d.GetStats())

	require.ErrorIs(t, d.RegisterClient(nil), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.UnregisterClient(""), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.JoinRoom("", "room"), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.JoinRoom("c1", ""), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.LeaveRoom("", "room"), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.LeaveRoom("c1", ""), errors.ErrInvalidArguments)
	_, err := d.CreateRoom("", "room")
	require.ErrorIs(t, err, errors.ErrInvalidArguments)
	_, err = d.CreateRoom("c1", "")
	require.ErrorIs(t, err, errors.ErrInvalidArguments)
	require.ErrorIs(t, d.DeleteRoom("", "room"), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.DeleteRoom("c1", ""), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.SendToClient("", &message.Message{}), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.SendToClient("c1", nil), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.Broadcast(nil), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.BroadcastToRoom("", &message.Message{}), errors.ErrInvalidArguments)
	require.ErrorIs(t, d.BroadcastToRoom("room", nil), errors.ErrInvalidArguments)

	_, err = d.GetRoomsGlobal(context.Background())
	require.ErrorIs(t, err, errors.ErrClusterManagerNotProvided)
	_, err = d.GetClientsGlobal(context.Background())
	require.ErrorIs(t, err, errors.ErrClusterManagerNotProvided)
	_, err = d.GetClientsInRoomGlobal(context.Background(), "room")
	require.ErrorIs(t, err, errors.ErrClusterManagerNotProvided)
}

func TestLocalDispatcher_Flow(t *testing.T) {
	h := hub.NewHub(hub.DefaultHubConfig())
	cancel := startHubForDispatcherTest(t, h)
	defer cancel()

	d := NewLocalDispatcher(h, testSerializers())

	c1 := hub.NewClient("c1", nil, nil, 16)
	c1.SetUserData("role", "owner")
	c2 := hub.NewClient("c2", nil, nil, 16)

	require.NoError(t, d.RegisterClient(c1))
	require.NoError(t, d.RegisterClient(c2))
	waitClientPresence(t, h, "c1", true)
	waitClientPresence(t, h, "c2", true)

	_, err := d.CreateRoom("c1", "room-1")
	require.NoError(t, err)
	require.NoError(t, d.JoinRoom("c1", "room-1"))

	clients := d.GetClients()
	require.Contains(t, clients, "c1")
	require.Equal(t, "owner", clients["c1"].UserData["role"])

	rooms := d.GetRooms()
	require.Contains(t, rooms, "room-1")
	require.Equal(t, 1, rooms["room-1"].ClientCount)

	inRoom := d.GetClientsInRoom("room-1")
	require.Contains(t, inRoom, "c1")
	require.NotContains(t, inRoom, "c2")

	sendMsg := message.NewRawMessage(message.TextMessage, []byte("direct"))
	require.NoError(t, d.SendToClient("c1", sendMsg))
	select {
	case got := <-c1.SendChan:
		require.Equal(t, []byte("direct"), got.RawData)
		require.Equal(t, "c1", got.To)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting direct message")
	}

	broadcastMsg := message.NewRawMessage(message.TextMessage, []byte("all"))
	require.NoError(t, d.Broadcast(broadcastMsg))
	select {
	case got := <-c1.SendChan:
		require.Equal(t, []byte("all"), got.RawData)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting broadcast for c1")
	}
	select {
	case got := <-c2.SendChan:
		require.Equal(t, []byte("all"), got.RawData)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting broadcast for c2")
	}

	roomMsg := message.NewRawMessage(message.TextMessage, []byte("room-only"))
	require.NoError(t, d.BroadcastToRoom("room-1", roomMsg))
	select {
	case got := <-c1.SendChan:
		require.Equal(t, []byte("room-only"), got.RawData)
		require.Equal(t, "room-1", got.Room)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting room broadcast for c1")
	}
	select {
	case <-c2.SendChan:
		t.Fatal("c2 should not receive room-only broadcast")
	case <-time.After(100 * time.Millisecond):
	}

	require.NoError(t, d.LeaveRoom("c1", "room-1"))
	require.NoError(t, d.DeleteRoom("c1", "room-1"))
	require.NoError(t, d.DisconnectClient("c1"))
	waitClientPresence(t, h, "c1", false)

	require.NoError(t, d.DisconnectAll())
	waitClientPresence(t, h, "c2", false)
}

