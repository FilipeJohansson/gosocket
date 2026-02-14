// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

import (
	"time"

	"github.com/FilipeJohansson/gosocket/internal/hub"
)

type ClientDTO struct {
	ID       string                 `json:"id"`
	UserData map[string]interface{} `json:"user_data,omitempty"`
}

type RoomDTO struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	ClientCount int       `json:"client_count"`
}

func toClientDTO(client *hub.Client) ClientDTO {
	if client == nil {
		return ClientDTO{}
	}
	return ClientDTO{
		ID:       client.GetID(),
		UserData: snapshotUserData(client),
	}
}

func toClientsDTO(clients map[string]*hub.Client) map[string]ClientDTO {
	out := make(map[string]ClientDTO, len(clients))
	for id, client := range clients {
		out[id] = toClientDTO(client)
	}
	return out
}

func toRoomsDTO(rooms map[string]*hub.Room) map[string]RoomDTO {
	out := make(map[string]RoomDTO, len(rooms))
	for _, room := range rooms {
		if room == nil {
			continue
		}
		out[room.Name()] = RoomDTO{
			ID:          room.ID(),
			Name:        room.Name(),
			OwnerID:     room.OwnerId(),
			CreatedAt:   room.CreatedAt(),
			ClientCount: len(room.Clients()),
		}
	}
	return out
}

func snapshotUserData(client *hub.Client) map[string]interface{} {
	if client == nil {
		return nil
	}

	raw := client.GetUserData()
	data, ok := raw.(map[string]interface{})
	if !ok || data == nil {
		return nil
	}

	out := make(map[string]interface{}, len(data))
	for k, v := range data {
		out[k] = v
	}
	return out
}
