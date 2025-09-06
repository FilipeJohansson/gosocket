package gosocket

import (
	"encoding/json"
	"fmt"
	"sync"
)

type IHub interface {
	Run()
	Stop()
	AddClient(client *Client)
	RemoveClient(client *Client)
	BroadcastMessage(message *Message)
	BroadcastToRoom(room string, message *Message)
	CreateRoom(name string) error
	JoinRoom(client *Client, room string)
	LeaveRoom(client *Client, room string)
	GetRoomClients(room string) []*Client
	GetStats() map[string]interface{}
	GetClients() map[*Client]bool
	GetRooms() map[string]map[*Client]bool
	DeleteRoom(name string) error
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

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*Client]bool),
		Rooms:      make(map[string]map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan *Message),
	}
}

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

func (h *Hub) AddClient(client *Client) {
	h.Register <- client
}

func (h *Hub) RemoveClient(client *Client) {
	h.Unregister <- client
}

func (h *Hub) BroadcastMessage(message *Message) {
	h.Broadcast <- message
}

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

func (h *Hub) GetClients() map[*Client]bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clientsCopy := make(map[*Client]bool)
	for client, value := range h.Clients {
		clientsCopy[client] = value
	}
	return clientsCopy
}

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

func (h *Hub) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.running
}

func (h *Hub) removeClientFromAllRooms(client *Client) {
	for roomName := range h.Rooms {
		h.LeaveRoom(client, roomName)
	}
}

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
