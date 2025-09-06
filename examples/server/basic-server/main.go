//go:build example

package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws := gosocket.NewServer()

	ws.WithPort(8080).
		WithPath("/ws").
		OnConnect(func(client *gosocket.Client) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}).
		OnMessage(func(client *gosocket.Client, message *gosocket.Message) error {
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			client.Send(message.RawData)
			return nil
		}).
		OnDisconnect(func(client *gosocket.Client) error {
			fmt.Printf("Client disconnected: %s\n", client.ID)
			return nil
		})

	log.Fatal(ws.Start())
}
