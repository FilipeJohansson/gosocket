// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

type Room struct {
	OwnerId string
	Name    string
	Clients *SharedCollection[*Client]
}

func NewRoom(ownerId, name string) *Room {
	return &Room{
		OwnerId: ownerId,
		Name:    name,
		Clients: NewSharedCollection[*Client](),
	}
}
