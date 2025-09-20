// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"sync"
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

	// BroadcastMessage sends a message to all connected clients.
	BroadcastMessage(message *Message)

	// BroadcastToRoom sends a message to all clients in a specific room.
	BroadcastToRoom(room string, message *Message)

	// CreateRoom creates a new room with the given name.
	CreateRoom(name string) error

	// JoinRoom adds a client to a room.
	JoinRoom(client *Client, room string)

	// LeaveRoom removes a client from a room.
	LeaveRoom(client *Client, room string)

	// GetRoomClients returns a list of clients in a room.
	GetRoomClients(room string) []*Client

	// GetStats returns statistics about the hub.
	GetStats() map[string]interface{}

	// GetClients returns a map of all connected clients.
	GetClients() map[*Client]bool

	// GetRooms returns a map of all rooms and their clients.
	GetRooms() map[string]map[*Client]bool

	// DeleteRoom deletes a room with the given name.
	DeleteRoom(name string) error

	// IsRunning returns whether the hub is currently running.
	IsRunning() bool

	Log(logType LogType, level LogLevel, msg string, args ...interface{})
}

type Hub struct {
	Clients    map[*Client]bool
	Rooms      map[string]map[*Client]bool
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
		Clients:    make(map[*Client]bool),
		Rooms:      make(map[string]map[*Client]bool),
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
				h.mu.Lock()
				h.Clients[client] = true
				h.mu.Unlock()
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
				h.mu.Lock()
				if _, exists := h.Clients[client]; exists {
					delete(h.Clients, client)
					h.safeCloseClientChannel(client)
					h.removeClientFromAllRoomsUnsafe(client)
					h.Log(LogTypeClient, LogLevelDebug, "Client unregistered: %s", client.ID)
				}
				h.mu.Unlock()
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
				h.mu.RLock()
				h.broadcastToClients(message, h.Clients)
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

	for client := range h.Clients {
		h.safeCloseClientChannel(client)
	}

	h.Clients = make(map[*Client]bool)
	h.Rooms = make(map[string]map[*Client]bool)
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
func (h *Hub) BroadcastToRoom(room string, message *Message) {
	h.mu.RLock()

	var clientsCopy map[*Client]bool
	roomClients, exists := h.Rooms[room]
	if exists {
		clientsCopy = make(map[*Client]bool)
		for client, value := range roomClients {
			clientsCopy[client] = value
		}
	}
	h.mu.RUnlock()

	if exists {
		h.broadcastToClients(message, clientsCopy)
		h.Log(LogTypeBroadcast, LogLevelDebug, "Message broadcasted to room %s (%d clients)", room, len(clientsCopy))
	}
}

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the name is empty, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) CreateRoom(name string) error {
	if name == "" {
		return ErrRoomNameEmpty
	}

	h.mu.Lock()
	if h.Rooms[name] == nil {
		h.Rooms[name] = make(map[*Client]bool)
		h.Log(LogTypeOther, LogLevelDebug, "Room created: %s", name)
	}
	h.mu.Unlock()

	return nil
}

