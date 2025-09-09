//go:build example

package main

import (
	"fmt"
	"net/http"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws, err := gosocket.NewHandler(
		gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}),
		gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.HandlerContext) error {
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			client.Send(message.RawData)
			return nil
		}),
		gosocket.OnDisconnect(func(client *gosocket.Client, ctx *gosocket.HandlerContext) error {
			fmt.Printf("Client disconnected: %s\n", client.ID)
			return nil
		}),
	)

	if err != nil {
		panic(err)
	}

	http.Handle("/ws", ws)
	http.ListenAndServe(":8080", nil)
}
