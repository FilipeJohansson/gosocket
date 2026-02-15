// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package hub

/**
 * hub.go defines the Hub, which is the core domain model of GoSocket.
 *
 * The Hub owns local state such as connected clients, rooms, and message
 * delivery rules.
 *
 * It represents intent only (send, broadcast, join, leave) and delegates
 * actual delivery decisions to the Dispatcher.
 *
 * MUST NOT be aware of clusters, networking, transports, or distributed
 * concerns of any kind.
 */

import (
	"context"
	"fmt"
	"hash/fnv"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/errors"
	"github.com/FilipeJohansson/gosocket/internal/logger"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/stats"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type HubConfig struct {
	Logger             *logger.LoggerConfig
	BackpressurePolicy BackpressurePolicy
}

func DefaultHubConfig() *HubConfig {
	defaultLogger, defaultLevels := logger.DefaultLoggerConfig()
	return &HubConfig{
		Logger:             &logger.LoggerConfig{Logger: defaultLogger, Level: defaultLevels},
		BackpressurePolicy: DropNewest,
	}
}

type registerRequest struct {
	client *Client
	done   chan struct{}
}

type contextHolder struct {
	ctx context.Context
}

type Hub struct {
	Config *HubConfig

	ctx         atomic.Value
	clients     *utils.SharedCollection[*Client, string]
	rooms       *utils.SharedCollection[*Room, string]  // room id -> room
	roomsByName *utils.SharedCollection[string, string] // room name -> room id (roomID/roomName)
	roomsMu     sync.RWMutex

	register   chan registerRequest
	unregister chan *Client
	broadcast  chan *message.Message

	droppedMessages atomic.Uint64

	running atomic.Bool
}

func NewHub(cfg *HubConfig) *Hub {
	if cfg == nil {
		cfg = DefaultHubConfig()
	}

	if cfg.Logger == nil {
		defaultLogger, defaultLevels := logger.DefaultLoggerConfig()
		cfg.Logger = &logger.LoggerConfig{Logger: defaultLogger, Level: defaultLevels}
	}

	h := &Hub{
		Config:      cfg,
		clients:     utils.NewSharedCollection[*Client, string](),
		rooms:       utils.NewSharedCollection[*Room, string](),
		roomsByName: utils.NewSharedCollection[string, string](),

		register:   make(chan registerRequest),
		unregister: make(chan *Client),
		broadcast:  make(chan *message.Message, 1024), // TODO: make configurable
	}

	h.ctx.Store(contextHolder{ctx: context.Background()})

	return h
}

func (h *Hub) Run(ctx context.Context) {
	if h.running.Load() {
		h.log(logger.LogTypeError, logger.LogLevelError, "Hub is already running")
		return
	}

	h.running.Store(true)

	defer func() {
		h.running.Store(false)
		h.log(logger.LogTypeOther, logger.LogLevelInfo, "Hub stopped")
	}()
	h.log(logger.LogTypeOther, logger.LogLevelInfo, "Hub is running...")

	h.ctx.Store(contextHolder{ctx: ctx})

	for {
		select {
		case <-ctx.Done():
			h.log(logger.LogTypeOther, logger.LogLevelInfo, "Hub stopping (context cancelled)")
			return

		case req, ok := <-h.register:
			if !ok {
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Register channel closed")
				return
			}
			if req.client != nil {
				h.clients.Add(req.client, req.client.GetID())
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Client registered: %s", req.client.GetID())
			}
			if req.done != nil {
				close(req.done)
			}

			select {
			case <-ctx.Done():
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Register channel closed (context cancelled)")
				return
			default:
			}

		case client, ok := <-h.unregister:
			if client == nil {
				continue
			}
			if !ok {
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Unregister channel closed")
				return
			}

			select {
			case <-ctx.Done():
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Unregister channel closed (context cancelled)")
				return
			default:
				if _, exists := h.clients.Get(client.GetID()); !exists {
					h.log(logger.LogTypeClient, logger.LogLevelDebug, "Client not found: %s", client.GetID())
					continue
				}

				id := client.GetID()
				if h.clients.Remove(id) {
					h.log(
						logger.LogTypeClient,
						logger.LogLevelDebug,
						"Client unregistered: %s",
						id,
					)
				}

				h.leaveClientFromAllRoomsUnsafe(id)

				h.safeCloseClientChannel(client)
				h.log(logger.LogTypeClient, logger.LogLevelDebug, "Client unregistering: %s", client.GetID())
			}

		case msg, ok := <-h.broadcast:
			if msg == nil {
				continue
			}
			if !ok {
				h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Broadcast channel closed")
				return
			}

			select {
			case <-ctx.Done():
				h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Broadcast channel closed (context cancelled)")
				return
			default:
				clients := h.clients.GetAll()
				h.broadcastToClients(msg, clients)
				h.log(logger.LogTypeMessage, logger.LogLevelDebug, "Broadcasting message to all clients")
			}
		}
	}
}