// JoinRoom adds a client to a room. If the room does not exist, it is created.
//
// This method is safe to call concurrently.
func (h *Hub) JoinRoom(client *Client, room string) {
	if h.Rooms[room] == nil {
		err := h.CreateRoom(room)
		if err != nil {
			h.Log(LogTypeOther, LogLevelError, "Error creating room: %s", err)
			return
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.Rooms[room][client] = true
	h.Log(LogTypeOther, LogLevelDebug, "Client %s joined room: %s", client.ID, room)
}

// LeaveRoom removes the given client from the given room. If the room does not exist, or if the client is not in the room, the method does nothing.
//
// This method is safe to call concurrently.
func (h *Hub) LeaveRoom(client *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if roomClients, exists := h.Rooms[room]; exists {
		if _, clientInRoom := roomClients[client]; clientInRoom {
			delete(roomClients, client)
			h.Log(LogTypeOther, LogLevelDebug, "Client %s left room: %s", client.ID, room)

			// remove room if empty
			if len(roomClients) == 0 {
				h.Log(LogTypeOther, LogLevelDebug, "Room %s is empty, removing it", room)
				err := h.DeleteRoom(room)
				if err != nil {
					h.Log(LogTypeOther, LogLevelError, "Error deleting room: %s", err)
				}
			}
		}
	}
}

// GetRoomClients returns all clients in the specified room. If the room does not exist,
// an empty slice is returned.
//
// This method is safe to call concurrently.
func (h *Hub) GetRoomClients(room string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var clients []*Client
	if roomClients, exists := h.Rooms[room]; exists {
		for client := range roomClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetStats returns a map with the following keys:
//
// - total_clients: The total number of clients connected to the hub.
// - total_rooms: The total number of rooms in the hub.
// - rooms: A map with room names as keys and the number of clients in each room as values.
//
// This method is safe to call concurrently.
func (h *Hub) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := make(map[string]interface{})

	stats["total_clients"] = len(h.Clients)
	stats["total_rooms"] = len(h.Rooms)

	roomStats := make(map[string]int)
	for roomName, roomClients := range h.Rooms {
		roomStats[roomName] = len(roomClients)
	}
	stats["rooms"] = roomStats

	return stats
}

// GetClients returns a copy of the clients map, where the keys are the client pointers
// and the values are booleans indicating whether the client is connected to the hub.
//
// This method is safe to call concurrently.
func (h *Hub) GetClients() map[*Client]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clientsCopy := make(map[*Client]bool)
	for client, value := range h.Clients {
		clientsCopy[client] = value
	}
	return clientsCopy
}

// GetRooms returns a copy of the rooms map, where the keys are the room names and the values
// are maps with client pointers as keys and booleans indicating whether the client is connected
// to the room.
//
// This method is safe to call concurrently.
func (h *Hub) GetRooms() map[string]map[*Client]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	roomsCopy := make(map[string]map[*Client]bool)
	for roomName, roomClients := range h.Rooms {
		roomClientsCopy := make(map[*Client]bool)
		for client, value := range roomClients {
			roomClientsCopy[client] = value
		}
		roomsCopy[roomName] = roomClientsCopy
	}
	return roomsCopy
}

// DeleteRoom deletes a room with the given name. If the room does not exist, it will return
// an error. If the room exists, it will remove all clients from the room and remove the room
// from the hub.
//
// This method is safe to call concurrently.
func (h *Hub) DeleteRoom(name string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if roomClients, exists := h.Rooms[name]; exists {
		// first, remove all clients from the room
		for client := range roomClients {
			delete(roomClients, client)
		}
		delete(h.Rooms, name)
		h.Log(LogTypeOther, LogLevelDebug, "Room deleted: %s", name)
		return nil
	}

	return newRoomNotFoundError(name)
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
	for roomName, roomClients := range h.Rooms {
		if _, clientInRoom := roomClients[client]; clientInRoom {
			delete(roomClients, client)
			h.Log(LogTypeOther, LogLevelDebug, "Client %s left room: %s", client.ID, roomName)

			// remove room if empty
			if len(roomClients) == 0 {
				delete(h.Rooms, roomName)
				h.Log(LogTypeOther, LogLevelDebug, "Room %s is empty, removing it", roomName)
			}
		}
	}
}

// broadcastToClients broadcasts the given message to all clients in the given clients map.
//
// If a client's message channel is full or closed, it is removed from the hub.
//
// This method is safe to call concurrently, as it takes a read lock on the hub's clients map.
func (h *Hub) broadcastToClients(message *Message, clients map[*Client]bool) {
	defer func() {
		if r := recover(); r != nil {
			h.Log(LogTypeBroadcast, LogLevelError, "PANIC RECOVERED in broadcastToClients: %v\nStack trace:\n%s\n", r, string(debug.Stack()))
		}
	}()

	data := message.RawData
	if data == nil && message.Data != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.Log(LogTypeBroadcast, LogLevelError, "PANIC RECOVERED in JSON marshal during broadcast: %v\n", r)
					return
				}
			}()

			// TODO: use the right serializer based on Encoding
			if jsonData, err := json.Marshal(message.Data); err == nil {
				data = jsonData
			}
		}()
	}

	if data == nil {
		return
	}

	var clientsToRemove []*Client
	for client := range clients {
		func() {
			defer func() {
				if r := recover(); r != nil {
					h.Log(LogTypeBroadcast, LogLevelError, "PANIC RECOVERED sending to client %s: %v\n", client.ID, r)
					clientsToRemove = append(clientsToRemove, client)
				}
			}()

			select {
			case client.MessageChan <- data:
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
