// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

type Room struct {
	ownerId string
	name    string
	clients *SharedCollection[*Client]
}

func NewRoom(ownerId, name string) *Room {
	return &Room{
		ownerId: ownerId,
		name:    name,
		clients: NewSharedCollection[*Client](),
	}
}

func (r *Room) Name() string {
	return r.name
}

func (r *Room) OwnerId() string {
	return r.ownerId
}

func (r *Room) Clients() map[uint64]*Client {
	return r.clients.GetAll()
}

func (r *Room) AddClient(client *Client) {
	r.clients.AddWithStringId(client, client.ID)
}

func (r *Room) RemoveClient(id string) bool {
	return r.clients.RemoveByStringId(id)
}

func (r *Room) GetClient(id string) (*Client, bool) {
	return r.clients.GetByStringId(id)
}
