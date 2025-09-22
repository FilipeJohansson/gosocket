// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

type Room struct {
	id      string
	ownerId string
	name    string
	clients *SharedCollection[*Client, string]
}

func NewRoom(id, ownerId, name string) *Room {
	return &Room{
		id:      id,
		ownerId: ownerId,
		name:    name,
		clients: NewSharedCollection[*Client, string](),
	}
}

func (r *Room) ID() string {
	return r.id
}

func (r *Room) Name() string {
	return r.name
}

func (r *Room) OwnerId() string {
	return r.ownerId
}

func (r *Room) Clients() map[string]*Client {
	return r.clients.GetAll()
}

func (r *Room) AddClient(client *Client) {
	r.clients.Add(client, client.ID)
}

func (r *Room) RemoveClient(id string) bool {
	return r.clients.Remove(id)
}

func (r *Room) GetClient(id string) (*Client, bool) {
	return r.clients.Get(id)
}
