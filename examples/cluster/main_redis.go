//go:build example

package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	redisNode1Name := "test-redis-node-1"
	redisNode2Name := "test-redis-node-2"

	createRedisManager := func(nodeID string) gosocket.ClusterManager {
		m, err := gosocket.NewTestRedisManager("127.0.0.1:6379", "test-topic", nodeID)
		if err != nil {
			log.Fatalf("failed to create RedisManager: %v", err)
		}
		return m
	}
	m1 := createRedisManager(redisNode1Name)
	m2 := createRedisManager(redisNode2Name)

	// Create handler/server with a simple broadcast handler
	s1, err := gosocket.NewServer(
		gosocket.WithPath("/ws"),
		gosocket.WithPort(8080),
		//* Configure clustering via server options
		gosocket.WithCluster(gosocket.ClusterConfig{Manager: m1, NodeID: redisNode1Name}),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Println("S1 OnConnect: ", client.GetID())
			return nil
		}),
		gosocket.OnMessage(func(m *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("S1 OnMessage message: %s\n", string(m.RawData))
			if string(m.RawData) == "secret-message" {
				d.SendToClient(client.GetID(), m)
				return nil
			}
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
		gosocket.WithCluster(gosocket.ClusterConfig{Manager: m2, NodeID: redisNode2Name}),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Println("S2 OnConnect: ", client.GetID())
			return nil
		}),
		gosocket.OnMessage(func(m *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			fmt.Printf("S2 OnMessage message: %s\n", string(m.RawData))
			// simply broadcast incoming messages to all local+clustered nodes
			d.Broadcast(m)
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
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
