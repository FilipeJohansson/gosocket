//go:build example

package main

import (
	"fmt"
	"net/http"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws, err := gosocket.NewHandler(
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Client connected: %s\n", client.GetID())
			return nil
		}),
		gosocket.OnMessage(func(message *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			d.SendToClient(client.GetID(), message)
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
		panic(err)
	}

	http.Handle("/ws", ws)
	http.ListenAndServe(":8080", nil)
}
