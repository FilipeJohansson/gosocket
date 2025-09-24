// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"runtime/debug"
	"sync"
	"time"
)

// IHub is an interface for a hub that manages client connections and rooms.
//
// It provides methods for running and stopping the hub, managing clients and rooms,
// broadcasting messages, and retrieving statistics and client information.
type IHub interface {
	// Run starts the hub.
	Run(ctx context.Context)

	// Stop stops the hub.
	Stop()

	// AddClient adds a client to the hub.
	AddClient(client *Client)

	// RemoveClient removes a client from the hub.
	RemoveClient(client *Client)

	// SendToClient sends a message from one client to a specific client.
	SendToClient(fromClientId, toClientId string, message *Message) error

	// BroadcastMessage sends a message to all connected clients.
	BroadcastMessage(message *Message)

	// BroadcastToRoom sends a message to all clients in a specific room.
	BroadcastToRoom(roomId string, message *Message) error

	// CreateRoom creates a new room with the given name.
	CreateRoom(ownerId, roomName string, customId ...string) (*Room, error)

	// JoinRoom adds a client to a room.
	JoinRoom(client *Client, roomId string) error

	// LeaveRoom removes a client from a room.
	LeaveRoom(client *Client, roomId string)

	// DeleteRoom deletes a room with the given name and returns whether the room was deleted.
	DeleteRoom(roomId string) error

	// DeleteEmptyRooms deletes all empty rooms from the hub.
	DeleteEmptyRooms() []string

	// DeleteEmptyRoomsExcluding deletes all empty rooms from the hub, excluding the rooms with the given IDs.
	DeleteEmptyRoomsExcluding(excludeIds []string) []string

	// LeaveAllFromRoom removes all clients from a room.
	LeaveAllFromRoom(roomId string) error

	// GetClientsInRoom returns a list of clients in a room.
	GetClientsInRoom(roomId string) map[string]*Client

	// GetStats returns statistics about the hub.
	GetStats() map[string]interface{}

	// GetClients returns a list of all connected clients.
	GetClients() map[string]*Client

	// GetClient returns a client with the given ID.
	GetClient(id string) *Client

	// GetRooms returns a list of all rooms.
	GetRooms() map[string]*Room

	// GetRoom returns a room with the given name.
	GetRoom(roomId string) (*Room, error)

	// IsRunning returns whether the hub is currently running.
	IsRunning() bool

	Log(logType LogType, level LogLevel, msg string, args ...interface{})
}

type Hub struct {
	Clients    *SharedCollection[*Client, string]
	Rooms      *SharedCollection[*Room, string]
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
	mu         sync.RWMutex
	running    bool

	logger *LoggerConfig
}

// NewHub creates a new Hub instance. It returns a pointer to a Hub struct.
//
// The created Hub instance will have empty maps for the clients and rooms.
// The Register, Unregister and Broadcast channels will be created with a default buffer size.
func NewHub(logger *LoggerConfig) *Hub {
	return &Hub{
		Clients:    NewSharedCollection[*Client, string](),
		Rooms:      NewSharedCollection[*Room, string](),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan *Message),
		logger:     logger,
	}
}

// Run starts the hub. It is a blocking call and will run until the Stop method is called.
//
// It handles the following events:
// - Register: adds a client to the hub and broadcasts the client to all other clients
// - Unregister: removes a client from the hub, closes the client's message channel and removes the client from all rooms
// - Broadcast: broadcasts a message to all clients in the hub
func (h *Hub) Run(ctx context.Context) {
	h.Log(LogTypeOther, LogLevelInfo, "Hub started")
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		h.running = false
		h.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case client, ok := <-h.Register:
			if !ok {
				h.Log(LogTypeClient, LogLevelDebug, "Register channel closed")
				return // channel closed, hub stopped
			}

			select {
			case <-ctx.Done():
				h.Log(LogTypeClient, LogLevelDebug, "Register channel closed")
				return
			default:
				h.Clients.Add(client, client.ID)
				h.Log(LogTypeClient, LogLevelDebug, "Client registered: %s", client.ID)
			}

		case client, ok := <-h.Unregister:
			if !ok {
				h.Log(LogTypeClient, LogLevelDebug, "Unregister channel closed")
				return // channel closed, hub stopped
			}

			select {
			case <-ctx.Done():
				h.Log(LogTypeClient, LogLevelDebug, "Unregister channel closed")
				return
			default:
				if _, exists := h.Clients.Get(client.ID); exists {
					h.Clients.Remove(client.ID)

					h.mu.Lock()
					h.safeCloseClientChannel(client)
					h.mu.Unlock()

					h.removeClientFromAllRoomsUnsafe(client)
					h.Log(LogTypeClient, LogLevelDebug, "Client unregistered: %s", client.ID)
				}
			}

		case message, ok := <-h.Broadcast:
			if !ok {
				h.Log(LogTypeBroadcast, LogLevelDebug, "Broadcast channel closed")
				return // channel closed, hub stopped
			}

			select {
			case <-ctx.Done():
				return
			default:
				clients := h.Clients.GetAll()
				h.mu.RLock()
				h.broadcastToClients(message, clients)
				h.mu.RUnlock()
			}
		}
	}
}

