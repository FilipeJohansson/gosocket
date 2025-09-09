//go:build example

package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
	"github.com/FilipeJohansson/gosocket/server"
)

func main() {
	ws := server.New(
		server.WithPort(8080),
		server.WithPath("/ws"),
		server.OnConnect(func(client *gosocket.Client, ctx *handler.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", client.ID)
			return nil
		}),
		server.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *handler.HandlerContext) error {
			fmt.Printf("Received: %s\n", string(message.RawData))
			// Echo back
			client.Send(message.RawData)
			return nil
		}),
		server.OnDisconnect(func(client *gosocket.Client, ctx *handler.HandlerContext) error {
			fmt.Printf("Client disconnected: %s\n", client.ID)
			return nil
		}),
	)

	log.Fatal(ws.Start())
}
