// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package cluster

import "github.com/FilipeJohansson/gosocket/internal/message"

/**
 * event.go defines the ClusterEvent data model used for communication
 * between distributed GoSocket nodes.
 *
 * It describes the minimal, serializable representation of events that
 * must be propagated across the cluster.
 *
 * This file should remain stable to ensure compatibility between nodes.
 *
 * MUST NOT contain execution logic, routing logic, or cluster backend
 * implementations.
 */

type EventType int

const (
	EventSendToClient EventType = iota
	EventBroadcast
	EventBroadcastToRoom
	EventCreateRoom
	EventDeleteRoom
	EventJoinRoom
	EventLeaveRoom
)

type Event struct {
	Type         EventType
	MsgType      message.MessageType
	Origin       string
	TargetNode   string
	ToClientID   string
	FromClientID string
	RoomName     string
	Payload      []byte
	Encoding     int
}