// SendToClient delivers a message to a single local client.
func (h *Hub) SendToClient(clientID string, message *message.Message) error {
	client := h.GetClient(clientID)
	if client == nil {
		h.log(
			logger.LogTypeClient,
			logger.LogLevelError,
			"SendToClient: failed to find client %s",
			clientID,
		)
		return errors.ErrClientNotFound
	}

	return h.sendWithBackpressure(client, message)
}

func (h *Hub) Broadcast(message *message.Message) error {
	ctx := h.ctx.Load().(contextHolder).ctx
	select {
	case h.broadcast <- message:
		h.log(
			logger.LogTypeMessage,
			logger.LogLevelDebug,
			"Broadcast: message broadcasted to all clients",
		)
		return nil

	case <-ctx.Done():
		return errors.ErrHubStopped

	default:
		h.log(
			logger.LogTypeMessage,
			logger.LogLevelError,
			"Broadcast: failed to broadcast message to all clients",
		)
		return errors.ErrBroadcastFull
	}
}

func (h *Hub) BroadcastToRoom(roomName string, message *message.Message) error {
	if roomName == "" {
		h.log(
			logger.LogTypeRoom,
			logger.LogLevelError,
			"BroadcastToRoom: room name is empty",
		)
		return errors.ErrRoomNameEmpty
	}

	if message == nil {
		h.log(
			logger.LogTypeMessage,
			logger.LogLevelError,
			"BroadcastToRoom: message is nil",
		)
		return errors.ErrNilMessage
	}

	h.roomsMu.RLock()
	roomId, exists := h.roomsByName.Get(roomName)
	h.roomsMu.RUnlock()

	if !exists {
		h.log(
			logger.LogTypeRoom,
			logger.LogLevelError,
			"BroadcastToRoom: failed to find room %s",
			roomName,
		)
		return errors.ErrRoomNotFound
	}

	room, exists := h.rooms.Get(roomId)
	if !exists {
		h.log(
			logger.LogTypeRoom,
			logger.LogLevelError,
			"BroadcastToRoom: failed to find room %s",
			roomName,
		)
		return errors.ErrRoomNotFound
	}

	var resultErr error = nil
	clientsCopy := room.Clients()
	for _, client := range clientsCopy {
		if err := h.sendWithBackpressure(client, message); err != nil {
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"BroadcastToRoom: failed to send message to client %s",
				client.GetID(),
			)
			resultErr = errors.ErrBroadcastToRoomSomeClientFull
		}
	}

	h.log(
		logger.LogTypeMessage,
		logger.LogLevelDebug,
		"BroadcastToRoom: message broadcasted to room %s",
		roomName,
	)
	return resultErr
}

func (h *Hub) AddClient(c *Client) error {
	ctx := h.ctx.Load().(contextHolder).ctx

	done := make(chan struct{}, 1)

	select {
	case h.register <- registerRequest{client: c, done: done}:
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return errors.ErrHubStopped
		}
	case <-ctx.Done():
		return errors.ErrHubStopped
	}
}

func (h *Hub) RemoveClient(clientID string) error {
	ctx := h.ctx.Load().(contextHolder).ctx
	select {
	case h.unregister <- &Client{
		id: clientID,
	}:
		return nil
	case <-ctx.Done():
		return errors.ErrHubStopped
	}
}

// GetClient returns the client with the given ID. If the client does not exist, nil is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetClient(id string) *Client {
	client, exists := h.clients.Get(id)
	if !exists {
		return nil
	}
	return client
}