// Stop stops the hub. It is a blocking call and will not return until the hub is
// fully stopped.
//
// It will close the message channel of all clients and reset the hub's state.
//
// The Stop method is safe to call concurrently.
func (h *Hub) Stop() {
	h.Log(LogTypeOther, LogLevelInfo, "Hub stopping...")

	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false

	h.Clients.ForEach(func(id string, client *Client) {
		h.safeCloseClientChannel(client)
	})

	h.Clients = NewSharedCollection[*Client, string]()
	h.Rooms = NewSharedCollection[*Room, string]()
	h.mu.Unlock()

	h.safeCloseChannel(h.Register)
	h.safeCloseChannel(h.Unregister)
	h.safeCloseChannel(h.Broadcast)

	h.Log(LogTypeOther, LogLevelInfo, "Hub stopped")
}

// AddClient adds a client to the hub, and broadcasts the client to all other clients.
// It does not return an error, even if the client is already in the hub.
func (h *Hub) AddClient(client *Client) {
	h.Register <- client
}

// RemoveClient removes a client from the hub and closes the client's message channel.
// It does not return an error, even if the client is not in the hub.
func (h *Hub) RemoveClient(client *Client) {
	h.Unregister <- client
}

func (h *Hub) SendToClient(fromClientId, toClientId string, message *Message) error {
	if fromClientId == "" || toClientId == "" {
		return ErrInvalidClientId
	} else if message == nil || (message.RawData == nil && message.Data == nil) {
		return ErrNoDataToSend
	}

	fromClient := h.GetClient(fromClientId)
	if fromClient == nil {
		h.Log(LogTypeClient, LogLevelError, "SendToClient: From client not found: %s", fromClientId)
		return ErrClientNotFound
	}

	fromClient.mu.Lock()
	if fromClient.Conn == nil {
		fromClient.mu.Unlock()
		fromClient.Hub.Log(LogTypeClient, LogLevelError, "SendToClient (ID: %s): Connection is nil", fromClient.ID)
		return ErrClientConnNil
	}
	fromClient.mu.Unlock()

	var toClient *Client
	if fromClientId == toClientId {
		toClient = fromClient
	} else {
		toClient = h.GetClient(toClientId)
		if toClient == nil {
			h.Log(LogTypeClient, LogLevelError, "SendToClient: To client not found: %s", toClientId)
			return ErrClientNotFound
		}

		toClient.mu.Lock()
		if toClient.Conn == nil {
			toClient.mu.Unlock()
			toClient.Hub.Log(LogTypeClient, LogLevelError, "SendToClient (ID: %s): Connection is nil", toClient.ID)
			return ErrClientConnNil
		}
		toClient.mu.Unlock()
	}

	message.From = fromClientId
	message.To = toClientId
	if message.Created.IsZero() {
		message.Created = time.Now()
	}

	h.Log(LogTypeMessage, LogLevelDebug, "Sending message from %s to %s", fromClientId, toClientId)

	select {
	case toClient.MessageChan <- message:
		h.Log(LogTypeMessage, LogLevelDebug, "Message sent to client %s channel", toClientId)
		return nil
	default:
		h.Log(LogTypeClient, LogLevelError, "SendToClient: Client %s channel is full", toClientId)
		safeGoroutine("Client.Disconnect", func() {
			_ = toClient.Disconnect()
		})
		return ErrClientFull
	}
}

// BroadcastMessage broadcasts a message to all clients in the hub.
//
// It is a shorthand for calling Broadcast(h, message).
func (h *Hub) BroadcastMessage(message *Message) {
	h.Broadcast <- message
}

