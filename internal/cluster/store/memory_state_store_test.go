// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package store

import (
	"context"
	"testing"
)

func TestMemoryStateStore_Rooms(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	if err := store.UpsertRoom(ctx, RoomInfo{Name: "room-1", OwnerID: "owner-1"}); err != nil {
		t.Fatalf("UpsertRoom failed: %v", err)
	}
	if err := store.UpsertRoom(ctx, RoomInfo{Name: "room-2", OwnerID: "owner-2"}); err != nil {
		t.Fatalf("UpsertRoom failed: %v", err)
	}

	rooms, err := store.GetRooms(ctx)
	if err != nil {
		t.Fatalf("GetRooms failed: %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("unexpected number of rooms: got %d, want 2", len(rooms))
	}

	if err := store.DeleteRoom(ctx, "room-1"); err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	rooms, err = store.GetRooms(ctx)
	if err != nil {
		t.Fatalf("GetRooms failed: %v", err)
	}
	if len(rooms) != 1 {
		t.Fatalf("unexpected number of rooms after delete: got %d, want 1", len(rooms))
	}
}

func TestMemoryStateStore_Presence(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	if err := store.AddClientToRoom(ctx, ClientPresence{
		ClientID: "c1",
		RoomName: "r1",
		NodeID:   "n1",
	}); err != nil {
		t.Fatalf("AddClientToRoom failed: %v", err)
	}
	if err := store.AddClientToRoom(ctx, ClientPresence{
		ClientID: "c2",
		RoomName: "r1",
		NodeID:   "n2",
	}); err != nil {
		t.Fatalf("AddClientToRoom failed: %v", err)
	}

	clients, err := store.GetClientsInRoom(ctx, "r1")
	if err != nil {
		t.Fatalf("GetClientsInRoom failed: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("unexpected number of clients: got %d, want 2", len(clients))
	}

	if err := store.RemoveClientFromRoom(ctx, "c1", "r1"); err != nil {
		t.Fatalf("RemoveClientFromRoom failed: %v", err)
	}
	clients, err = store.GetClientsInRoom(ctx, "r1")
	if err != nil {
		t.Fatalf("GetClientsInRoom failed: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("unexpected number of clients after removal: got %d, want 1", len(clients))
	}
}

func TestMemoryStateStore_ClientNode(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	if err := store.SetClientNode(ctx, "c1", "node-A"); err != nil {
		t.Fatalf("SetClientNode failed: %v", err)
	}

	node, err := store.ResolveClientNode(ctx, "c1")
	if err != nil {
		t.Fatalf("ResolveClientNode failed: %v", err)
	}
	if node != "node-A" {
		t.Fatalf("unexpected node: got %q, want %q", node, "node-A")
	}

	if err := store.RemoveClientNode(ctx, "c1"); err != nil {
		t.Fatalf("RemoveClientNode failed: %v", err)
	}
	node, err = store.ResolveClientNode(ctx, "c1")
	if err != nil {
		t.Fatalf("ResolveClientNode failed: %v", err)
	}
	if node != "" {
		t.Fatalf("expected empty node after removal, got %q", node)
	}
}

func TestMemoryStateStore_GetClients(t *testing.T) {
	store := NewMemoryStateStore()
	ctx := context.Background()

	if err := store.SetClientNode(ctx, "c1", "node-A"); err != nil {
		t.Fatalf("SetClientNode failed: %v", err)
	}
	if err := store.SetClientNode(ctx, "c2", "node-B"); err != nil {
		t.Fatalf("SetClientNode failed: %v", err)
	}

	clients, err := store.GetClients(ctx)
	if err != nil {
		t.Fatalf("GetClients failed: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("unexpected number of clients: got %d, want 2", len(clients))
	}
}

func TestMemoryStateStore_ContextCanceled(t *testing.T) {
	store := NewMemoryStateStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.UpsertRoom(ctx, RoomInfo{Name: "r1"}); err == nil {
		t.Fatal("expected canceled context error, got nil")
	}
}