// GetClients returns a copy of the clients map, where the keys are the client pointers
// and the values are booleans indicating whether the client is connected to the hub.
//
// This method is safe to call concurrently.
func (h *Hub) GetClients() map[string]*Client {
	return h.clients.GetAll()
}

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the name is empty, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) CreateRoom(ownerId, roomName string, customId ...string) (*Room, error) {
	if roomName == "" {
		return nil, errors.ErrRoomNameEmpty
	}

	h.roomsMu.Lock()
	defer h.roomsMu.Unlock()

	if _, exists := h.roomsByName.Get(roomName); exists {
		return nil, errors.ErrRoomAlreadyExists
	}

	var roomId string
	if len(customId) > 0 && customId[0] != "" {
		roomId = customId[0]
	} else {
		for {
			hash := fnv.New64a()
			_, err := hash.Write([]byte(roomName + fmt.Sprint(time.Now().UnixNano())))
			if err != nil {
				return nil, err
			}
			roomId = fmt.Sprintf("%x", hash.Sum64())
			if _, exists := h.rooms.Get(roomId); !exists {
				break
			}
		}
	}

	if _, exists := h.rooms.Get(roomId); exists {
		return nil, errors.ErrRoomAlreadyExists
	}

	room := NewRoom(roomId, ownerId, roomName)
	h.rooms.Add(room, roomId)
	h.roomsByName.Add(roomId, roomName)

	h.log(logger.LogTypeRoom, logger.LogLevelInfo, "Room created: %s", roomName)
	return room, nil
}

// DeleteRoom deletes a room with the given name. If the room does not exist, it will return
// an error. If the room exists, it will remove all clients from the room and remove the room
// from the hub.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteRoom(roomName string) error {
	roomID := roomName
	room, exists := h.rooms.Get(roomID)
	if !exists {
		h.roomsMu.RLock()
		resolvedID, byName := h.roomsByName.Get(roomName)
		h.roomsMu.RUnlock()
		if !byName {
			return errors.NewRoomNotFoundError(roomName)
		}
		roomID = resolvedID
		room, exists = h.rooms.Get(roomID)
		if !exists {
			return errors.NewRoomNotFoundError(roomName)
		}
	}

	h.roomsMu.Lock()
	h.roomsByName.Remove(room.name)
	h.roomsMu.Unlock()

	err := h.LeaveAllFromRoom(roomID)
	if err != nil {
		return err
	}

	if h.rooms.Remove(roomID) {
		h.log(logger.LogTypeRoom, logger.LogLevelInfo, "Room deleted: %s", roomID)
	}
	return nil
}

// JoinRoom adds a client to a room. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) JoinRoom(clientID string, roomName string) error {
	if roomName == "" {
		return errors.ErrRoomNameEmpty
	}

	client := h.GetClient(clientID)
	if client == nil {
		h.log(
			logger.LogTypeClient,
			logger.LogLevelError,
			"JoinRoom: failed to find client %s",
			clientID,
		)
		return errors.ErrClientNotFound
	}

	h.roomsMu.RLock()
	roomId, exists := h.roomsByName.Get(roomName)
	h.roomsMu.RUnlock()

	if !exists {
		return errors.NewRoomNotFoundError(roomName)
	}

	room, exists := h.rooms.Get(roomId)
	if !exists {
		h.log(logger.LogTypeRoom, logger.LogLevelError, "Room not found: %s", roomName)
		return errors.NewRoomNotFoundError(roomName)
	}

	// check if client is already in room
	if room.HasClient(clientID) {
		h.log(logger.LogTypeRoom, logger.LogLevelError, "Client already in room: %s", clientID)
		return errors.ErrClientAlreadyInRoom
	}

	room.AddClient(client)
	h.log(logger.LogTypeRoom, logger.LogLevelInfo, "Room %s: client %s joined", roomName, client.GetID())
	return nil
}

// LeaveRoom removes the given client from the given room. If the room does not exist, or if the client is not in the room, the method does nothing.
//
// This method is safe to call concurrently.
func (h *Hub) LeaveRoom(clientID string, roomName string) error {
	if roomName == "" {
		return errors.ErrRoomNameEmpty
	}

	client := h.GetClient(clientID)
	if client == nil {
		h.log(
			logger.LogTypeClient,
			logger.LogLevelError,
			"LeaveRoom: failed to find client %s",
			clientID,
		)
		return errors.ErrClientNotFound
	}

	h.roomsMu.RLock()
	roomId, exists := h.roomsByName.Get(roomName)
	h.roomsMu.RUnlock()

	if !exists {
		return errors.NewRoomNotFoundError(roomName)
	}

	if room, exists := h.rooms.Get(roomId); exists {
		if room.RemoveClient(client.GetID()) {
			h.log(logger.LogTypeRoom, logger.LogLevelInfo, "Room %s: client %s left", roomName, client.GetID())
		}
	}

	return nil
}

