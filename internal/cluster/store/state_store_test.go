package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopStateStore(t *testing.T) {
	s := NewNoopStateStore()
	require.NotNil(t, s)

	ctx := context.Background()

	require.NoError(t, s.UpsertRoom(ctx, RoomInfo{Name: "room-1"}))
	require.NoError(t, s.DeleteRoom(ctx, "room-1"))

	rooms, err := s.GetRooms(ctx)
	require.NoError(t, err)
	require.Nil(t, rooms)

	require.NoError(t, s.AddClientToRoom(ctx, ClientPresence{ClientID: "c1", RoomName: "room-1", NodeID: "n1"}))
	require.NoError(t, s.RemoveClientFromRoom(ctx, "c1", "room-1"))

	presence, err := s.GetClientsInRoom(ctx, "room-1")
	require.NoError(t, err)
	require.Nil(t, presence)

	require.NoError(t, s.SetClientNode(ctx, "c1", "n1"))
	require.NoError(t, s.RemoveClientNode(ctx, "c1"))

	nodeID, err := s.ResolveClientNode(ctx, "c1")
	require.NoError(t, err)
	require.Equal(t, "", nodeID)

	clients, err := s.GetClients(ctx)
	require.NoError(t, err)
	require.Nil(t, clients)
}
