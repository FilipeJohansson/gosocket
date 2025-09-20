//go:build example

package main

import (
	"log"

	"github.com/FilipeJohansson/gosocket"
)

var levels = map[gosocket.LogType]gosocket.LogLevel{
	gosocket.LogTypeConnection: gosocket.LogLevelInfo,
	gosocket.LogTypeMessage:    gosocket.LogLevelDebug,
	gosocket.LogTypeError:      gosocket.LogLevelError,
	gosocket.LogTypeServer:     gosocket.LogLevelInfo,
	gosocket.LogTypeClient:     gosocket.LogLevelDebug,
}

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8080),
		gosocket.WithPath("/ws"),
		gosocket.WithLogger(&gosocket.DefaultLogger{}, levels),
		gosocket.OnConnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
			return nil
		}),
		gosocket.OnMessage(func(client *gosocket.Client, message *gosocket.Message, ctx *gosocket.Context) error {
			client.Send(message.RawData)
			return nil
		}),
		gosocket.OnDisconnect(func(client *gosocket.Client, ctx *gosocket.Context) error {
			return nil
		}),
	)

	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(ws.Start())
}
