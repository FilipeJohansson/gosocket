// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package backends

import (
	"context"
	"sync"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
)

type MemorySubscription struct {
	ch <-chan *cluster.Event
}

func (s *MemorySubscription) Events() <-chan *cluster.Event {
	return s.ch
}

func (s *MemorySubscription) Close() error {
	return nil
}

type MemoryManager struct {
	nodes map[string]chan<- *cluster.Event
	mu    sync.RWMutex
}

// NewTestMemoryManager returns a simple in-memory cluster manager useful for
// testing and single-process simulations. It will forward published events to
// registered node channels (non-blocking, best-effort).
func NewTestMemoryManager() cluster.Manager {
	return &MemoryManager{nodes: make(map[string]chan<- *cluster.Event)}
}

func (m *MemoryManager) Subscribe(ctx context.Context, nodeID string) (cluster.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan *cluster.Event)
	m.nodes[nodeID] = ch
	return &MemorySubscription{ch: ch}, nil
}

func (m *MemoryManager) Publish(evt *cluster.Event) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.nodes {
		// Best-effort delivery: non-blocking send
		select {
		case ch <- evt:
		default:
		}
	}
	return nil
}
