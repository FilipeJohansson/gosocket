package gosocket

import (
	"encoding/json"
	"fmt"
)

type Hub struct {
	Clients    map[*Client]bool
	Rooms      map[string]map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
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

	for {
		select {
		case client := <-h.Register:
			h.Clients[client] = true
			fmt.Printf("Client registered: %s (total: %d)\n", client.ID, len(h.Clients))

		case client := <-h.Unregister:
			if _, exists := h.Clients[client]; exists {
				delete(h.Clients, client)
				close(client.MessageChan)

				h.removeClientFromAllRooms(client)

				fmt.Printf("Client unregistered: %s (total: %d)\n", client.ID, len(h.Clients))
			}

		case message := <-h.Broadcast:
			h.broadcastToClients(message, h.Clients)
		}
	}
}

func (h *Hub) Stop() {
	fmt.Println("Hub stopping...")

	for client := range h.Clients {
		close(client.MessageChan)
	}

	h.Clients = make(map[*Client]bool)
	h.Rooms = make(map[string]map[*Client]bool)

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
	if roomClients, exists := h.Rooms[room]; exists {
		h.broadcastToClients(message, roomClients)
		fmt.Printf("Message broadcasted to room %s (%d clients)\n", room, len(roomClients))
	}
}

func (h *Hub) JoinRoom(client *Client, room string) {
	if h.Rooms[room] == nil {
		h.Rooms[room] = make(map[*Client]bool)
		fmt.Printf("Room created: %s\n", room)
	}

	h.Rooms[room][client] = true
	fmt.Printf("Client %s joined room: %s\n", client.ID, room)
}

func (h *Hub) LeaveRoom(client *Client, room string) {
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
	var clients []*Client
	if roomClients, exists := h.Rooms[room]; exists {
		for client := range roomClients {
			clients = append(clients, client)
		}
	}
	return clients
}

func (h *Hub) GetStats() map[string]interface{} {
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

func (h *Hub) removeClientFromAllRooms(client *Client) {
	for roomName := range h.Rooms {
		h.LeaveRoom(client, roomName)
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

	for client := range clients {
		select {
		case client.MessageChan <- data:
			// success
		default:
			// client channel full or closed? remove him
			fmt.Printf("Client %s channel full/closed, removing\n", client.ID)
			h.RemoveClient(client)
		}
	}
}
