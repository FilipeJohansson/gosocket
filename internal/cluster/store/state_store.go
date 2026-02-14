// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package store

import (
	"context"
)

type RoomInfo struct {
	Name      string
	OwnerID   string
	OwnerNode string
	ShardKey  string
}

type ClientPresence struct {
	ClientID string
	RoomName string
	NodeID   string
}

type ClientLocation struct {
	ClientID string
	NodeID   string
}

type StateStore interface {
	UpsertRoom(ctx context.Context, room RoomInfo) error
	DeleteRoom(ctx context.Context, roomName string) error
	GetRooms(ctx context.Context) ([]RoomInfo, error)

	AddClientToRoom(ctx context.Context, p ClientPresence) error
	RemoveClientFromRoom(ctx context.Context, clientID, roomName string) error
	GetClientsInRoom(ctx context.Context, roomName string) ([]ClientPresence, error)

	SetClientNode(ctx context.Context, clientID, nodeID string) error
	RemoveClientNode(ctx context.Context, clientID string) error
	ResolveClientNode(ctx context.Context, clientID string) (string, error)
	GetClients(ctx context.Context) ([]ClientLocation, error)
}

type NoopStateStore struct{}

func NewNoopStateStore() StateStore { return &NoopStateStore{} }

func (n *NoopStateStore) UpsertRoom(ctx context.Context, room RoomInfo) error {
	return nil
}
func (n *NoopStateStore) DeleteRoom(ctx context.Context, roomName string) error {
	return nil
}
func (n *NoopStateStore) GetRooms(ctx context.Context) ([]RoomInfo, error) {
	return nil, nil
}

func (n *NoopStateStore) AddClientToRoom(ctx context.Context, p ClientPresence) error {
	return nil
}
func (n *NoopStateStore) RemoveClientFromRoom(ctx context.Context, clientID, roomName string) error {
	return nil
}
func (n *NoopStateStore) GetClientsInRoom(ctx context.Context, roomName string) ([]ClientPresence, error) {
	return nil, nil
}

func (n *NoopStateStore) SetClientNode(ctx context.Context, clientID, nodeID string) error {
	return nil
}
func (n *NoopStateStore) RemoveClientNode(ctx context.Context, clientID string) error {
	return nil
}
func (n *NoopStateStore) ResolveClientNode(ctx context.Context, clientID string) (string, error) {
	return "", nil
}
func (n *NoopStateStore) GetClients(ctx context.Context) ([]ClientLocation, error) {
	return nil, nil
}
