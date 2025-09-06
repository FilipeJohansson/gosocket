// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"encoding/json"
	"fmt"
	"sync"
)

// IHub is an interface for a hub that manages client connections and rooms.
//
// It provides methods for running and stopping the hub, managing clients and rooms,
// broadcasting messages, and retrieving statistics and client information.
type IHub interface {
	// Run starts the hub.
	Run()

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
}

type Hub struct {
	Clients    map[*Client]bool
	Rooms      map[string]map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
	mu         sync.RWMutex
	running    bool
}

// NewHub creates a new Hub instance. It returns a pointer to a Hub struct.
//
// The created Hub instance will have empty maps for the clients and rooms.
// The Register, Unregister and Broadcast channels will be created with a default buffer size.
func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Rooms:      make(map[string]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan *Message),
	}
}

// Run starts the hub. It is a blocking call and will run until the Stop method is called.
//
// It handles the following events:
// - Register: adds a client to the hub and broadcasts the client to all other clients
// - Unregister: removes a client from the hub, closes the client's message channel and removes the client from all rooms
// - Broadcast: broadcasts a message to all clients in the hub
func (h *Hub) Run() {
	fmt.Println("Hub started")
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()

	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			h.mu.Unlock()
			fmt.Printf("Client registered: %s (total: %d)\n", client.ID, len(h.Clients))

		case client := <-h.Unregister:
			h.mu.Lock()
			if _, exists := h.Clients[client]; exists {
				delete(h.Clients, client)
				close(client.MessageChan)

				h.removeClientFromAllRoomsUnsafe(client)

				fmt.Printf("Client unregistered: %s (total: %d)\n", client.ID, len(h.Clients))
			}
			h.mu.Unlock()

		case message := <-h.Broadcast:
			h.mu.RLock()
			h.broadcastToClients(message, h.Clients)
			h.mu.RUnlock()
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
	fmt.Println("Hub stopping...")

	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false

	for client := range h.Clients {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Recovered from panic closing client channel: %v\n", r)
				}
			}()
			if client.MessageChan != nil {
				close(client.MessageChan)
			}
		}()
	}

	h.Clients = make(map[*Client]bool)
	h.Rooms = make(map[string]map[*Client]bool)
	h.mu.Unlock()

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic in Stop(): %v\n", r)
			}
		}()
		if h.Register != nil {
			close(h.Register)
		}
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic in Stop(): %v\n", r)
			}
		}()
		if h.Unregister != nil {
			close(h.Unregister)
		}
	}()

	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic in Stop(): %v\n", r)
			}
		}()
		if h.Broadcast != nil {
			close(h.Broadcast)
		}
	}()

	fmt.Println("Hub stopped")
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
		fmt.Printf("Message broadcasted to room %s (%d clients)\n", room, len(clientsCopy))
	}
}

// CreateRoom creates a new room with the given name. If the room already exists, the method
// will return nil. If the name is empty, an error is returned.
//
// This method is safe to call concurrently.
func (h *Hub) CreateRoom(name string) error {
	if name == "" {
		return fmt.Errorf("room name cannot be empty")
	}

	h.mu.Lock()
	if h.Rooms[name] == nil {
		h.Rooms[name] = make(map[*Client]bool)
		fmt.Printf("Room created: %s\n", name)
	}
	h.mu.Unlock()

	return nil
}

// JoinRoom adds a client to a room. If the room does not exist, it is created.
//
// This method is safe to call concurrently.
func (h *Hub) JoinRoom(client *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.Rooms[room] == nil {
		h.Rooms[room] = make(map[*Client]bool)
		fmt.Printf("Room created: %s\n", room)
	}

	h.Rooms[room][client] = true
	fmt.Printf("Client %s joined room: %s\n", client.ID, room)
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
			fmt.Printf("Client %s left room: %s\n", client.ID, room)

			// remove room if empty
			if len(roomClients) == 0 {
				delete(h.Rooms, room)
				fmt.Printf("Room deleted (empty): %s\n", room)
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
		fmt.Printf("Room deleted: %s\n", name)
		return nil
	}

	return fmt.Errorf("room not found: %s", name)
}

// IsRunning returns a boolean indicating whether the hub is currently running.
//
// This method is safe to call concurrently.
func (h *Hub) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

// removeClientFromAllRooms removes the given client from all rooms it is currently in.
// It is used when a client is removed from the hub, to ensure that it is removed from
// all rooms it is currently in. This prevents the client from receiving messages from
// rooms it is no longer in.
//
// This method is safe to call concurrently, as it takes a read lock on the hub's rooms
// map and then calls LeaveRoom on each room the client is in, which also takes a read
// lock on the room's clients map.
func (h *Hub) removeClientFromAllRooms(client *Client) {
	for roomName := range h.Rooms {
		h.LeaveRoom(client, roomName)
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
			fmt.Printf("Client %s left room: %s\n", client.ID, roomName)

			// remove room if empty
			if len(roomClients) == 0 {
				delete(h.Rooms, roomName)
				fmt.Printf("Room deleted (empty): %s\n", roomName)
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
	data := message.RawData
	if data == nil && message.Data != nil {
		// TODO: use the right serializer based on Encoding
		if jsonData, err := json.Marshal(message.Data); err == nil {
			data = jsonData
		}
	}

	var clientsToRemove []*Client
	for client := range clients {
		select {
		case client.MessageChan <- data:
			// success
		default:
			// client channel full or closed? remove him
			fmt.Printf("Client %s channel full/closed, removing\n", client.ID)
			clientsToRemove = append(clientsToRemove, client)
		}
	}

	for _, client := range clientsToRemove {
		h.RemoveClient(client)
	}
}
