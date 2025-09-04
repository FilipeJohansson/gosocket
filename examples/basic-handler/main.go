package main

import (
	"fmt"
	"net/http"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	handler := gosocket.NewHandler()

	handler.
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

	http.Handle("/ws", handler)
	http.ListenAndServe(":8080", nil)
}
