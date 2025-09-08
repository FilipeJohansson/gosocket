//go:build example

package main

import (
	"fmt"
	"net/http"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
)

func main() {
	handler := handler.NewHandler(
		handler.OnConnect(func(client *gosocket.Client, ctx *handler.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}),
		handler.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *handler.HandlerContext) error {
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			client.Send(message.RawData)
			return nil
		}),
		handler.OnDisconnect(func(client *gosocket.Client, ctx *handler.HandlerContext) error {
			fmt.Printf("Client disconnected: %s\n", client.ID)
			return nil
		}),
	)

	http.Handle("/ws", handler)
	http.ListenAndServe(":8080", nil)
}