func (h *Hub) DisconnectClient(clientID string) error {
	if clientID == "" {
		return errors.ErrInvalidArguments
	}

	client := h.GetClient(clientID)
	if client == nil {
		return errors.ErrClientNotFound
	}

	// remove from hub, take care of:
	// - leave all rooms
	// - close SendChan
	// - right lifecycle
	if err := h.RemoveClient(clientID); err != nil {
		return err
	}

	// close connection if exists
	if client.Conn != nil {
		_ = client.Conn.Close()
	}

	h.log(
		logger.LogTypeClient,
		logger.LogLevelInfo,
		"Client disconnected by hub: %s",
		clientID,
	)

	return nil
}

// DeleteEmptyRooms deletes all empty rooms from the hub.
// Returns a slice of room IDs that were deleted.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteEmptyRooms() []string {
	var deletedRooms []string

	h.rooms.ForEach(func(roomId string, room *Room) {
		if room.IsEmpty() {
			if err := h.DeleteRoom(roomId); err == nil {
				deletedRooms = append(deletedRooms, roomId)
			}
		}
	})

	h.log(logger.LogTypeRoom, logger.LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

// DeleteEmptyRoomsExcluding deletes all empty rooms from the hub, except for the rooms with the given IDs.
// Returns a slice of room IDs that were deleted.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteEmptyRoomsExcluding(excludeIds []string) []string {
	var deletedRooms []string

	h.rooms.ForEach(func(roomId string, room *Room) {
		excluded := false
		for _, id := range excludeIds {
			if roomId == id {
				excluded = true
				break
			}
		}

		if !excluded && room.IsEmpty() {
			if err := h.DeleteRoom(roomId); err == nil {
				deletedRooms = append(deletedRooms, roomId)
			}
		}
	})

	h.log(logger.LogTypeRoom, logger.LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

// LeaveAllFromRoom removes all clients from the given room. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) LeaveAllFromRoom(roomId string) error {
	room, exists := h.rooms.Get(roomId)
	if !exists {
		return errors.NewRoomNotFoundError(roomId)
	}

	var clientsRemoved []string
	for id := range room.Clients() {
		if room.RemoveClient(id) {
			clientsRemoved = append(clientsRemoved, id)
		}
	}

	h.log(logger.LogTypeRoom, logger.LogLevelDebug, "Removed %d clients from room: %s", len(clientsRemoved), roomId)
	h.log(logger.LogTypeRoom, logger.LogLevelInfo, "Room %s: all clients left", roomId)
	return nil
}

// GetClientsInRoom returns all clients in the specified room. If the room does not exist,
// an empty slice is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetClientsInRoom(roomIDOrName string) map[string]*Client {
	room, exists := h.rooms.Get(roomIDOrName)
	if exists {
		return room.Clients()
	}

	h.roomsMu.RLock()
	roomID, foundByName := h.roomsByName.Get(roomIDOrName)
	h.roomsMu.RUnlock()
	if !foundByName {
		return map[string]*Client{}
	}

	room, exists = h.rooms.Get(roomID)
	if !exists {
		return map[string]*Client{}
	}

	return room.Clients()
}

// GetRooms returns a copy of the rooms map, where the keys are the room IDs and the values are the room pointers.
//
// This method is safe to call concurrently.
func (h *Hub) GetRooms() map[string]*Room {
	return h.rooms.GetAll()
}

// GetRoomById returns the room with the given ID. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetRoom(roomId string) (*Room, error) {
	room, exists := h.rooms.Get(roomId)
	if !exists {
		return nil, errors.NewRoomNotFoundError(roomId)
	}
	return room, nil
}

func (h *Hub) DroppedMessages() uint64 {
	return h.droppedMessages.Load()
}

func (h *Hub) Running() bool { return h.running.Load() }

// GetStats returns a map with the following keys:
//
// - total_clients: The total number of clients connected to the hub.
// - total_rooms: The total number of rooms in the hub.
// - rooms: A map with room names as keys and the number of clients in each room as values.
//
// This method is safe to call concurrently.
func (h *Hub) GetStats() stats.HubStats {
	//* basic stats
	totalClients := h.clients.Len()
	totalRooms := h.rooms.Len()

	//* room stats
	roomStats := make(map[string]stats.RoomStat)
	h.rooms.ForEach(func(id string, room *Room) {
		roomStats[id] = stats.RoomStat{
			ID:          room.ID(),
			Name:        room.Name(),
			ClientCount: len(room.Clients()),
			OwnerID:     room.OwnerId(),
			CreatedAt:   room.CreatedAt(),
		}
	})

	return stats.HubStats{
		ActiveConnections: totalClients,
		TotalRooms:        totalRooms,
		RoomStats:         roomStats,
		DroppedMessages:   h.droppedMessages.Load(),
	}
}

