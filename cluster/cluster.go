// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package cluster

// Cluster package provides a minimal cluster/event interface used by the hub
// to replicate messages across nodes. The types here avoid importing the
// gosocket package to prevent circular imports; events carry raw bytes and
// simple metadata.

type EventType int

const (
	EventBroadcastAll EventType = iota
	EventBroadcastRoom
	EventSendToClient
)

// ClusterEvent represents a message/event that is replicated across cluster
// nodes. The Raw field contains the serialized gosocket event.
type ClusterEvent struct {
	Type         EventType
	Raw          []byte
	Room         string
	To           string
	From         string
	Encoding     int
	OriginNodeID string
}

// ClusterManager is the minimal interface the hub needs to publish/receive
// cluster events. Implementations may be a no-op (single-node) or a real
// distributed backend.
type ClusterManager interface {
	RegisterNode(nodeID string, ch chan<- *ClusterEvent) error
	UnregisterNode(nodeID string)
	PublishEvent(evt *ClusterEvent)
}

// NewNoopManager returns a ClusterManager that does nothing. Useful as a
// default so the hub can call into a cluster manager without checking for nil.
func NewNoopManager() ClusterManager { return &noopManager{} }

type noopManager struct{}

func (n *noopManager) RegisterNode(nodeID string, ch chan<- *ClusterEvent) error { return nil }
func (n *noopManager) UnregisterNode(nodeID string)                              {}
func (n *noopManager) PublishEvent(evt *ClusterEvent)                            {}
