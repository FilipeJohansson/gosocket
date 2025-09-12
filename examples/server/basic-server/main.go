//go:build example

package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(-1),
		gosocket.WithPath("/ws"),
		gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}),
		gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			client.Send(message.RawData)
			return nil
		}),
		gosocket.OnDisconnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
			fmt.Printf("Client disconnected: %s\n", client.ID)
			return nil
		}),
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(ws.Start())
}
