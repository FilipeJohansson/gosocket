// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

/**
 * local.go implements a Dispatcher that delivers messages strictly within
 * the local Runtime.
 *
 * It is responsible for applying messages to the Hub without any knowledge
 * of clustering or distributed systems.
 *
 * This dispatcher is used in standalone and non-clustered scenarios.
 *
 * MUST NOT publish cluster events or reference cluster-specific concepts.
 */

import (
	"context"

	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
)

type LocalDispatcher struct {
	hub         *hub.Hub
	serializers map[message.EncodingType]message.Serializer
}

func NewLocalDispatcher(
	h *hub.Hub,
	serializers map[message.EncodingType]message.Serializer,
) *LocalDispatcher {
	return &LocalDispatcher{
		hub:         h,
		serializers: serializers,
	}
}

func (d *LocalDispatcher) Start(ctx context.Context) error {
	return nil
}

func (d *LocalDispatcher) Stop() error {
	return nil
}

func (d *LocalDispatcher) RegisterClient(client *hub.Client) error {
	if client == nil {
		return errors.ErrInvalidArguments
	}

	return d.hub.AddClient(client)
}

func (d *LocalDispatcher) UnregisterClient(clientID string) error {
	if clientID == "" {
		return errors.ErrInvalidArguments
	}

	return d.hub.RemoveClient(clientID)
}

func (d *LocalDispatcher) DisconnectClient(clientID string) error {
	return d.hub.DisconnectClient(clientID)
}

func (d *LocalDispatcher) DisconnectAll() error {
	clients := d.hub.GetClients()
	for id := range clients {
		_ = d.hub.DisconnectClient(id)
	}
	return nil
}

func (d *LocalDispatcher) JoinRoom(clientID, roomName string) error {
	if clientID == "" || roomName == "" {
		return errors.ErrInvalidArguments
	}

	return d.hub.JoinRoom(clientID, roomName)
}

func (d *LocalDispatcher) LeaveRoom(clientID, roomName string) error {
	if clientID == "" || roomName == "" {
		return errors.ErrInvalidArguments
	}

	return d.hub.LeaveRoom(clientID, roomName)
}

func (d *LocalDispatcher) CreateRoom(clientID, roomName string) (*hub.Room, error) {
	if clientID == "" || roomName == "" {
		return nil, errors.ErrInvalidArguments
	}

	return d.hub.CreateRoom(clientID, roomName)
}

func (d *LocalDispatcher) DeleteRoom(clientID, roomName string) error {
	if clientID == "" || roomName == "" {
		return errors.ErrInvalidArguments
	}

	return d.hub.DeleteRoom(roomName)
}

func (d *LocalDispatcher) SendToClient(clientID string, msg *message.Message) error {
	if clientID == "" || msg == nil {
		// TODO: log
		// d.hub.log(
		// 	logger.LogTypeMessage,
		// 	logger.LogLevelError,
		// 	"SendToClient: invalid arguments",
		// )
		return errors.ErrInvalidArguments
	}

	// h.log(
	// 	logger.LogTypeMessage,
	// 	logger.LogLevelDebug,
	// 	"SendToClient: sending message to client %s",
	// 	clientID,
	// )

	m, err := normalizeMessage(msg, normalizeOptions{
		toClientID: clientID,
	}, d.serializers)
	if err != nil {
		return err
	}

	return d.hub.SendToClient(clientID, m)
}

func (d *LocalDispatcher) Broadcast(msg *message.Message) error {
	if msg == nil {
		// h.log(
		// 	logger.LogTypeMessage,
		// 	logger.LogLevelError,
		// 	"Broadcast: invalid arguments",
		// )
		return errors.ErrInvalidArguments
	}

	// h.log(
	// 	logger.LogTypeMessage,
	// 	logger.LogLevelDebug,
	// 	"Broadcast: broadcasting message to all clients",
	// )

	m, err := normalizeMessage(msg, normalizeOptions{}, d.serializers)
	if err != nil {
		return err
	}

	return d.hub.Broadcast(m)
}

func (d *LocalDispatcher) BroadcastToRoom(roomName string, msg *message.Message) error {
	if roomName == "" || msg == nil {
		// h.log(
		// 	logger.LogTypeMessage,
		// 	logger.LogLevelError,
		// 	"BroadcastToRoom: invalid arguments",
		// )
		return errors.ErrInvalidArguments
	}

	// h.log(
	// 	logger.LogTypeMessage,
	// 	logger.LogLevelDebug,
	// 	"BroadcastToRoom: broadcasting message to room %s",
	// 	roomName,
	// )

	m, err := normalizeMessage(msg, normalizeOptions{
		roomName: roomName,
	}, d.serializers)
	if err != nil {
		return err
	}

	return d.hub.BroadcastToRoom(roomName, m)
}

func (d *LocalDispatcher) GetClients() map[string]ClientDTO {
	return toClientsDTO(d.hub.GetClients())
}

func (d *LocalDispatcher) GetRooms() map[string]RoomDTO {
	return toRoomsDTO(d.hub.GetRooms())
}

func (d *LocalDispatcher) GetClientsInRoom(roomName string) map[string]ClientDTO {
	return toClientsDTO(d.hub.GetClientsInRoom(roomName))
}

func (d *LocalDispatcher) GetRoomsGlobal(ctx context.Context) ([]store.RoomInfo, error) {
	_ = ctx
	return nil, errors.ErrClusterManagerNotProvided
}

func (d *LocalDispatcher) GetClientsGlobal(ctx context.Context) ([]store.ClientLocation, error) {
	_ = ctx
	return nil, errors.ErrClusterManagerNotProvided
}

func (d *LocalDispatcher) GetClientsInRoomGlobal(ctx context.Context, roomName string) ([]store.ClientPresence, error) {
	_ = ctx
	_ = roomName
	return nil, errors.ErrClusterManagerNotProvided
}

func (d *LocalDispatcher) GetStats() Stats {
	return Stats{} // TODO: implement
}
