// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

/**
 * dispatcher.go defines the Dispatcher interface and core dispatching
 * contracts.
 *
 * The Dispatcher is the single outbound message routing boundary, responsible
 * for deciding whether messages are delivered locally, published to the
 * cluster, or both.
 *
 * All message delivery must pass through a Dispatcher implementation.
 *
 * MUST NOT contain Hub state, transport logic, or cluster backend
 * implementations.
 */

import (
	"context"

	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
)

type Stats struct {
	NumClients int
	NumRooms   int
}

type Dispatcher interface {
	Start(ctx context.Context) error
	Stop() error

	RegisterClient(*hub.Client) error
	UnregisterClient(clientID string) error
	DisconnectClient(clientID string) error
	DisconnectAll() error //? need to be replicated?

	JoinRoom(clientID, roomName string) error
	LeaveRoom(clientID, roomName string) error
	CreateRoom(clientID, roomName string) (*hub.Room, error) //? need to be replicated?
	DeleteRoom(clientID, roomName string) error              //? need to be replicated?

	SendToClient(clientID string, message *message.Message) error
	Broadcast(message *message.Message) error
	BroadcastToRoom(roomName string, message *message.Message) error

	GetClients() map[string]ClientDTO
	GetRooms() map[string]RoomDTO
	GetClientsInRoom(roomName string) map[string]ClientDTO
	GetClientsGlobal(ctx context.Context) ([]store.ClientLocation, error)
	GetRoomsGlobal(ctx context.Context) ([]store.RoomInfo, error)
	GetClientsInRoomGlobal(ctx context.Context, roomName string) ([]store.ClientPresence, error)

	GetStats() Stats // local: read from hub, cluster: local + cluster
}
