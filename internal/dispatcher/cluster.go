// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

/**
 * cluster.go implements a Dispatcher that augments local dispatching with
 * distributed message propagation.
 *
 * It publishes cluster events when required and applies remote events
 * locally while preventing message loops.
 *
 * This file is the only place where cluster-aware dispatching decisions
 * are allowed.
 *
 * MUST NOT modify Hub state directly or allow messages to bypass loop
 * protection.
 */

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type ClusterDispatcher struct {
	local   *LocalDispatcher
	manager cluster.Manager
	state   store.StateStore
	nodeID  string

	sub cluster.Subscription

	ctx     context.Context
	cancel  context.CancelFunc
	running atomic.Bool
}

func NewClusterDispatcher(
	local *LocalDispatcher,
	manager cluster.Manager,
	state store.StateStore,
	nodeID string,
) *ClusterDispatcher {
	return &ClusterDispatcher{
		local:   local,
		manager: manager,
		state:   state,
		nodeID:  nodeID,
	}
}

func (d *ClusterDispatcher) Start(ctx context.Context) error {
	if d.running.Load() {
		return nil
	}

	if ctx == nil {
		return errors.New("dispatcher start: nil context")
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	sub, err := d.manager.Subscribe(d.ctx, d.nodeID)
	if err != nil {
		return err
	}
	d.sub = sub

	d.running.Store(true)

	utils.SafeGoroutine("cluster-dispatcher", func() {
		for {
			select {
			case <-d.ctx.Done():
				return
			case evt, ok := <-sub.Events():
				if !ok {
					return
				}
				if err := d.applyRemoteEvent(evt); err != nil {
					// TODO: log
					d.local.hub.Config.Logger.Logger.Log(
						logger.LogTypeMessage,
						logger.LogLevelError,
						"Error applying remote event: %v", err,
					)
				}
			}
		}
	})

	return nil
}

func (d *ClusterDispatcher) Stop() error {
	if !d.running.Load() {
		return nil
	}

	d.running.Store(false)

	if d.cancel != nil {
		d.cancel()
	}

	if d.sub != nil {
		_ = d.sub.Close()
		d.sub = nil
	}

	return nil
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) RegisterClient(client *hub.Client) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.RegisterClient(client); err != nil {
		return err
	}

	if d.state != nil && client != nil {
		if err := d.state.SetClientNode(d.ctx, client.GetID(), d.nodeID); err != nil {
			_ = d.local.UnregisterClient(client.GetID())
			return err
		}
	}

	return nil
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) UnregisterClient(clientID string) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.UnregisterClient(clientID); err != nil {
		return err
	}

	if d.state != nil {
		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		if err := d.state.RemoveClientNode(d.ctx, clientID); err != nil {
			return err
		}
	}

	return nil
}

func (d *ClusterDispatcher) DisconnectClient(clientID string) error {
	return d.local.DisconnectClient(clientID)
}

func (d *ClusterDispatcher) DisconnectAll() error {
	return d.local.DisconnectAll()
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) JoinRoom(clientID, roomName string) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.JoinRoom(clientID, roomName); err != nil {
		return err
	}

	if d.state != nil {
		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		if err := d.state.AddClientToRoom(d.ctx, store.ClientPresence{
			ClientID: clientID,
			RoomName: roomName,
			NodeID:   d.nodeID,
		}); err != nil {
			return err
		}
	}

	return d.publish(&cluster.Event{
		Type:         cluster.EventJoinRoom,
		Origin:       d.nodeID,
		ToClientID:   clientID,
		FromClientID: clientID,
		RoomName:     roomName,
	})
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) LeaveRoom(clientID, roomName string) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.LeaveRoom(clientID, roomName); err != nil {
		return err
	}

	if d.state != nil {
		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		if err := d.state.RemoveClientFromRoom(d.ctx, clientID, roomName); err != nil {
			return err
		}
	}

	return d.publish(&cluster.Event{
		Type:         cluster.EventLeaveRoom,
		Origin:       d.nodeID,
		ToClientID:   clientID,
		FromClientID: clientID,
		RoomName:     roomName,
	})
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) CreateRoom(clientID, roomName string) (*hub.Room, error) {
	if !d.running.Load() {
		return nil, errors.New("dispatcher not started")
	}

	room, err := d.local.CreateRoom(clientID, roomName)
	if err != nil {
		//? publish just when local fails?
		return nil, err
	}

	if d.state != nil {
		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		if err := d.state.UpsertRoom(d.ctx, store.RoomInfo{
			Name:      room.Name(),
			OwnerID:   room.OwnerId(),
			OwnerNode: d.nodeID,
		}); err != nil {
			return nil, err
		}
	}

	if err := d.publish(&cluster.Event{
		Type:         cluster.EventCreateRoom,
		Origin:       d.nodeID,
		ToClientID:   clientID,
		FromClientID: clientID,
		RoomName:     room.Name(),
	}); err != nil {
		return nil, err
	}

	return room, nil
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) DeleteRoom(clientID, roomName string) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.DeleteRoom(clientID, roomName); err != nil {
		return err
	}

	if d.state != nil {
		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		if err := d.state.DeleteRoom(d.ctx, roomName); err != nil {
			return err
		}
	}

	return d.publish(&cluster.Event{
		Type:         cluster.EventDeleteRoom,
		Origin:       d.nodeID,
		ToClientID:   clientID,
		FromClientID: clientID,
		RoomName:     roomName,
	})
}

