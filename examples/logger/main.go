//go:build example

package main

import (
	"log"

	"github.com/FilipeJohansson/gosocket"
)

var levels = map[gosocket.LogType]gosocket.LogLevel{
	gosocket.LogTypeServer:     gosocket.LogLevelInfo,
	gosocket.LogTypeClient:     gosocket.LogLevelDebug,
	gosocket.LogTypeConnection: gosocket.LogLevelInfo,
	gosocket.LogTypeMessage:    gosocket.LogLevelDebug,
	gosocket.LogTypeError:      gosocket.LogLevelError,
}

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8080),
		gosocket.WithPath("/ws"),
		gosocket.WithLogger(&gosocket.DefaultLogger{}, levels),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			return nil
		}),
		gosocket.OnMessage(func(message *gosocket.Message, d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			d.SendToClient(client.GetID(), message)
			return nil
		}),
		gosocket.OnDisconnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			return nil
		}),
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(ws.Start())
}
