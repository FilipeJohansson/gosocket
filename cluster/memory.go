// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package cluster

import "sync"

type memoryManager struct {
	mu    sync.RWMutex
	nodes map[string]chan<- *ClusterEvent
}

// NewMemoryManager returns a simple in-memory cluster manager useful for
// testing and single-process simulations. It will forward published events to
// registered node channels (non-blocking, best-effort).
func NewMemoryManager() ClusterManager {
	return &memoryManager{nodes: make(map[string]chan<- *ClusterEvent)}
}

func (m *memoryManager) RegisterNode(nodeID string, ch chan<- *ClusterEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nodes[nodeID] = ch
	return nil
}

func (m *memoryManager) UnregisterNode(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.nodes, nodeID)
}

func (m *memoryManager) PublishEvent(evt *ClusterEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, ch := range m.nodes {
		if evt != nil && evt.OriginNodeID != "" && evt.OriginNodeID == id {
			continue
		}
		// Best-effort delivery: non-blocking send
		select {
		case ch <- evt:
		default:
		}
	}
}