// broadcastToClients broadcasts the given message to all clients in the given clients map.
//
// If a client's message channel is full or closed, it is removed from the hub.
//
// This method is safe to call concurrently, as it takes a read lock on the hub's clients map.
func (h *Hub) broadcastToClients(message *message.Message, clients map[string]*Client) {
	defer func() {
		if r := recover(); r != nil {
			h.log(logger.LogTypeBroadcast, logger.LogLevelError, "PANIC RECOVERED in broadcastToClients: %v\nStack trace:\n%s\n", r, string(debug.Stack()))
		}
	}()

	if message.RawData == nil {
		h.log(logger.LogTypeBroadcast, logger.LogLevelError, "Broadcast message has no data to send")
		return
	}

	var clientsToRemove []*Client
	for _, client := range clients {
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.log(logger.LogTypeBroadcast, logger.LogLevelError, "PANIC RECOVERED sending to client %s: %v\n", client.GetID(), r)
					clientsToRemove = append(clientsToRemove, client)
				}
			}()

			if err := h.sendWithBackpressure(client, message); err != nil {
				h.log(logger.LogTypeBroadcast, logger.LogLevelDebug,
					"Client %s backpressure drop", client.GetID())
			}
		}()
	}

	if len(clientsToRemove) > 0 {
		utils.SafeGoroutine("RemoveProblematicClients", func() {
			for _, client := range clientsToRemove {
				_ = h.RemoveClient(client.GetID())
			}
		})
	}
}

func (h *Hub) sendWithBackpressure(client *Client, msg *message.Message) error {
	switch h.Config.BackpressurePolicy {
	case DropNewest:
		select {
		case client.SendChan <- msg:
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"message sent to client %s",
				client.GetID(),
			)
			return nil
		default:
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"failed to send message to client %s",
				client.GetID(),
			)
			h.droppedMessages.Add(1)
			return errors.ErrClientFull
		}

	case DropOldest:
		select {
		case client.SendChan <- msg:
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"message sent to client %s",
				client.GetID(),
			)
			return nil
		default:
			select {
			case <-client.SendChan:
				// drop oldest
				h.log(
					logger.LogTypeMessage,
					logger.LogLevelDebug,
					"dropped oldest message to client %s",
					client.GetID(),
				)
				h.droppedMessages.Add(1)
			default:
			}
			select {
			case client.SendChan <- msg:
				h.log(
					logger.LogTypeMessage,
					logger.LogLevelDebug,
					"message sent to client %s",
					client.GetID(),
				)
				return nil
			default:
				h.log(
					logger.LogTypeMessage,
					logger.LogLevelDebug,
					"failed to send message to client %s",
					client.GetID(),
				)
				h.droppedMessages.Add(1)
				return errors.ErrClientFull
			}
		}

	case Block:
		select {
		case client.SendChan <- msg:
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"message sent to client %s",
				client.GetID(),
			)
			return nil
		case <-h.ctx.Load().(contextHolder).ctx.Done():
			h.log(
				logger.LogTypeMessage,
				logger.LogLevelDebug,
				"failed to send message to client %s",
				client.GetID(),
			)
			return errors.ErrHubStopped
		}
	}

	return errors.ErrClientFull
}

func (h *Hub) leaveClientFromAllRoomsUnsafe(clientId string) {
	var roomsToRemove []*Room
	h.rooms.ForEach(func(roomId string, room *Room) {
		if _, exists := room.GetClient(clientId); exists {
			roomsToRemove = append(roomsToRemove, room)
		}
	})

	for _, room := range roomsToRemove {
		client := h.GetClient(clientId)
		if client == nil {
			continue
		}
		room.RemoveClient(client.GetID())
	}
}

func (h *Hub) log(t logger.LogType, l logger.LogLevel, msg string, args ...interface{}) {
	conf := h.Config.Logger
	if conf == nil {
		return
	}

	lvl, ok := conf.Level[t]
	if !ok {
		lvl = logger.LogLevelNone
	}

	if l <= lvl {
		conf.Logger.Log(t, l, msg, args...)
	}
}

func (h *Hub) safeCloseClientChannel(client *Client) {
	defer func() {
		if r := recover(); r != nil {
			h.log(logger.LogTypeClient, logger.LogLevelError, "PANIC RECOVERED closing channel for client %s: %v\n", client.GetID(), r)
		}
	}()

	if client.SendChan != nil {
		if client.closed.CompareAndSwap(false, true) {
			h.log(logger.LogTypeClient, logger.LogLevelDebug, "Closing client %s channel\n", client.GetID())
			close(client.SendChan)
		}
	}
}
