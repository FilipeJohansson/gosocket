//go:build example

package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8080),
		gosocket.WithPath("/ws"),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Client connected: %s\n", client.GetID())
			return nil
		}),
		gosocket.OnMessage(func(m *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Received: %s\n", string(m.RawData))
			// Echo back
			d.SendToClient(client.GetID(), gosocket.NewRawMessage(
				gosocket.TextMessage, m.RawData,
			))
			return nil
		}),
		gosocket.OnDisconnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Client disconnected: %s\n", client.GetID())
			return nil
		}),
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(ws.Start())
}
