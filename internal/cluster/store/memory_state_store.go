// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package store

import (
	"context"
	"sync"
)

// MemoryStateStore is an in-memory StateStore implementation intended for
// development and tests. It is process-local and does not provide durability.
type MemoryStateStore struct {
	mu sync.RWMutex

	rooms       map[string]RoomInfo
	roomMembers map[string]map[string]ClientPresence // roomName -> clientID -> presence
	clientNodes map[string]string                    // clientID -> nodeID
}

// NewMemoryStateStore returns a new MemoryStateStore.
// It is process-local and does not provide durability.
// It is intended for development and tests.
func NewMemoryStateStore() StateStore {
	return &MemoryStateStore{
		rooms:       make(map[string]RoomInfo),
		roomMembers: make(map[string]map[string]ClientPresence),
		clientNodes: make(map[string]string),
	}
}

func (s *MemoryStateStore) UpsertRoom(ctx context.Context, room RoomInfo) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if room.Name == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.rooms[room.Name] = room
	return nil
}

func (s *MemoryStateStore) DeleteRoom(ctx context.Context, roomName string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if roomName == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.rooms, roomName)
	delete(s.roomMembers, roomName)
	return nil
}

func (s *MemoryStateStore) GetRooms(ctx context.Context) ([]RoomInfo, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]RoomInfo, 0, len(s.rooms))
	for _, room := range s.rooms {
		out = append(out, room)
	}
	return out, nil
}

func (s *MemoryStateStore) AddClientToRoom(ctx context.Context, p ClientPresence) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if p.ClientID == "" || p.RoomName == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.roomMembers[p.RoomName]; !ok {
		s.roomMembers[p.RoomName] = make(map[string]ClientPresence)
	}
	s.roomMembers[p.RoomName][p.ClientID] = p
	return nil
}

func (s *MemoryStateStore) RemoveClientFromRoom(ctx context.Context, clientID, roomName string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if clientID == "" || roomName == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	clients, ok := s.roomMembers[roomName]
	if !ok {
		return nil
	}
	delete(clients, clientID)
	if len(clients) == 0 {
		delete(s.roomMembers, roomName)
	}
	return nil
}

func (s *MemoryStateStore) GetClientsInRoom(ctx context.Context, roomName string) ([]ClientPresence, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	if roomName == "" {
		return []ClientPresence{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	clients, ok := s.roomMembers[roomName]
	if !ok {
		return []ClientPresence{}, nil
	}

	out := make([]ClientPresence, 0, len(clients))
	for _, p := range clients {
		out = append(out, p)
	}
	return out, nil
}

func (s *MemoryStateStore) SetClientNode(ctx context.Context, clientID, nodeID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if clientID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientNodes[clientID] = nodeID
	return nil
}

func (s *MemoryStateStore) RemoveClientNode(ctx context.Context, clientID string) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if clientID == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clientNodes, clientID)
	return nil
}

func (s *MemoryStateStore) ResolveClientNode(ctx context.Context, clientID string) (string, error) {
	if err := ctxErr(ctx); err != nil {
		return "", err
	}
	if clientID == "" {
		return "", nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clientNodes[clientID], nil
}

func (s *MemoryStateStore) GetClients(ctx context.Context) ([]ClientLocation, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ClientLocation, 0, len(s.clientNodes))
	for clientID, nodeID := range s.clientNodes {
		out = append(out, ClientLocation{
			ClientID: clientID,
			NodeID:   nodeID,
		})
	}
	return out, nil
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
