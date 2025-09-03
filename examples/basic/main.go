package main

import (
	"fmt"
	"log"

	"github.com/FilipeJohansson/gosocket"
)

func main() {
	ws := gosocket.New()
    
    ws.WithPort(8080).
       WithPath("/ws").
       OnConnect(func(client *gosocket.Client) {
           fmt.Printf("Client connected: %s\n", client.ID)
       }).
       OnMessage(func(client *gosocket.Client, message *gosocket.Message) {
           fmt.Printf("Received: %s\n", string(message.RawData))
           // Echo back
           client.Send(message.RawData)
       }).
       OnDisconnect(func(client *gosocket.Client) {
           fmt.Printf("Client disconnected: %s\n", client.ID)
       })

    log.Fatal(ws.Start())
}