// BroadcastToRoom broadcasts a message to all clients in the given room.
//
// It returns an error if the room does not exist.
//
// It is a shorthand for calling Broadcast(h, message) after getting the clients from the room.
func (h *Hub) BroadcastToRoom(roomId string, message *Message) error {
	room, exists := h.Rooms.Get(roomId)
	if !exists {
		h.Log(LogTypeBroadcast, LogLevelError, "Room not found: %s", roomId)
		return newRoomNotFoundError(roomId)
	}

	clientsCopy := room.Clients()

	h.broadcastToClients(message, clientsCopy)
	h.Log(LogTypeBroadcast, LogLevelDebug, "Message broadcasted to room %s (%d clients)", roomId, len(clientsCopy))
	return nil
}

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the name is empty, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) CreateRoom(ownerId, roomName string, customId ...string) (*Room, error) {
	if roomName == "" {
		return nil, ErrRoomNameEmpty
	}

	roomId := roomName
	if len(customId) > 0 && customId[0] != "" {
		roomId = customId[0]
	}

	if _, exists := h.Rooms.Get(roomId); exists {
		return nil, ErrRoomAlreadyExists
	}

	room := NewRoom(roomId, ownerId, roomName)
	h.Rooms.Add(room, roomId)

	h.Log(LogTypeRoom, LogLevelInfo, "Room created: %s", roomName)
	return room, nil
}

// JoinRoom adds a client to a room. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) JoinRoom(client *Client, roomId string) error {
	room, exists := h.Rooms.Get(roomId)
	if !exists {
		h.Log(LogTypeRoom, LogLevelError, "Room not found: %s", roomId)
		return newRoomNotFoundError(roomId)
	}

	room.AddClient(client)
	h.Log(LogTypeRoom, LogLevelInfo, "Room %s: client %s joined", roomId, client.ID)
	return nil
}

// LeaveRoom removes the given client from the given room. If the room does not exist, or if the client is not in the room, the method does nothing.
//
// This method is safe to call concurrently.
func (h *Hub) LeaveRoom(client *Client, roomId string) {
	if room, exists := h.Rooms.Get(roomId); exists {
		if room.RemoveClient(client.ID) {
			h.Log(LogTypeRoom, LogLevelInfo, "Room %s: client %s left", roomId, client.ID)
		}
	}
}

// DeleteRoom deletes a room with the given name. If the room does not exist, it will return
// an error. If the room exists, it will remove all clients from the room and remove the room
// from the hub.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteRoom(roomId string) error {
	err := h.LeaveAllFromRoom(roomId)
	if err != nil {
		return err
	}

	if h.Rooms.Remove(roomId) {
		h.Log(LogTypeRoom, LogLevelInfo, "Room deleted: %s", roomId)
	}
	return nil
}

// DeleteEmptyRooms deletes all empty rooms from the hub.
// Returns a slice of room IDs that were deleted.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteEmptyRooms() []string {
	var deletedRooms []string

	h.Rooms.ForEach(func(roomId string, room *Room) {
		if room.IsEmpty() {
			if err := h.DeleteRoom(roomId); err == nil {
				deletedRooms = append(deletedRooms, roomId)
			}
		}
	})

	h.Log(LogTypeRoom, LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

// DeleteEmptyRoomsExcluding deletes all empty rooms from the hub, except for the rooms with the given IDs.
// Returns a slice of room IDs that were deleted.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteEmptyRoomsExcluding(excludeIds []string) []string {
	var deletedRooms []string

	h.Rooms.ForEach(func(roomId string, room *Room) {
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

	h.Log(LogTypeRoom, LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

// LeaveAllFromRoom removes all clients from the given room. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) LeaveAllFromRoom(roomId string) error {
	room, exists := h.Rooms.Get(roomId)
	if !exists {
		return newRoomNotFoundError(roomId)
	}

	var clientsRemoved []string
	for id := range room.Clients() {
		if room.RemoveClient(id) {
			clientsRemoved = append(clientsRemoved, id)
		}
	}

	h.Log(LogTypeRoom, LogLevelDebug, "Removed %d clients from room: %s", len(clientsRemoved), roomId)
	h.Log(LogTypeRoom, LogLevelInfo, "Room %s: all clients left", roomId)
	return nil
}

// GetClientsInRoom returns all clients in the specified room. If the room does not exist,
// an empty slice is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetClientsInRoom(roomId string) map[string]*Client {
	room, exists := h.Rooms.Get(roomId)
	if !exists {
		return map[string]*Client{}
	}
	return room.Clients()
}

// GetStats returns a map with the following keys:
//
// - total_clients: The total number of clients connected to the hub.
// - total_rooms: The total number of rooms in the hub.
// - rooms: A map with room names as keys and the number of clients in each room as values.
//
// This method is safe to call concurrently.
func (h *Hub) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["total_clients"] = h.Clients.Len()
	stats["total_rooms"] = h.Rooms.Len()

	roomStats := make(map[string]int)
	h.Rooms.ForEach(func(id string, room *Room) {
		roomStats[room.ID()] = len(room.Clients())
	})
	stats["rooms"] = roomStats

	return stats
}

// GetClients returns a copy of the clients map, where the keys are the client pointers
// and the values are booleans indicating whether the client is connected to the hub.
//
// This method is safe to call concurrently.
func (h *Hub) GetClients() map[string]*Client {
	return h.Clients.GetAll()
}

func (h *Hub) GetClient(id string) *Client {
	client, exists := h.Clients.Get(id)
	if !exists {
		return nil
	}
	return client
}

// GetRooms returns a copy of the rooms map, where the keys are the room IDs and the values are the room pointers.
//
// This method is safe to call concurrently.
func (h *Hub) GetRooms() map[string]*Room {
	return h.Rooms.GetAll()
}

// GetRoomById returns the room with the given ID. If the room does not exist, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetRoom(roomId string) (*Room, error) {
	room, exists := h.Rooms.Get(roomId)
	if !exists {
		return nil, newRoomNotFoundError(roomId)
	}
	return room, nil
}

