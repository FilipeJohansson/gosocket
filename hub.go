package gosocket

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
}

func (h *Hub) Stop() {
}

func (h *Hub) AddClient(client *Client) {
}

func (h *Hub) RemoveClient(client *Client) {
}

func (h *Hub) BroadcastMessage(message *Message) {
}

func (h *Hub) JoinRoom(client *Client, room string) {
}

func (h *Hub) LeaveRoom(client *Client, room string) {
}

func (h *Hub) GetRoomClients(room string) []*Client {
	return nil
}
