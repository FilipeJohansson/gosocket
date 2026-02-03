//go:build example

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/cluster"
)

func main() {
	// Example: use in-memory cluster manager to simulate multiple nodes.
	mem := cluster.NewMemoryManager()

	// create handler/server with a simple broadcast handler
	server, err := gosocket.NewServer(
		gosocket.WithPath("/ws"),
		// Configure clustering via server options
		gosocket.WithCluster(gosocket.ClusterConfig{Manager: mem}),
		gosocket.OnMessage(func(c *gosocket.Client, m *gosocket.Message, ctx *gosocket.Context) error {
			// simply broadcast incoming messages to all local+clustered nodes
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// Or configure clustering after server creation
	// server, err = server.With(gosocket.WithCluster(mem))
	// if err != nil {
	// 	log.Fatalf("failed to configure cluster: %v", err)
	// }

	// Start HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.Handler().HandleWebSocket)

	addr := ":8080"
	fmt.Printf("Starting example cluster server on %s (ws://localhost:8080/ws)\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