// IsRunning returns a boolean indicating whether the hub is currently running.
//
// This method is safe to call concurrently.
func (h *Hub) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

func (h *Hub) Log(logType LogType, level LogLevel, msg string, args ...interface{}) {
	lvl, ok := h.logger.Level[logType]
	if !ok {
		lvl = LogLevelNone
	}

	if level <= lvl {
		h.logger.Logger.Log(logType, level, msg, args...)
	}
}

// removeClientFromAllRoomsUnsafe removes the given client from all rooms it is currently in.
// This method is unsafe to call concurrently, as it does not take any locks on the hub's rooms
// map. It should only be called when the hub is not running, or when the hub is already locked
// for writing. This method is used when a client is removed from the hub, to ensure that it is
// removed from all rooms it is currently in. This prevents the client from receiving messages
// from rooms it is no longer in.
func (h *Hub) removeClientFromAllRoomsUnsafe(client *Client) {
	var roomsToProcess []string
	h.Rooms.ForEach(func(id string, room *Room) {
		if _, exists := room.GetClient(client.ID); exists {
			roomsToProcess = append(roomsToProcess, room.Name())
		}
	})

	for _, roomName := range roomsToProcess {
		h.LeaveRoom(client, roomName)
	}
}

// broadcastToClients broadcasts the given message to all clients in the given clients map.
//
// If a client's message channel is full or closed, it is removed from the hub.
//
// This method is safe to call concurrently, as it takes a read lock on the hub's clients map.
func (h *Hub) broadcastToClients(message *Message, clients map[string]*Client) {
	defer func() {
		if r := recover(); r != nil {
			h.Log(LogTypeBroadcast, LogLevelError, "PANIC RECOVERED in broadcastToClients: %v\nStack trace:\n%s\n", r, string(debug.Stack()))
		}
	}()

	if message.RawData == nil {
		h.Log(LogTypeBroadcast, LogLevelError, "Broadcast message has no data to send")
		return
	}

	var clientsToRemove []*Client
	for _, client := range clients {
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.Log(LogTypeBroadcast, LogLevelError, "PANIC RECOVERED sending to client %s: %v\n", client.ID, r)
					clientsToRemove = append(clientsToRemove, client)
				}
			}()

			select {
			case client.MessageChan <- message:
				// success
			default:
				h.Log(LogTypeBroadcast, LogLevelDebug, "Client %s channel full/closed, marking for removal", client.ID)
				clientsToRemove = append(clientsToRemove, client)
			}
		}()
	}

	if len(clientsToRemove) > 0 {
		safeGoroutine("RemoveProblematicClients", func() {
			for _, client := range clientsToRemove {
				h.RemoveClient(client)
			}
		})
	}
}

func (h *Hub) safeCloseClientChannel(client *Client) {
	defer func() {
		if r := recover(); r != nil {
			h.Log(LogTypeClient, LogLevelError, "PANIC RECOVERED closing channel for client %s: %v\n", client.ID, r)
		}
	}()

	if client.MessageChan != nil {
		select {
		case <-client.MessageChan:
			// already closed
		default:
			func() {
				defer func() {
					_ = recover()
				}()
				close(client.MessageChan)
			}()
		}
	}
}

func (h *Hub) safeCloseChannel(ch interface{}) {
	defer func() {
		if r := recover(); r != nil {
			h.Log(LogTypeError, LogLevelError, "PANIC RECOVERED closing channel: %v\n", r)
		}
	}()

	switch c := ch.(type) {
	case chan *Client:
		if c != nil {
			close(c)
		}
	case chan *Message:
		if c != nil {
			close(c)
		}
	}
}
