//go:build example

package main

import (
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	// Example: use in-memory cluster manager to simulate multiple nodes.
	mem := gosocket.NewTestMemoryManager()
	state := gosocket.NewTestMemoryStateStore()

	// Create handler/server with a simple broadcast handler
	s1, err := gosocket.NewServer(
		gosocket.WithPath("/ws"),
		gosocket.WithPort(8080),
		//* Configure clustering via server options
		gosocket.WithCluster(gosocket.ClusterConfig{
			Manager: mem,
			State:   state,
			NodeID:  "node-1",
		}),
		gosocket.OnMessage(func(m *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			// Simply broadcast incoming messages to all local+clustered nodes
			d.Broadcast(m)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	s2, err := gosocket.NewServer(
		gosocket.WithPath("/ws2"),
		gosocket.WithPort(8081),
		gosocket.OnMessage(func(m *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			// simply broadcast incoming messages to all local+clustered nodes
			d.Broadcast(m)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	//* Or configure clustering after server creation
	s2, err = s2.With(gosocket.WithCluster(gosocket.ClusterConfig{
		Manager: mem,
		State:   state,
		NodeID:  "node-2",
	}))
	if err != nil {
		log.Fatalf("failed to configure cluster: %v", err)
	}

	// Start servers in a goroutine
	go func() {
		if err := s1.Start(); err != nil {
			log.Fatalf("failed to start server 1: %v", err)
		}
	}()
	go func() {
		if err := s2.Start(); err != nil {
			log.Fatalf("failed to start server 2: %v", err)
		}
	}()

	select {}
}