// ! FIX: it's applied local first. If fails, state stays partially applied.
// ! FIX: apply rollback
// ! FIX: or local commit + async outbox and "degraded propagation" semantic error
// ! FIX: or invert order where makes sense and make sure it's idempotent
func (d *ClusterDispatcher) SendToClient(clientID string, message *message.Message) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	targetNode := ""
	if err := d.local.SendToClient(clientID, message); err != nil {
		if d.state == nil {
			return err
		}

		// ! FIX: if local succeeds and state fails,
		// ! FIX: return error and handler closes without already registered client cleanup
		targetNode, resolveErr := d.state.ResolveClientNode(d.ctx, clientID)
		if resolveErr != nil || targetNode == "" {
			return err
		}
	}

	return d.publish(&cluster.Event{
		Type:         cluster.EventSendToClient,
		MsgType:      message.Type,
		Origin:       d.nodeID,
		TargetNode:   targetNode,
		ToClientID:   clientID,
		FromClientID: message.From,
		Payload:      message.RawData,
		Encoding:     int(message.Encoding),
	})
}

func (d *ClusterDispatcher) GetRooms() map[string]RoomDTO {
	return d.local.GetRooms()
}

func (d *ClusterDispatcher) GetClients() map[string]ClientDTO {
	return d.local.GetClients()
}

func (d *ClusterDispatcher) GetClientsInRoom(roomName string) map[string]ClientDTO {
	return d.local.GetClientsInRoom(roomName)
}

func (d *ClusterDispatcher) GetRoomsGlobal(ctx context.Context) ([]store.RoomInfo, error) {
	if d.state == nil {
		return nil, errors.New("cluster state not configured")
	}
	return d.state.GetRooms(ctx)
}

func (d *ClusterDispatcher) GetClientsGlobal(ctx context.Context) ([]store.ClientLocation, error) {
	if d.state == nil {
		return nil, errors.New("cluster state not configured")
	}
	return d.state.GetClients(ctx)
}

func (d *ClusterDispatcher) GetClientsInRoomGlobal(ctx context.Context, roomName string) ([]store.ClientPresence, error) {
	if d.state == nil {
		return nil, errors.New("cluster state not configured")
	}
	return d.state.GetClientsInRoom(ctx, roomName)
}

func (d *ClusterDispatcher) Broadcast(message *message.Message) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.Broadcast(message); err != nil {
		return err
	}

	return d.publish(&cluster.Event{
		Type:     cluster.EventBroadcast,
		MsgType:  message.Type,
		Origin:   d.nodeID,
		Payload:  message.RawData,
		Encoding: int(message.Encoding),
	})
}

func (d *ClusterDispatcher) BroadcastToRoom(roomName string, message *message.Message) error {
	if !d.running.Load() {
		return errors.New("dispatcher not started")
	}

	if err := d.local.BroadcastToRoom(roomName, message); err != nil {
		return err
	}

	return d.publish(&cluster.Event{
		Type:     cluster.EventBroadcastToRoom,
		MsgType:  message.Type,
		Origin:   d.nodeID,
		RoomName: roomName,
		Payload:  message.RawData,
		Encoding: int(message.Encoding),
	})
}

func (d *ClusterDispatcher) GetStats() Stats {
	return Stats{} // TODO: implement
}

func (d *ClusterDispatcher) applyRemoteEvent(evt *cluster.Event) error {
	if !d.running.Load() {
		return nil
	}

	if evt == nil || d.isLocalOrigin(evt) {
		return nil
	}
	if evt.TargetNode != "" && evt.TargetNode != d.nodeID {
		return nil
	}

	switch evt.Type {
	case cluster.EventSendToClient:
		msg := &message.Message{
			Type:     evt.MsgType,
			To:       evt.ToClientID,
			From:     evt.FromClientID,
			RawData:  evt.Payload,
			Encoding: message.EncodingType(evt.Encoding),
		}
		m, err := normalizeMessage(msg, normalizeOptions{
			toClientID: evt.ToClientID,
		}, d.local.serializers)
		if err != nil {
			return err
		}

		return d.local.SendToClient(evt.ToClientID, m)
	case cluster.EventBroadcast:
		msg := &message.Message{
			Type:     evt.MsgType,
			RawData:  evt.Payload,
			Encoding: message.EncodingType(evt.Encoding),
		}
		m, err := normalizeMessage(msg, normalizeOptions{}, d.local.serializers)
		if err != nil {
			return err
		}

		return d.local.Broadcast(m)
	case cluster.EventBroadcastToRoom:
		msg := &message.Message{
			Type:     evt.MsgType,
			Room:     evt.RoomName,
			RawData:  evt.Payload,
			Encoding: message.EncodingType(evt.Encoding),
		}
		m, err := normalizeMessage(msg, normalizeOptions{
			roomName: evt.RoomName,
		}, d.local.serializers)
		if err != nil {
			return err
		}

		return d.local.BroadcastToRoom(evt.RoomName, m)
	}

	return nil
}

func (d *ClusterDispatcher) publish(evt *cluster.Event) error {
	if evt == nil {
		return nil
	}
	return d.manager.Publish(evt)
}

func (d *ClusterDispatcher) isLocalOrigin(evt *cluster.Event) bool {
	return evt.Origin == d.nodeID
}
