//go:build example

package main

import (
	"net/http"

	"github.com/FilipeJohansson/gosocket"
	"github.com/gin-gonic/gin"
)

func main() {
	// Create GoSocket handler
	ws, err := gosocket.NewHandler(
		gosocket.WithLogger(gosocket.DefaultLoggerConfig()),
		gosocket.OnConnect(func(d gosocket.Dispatcher, ctx *gosocket.Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}

			return d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(
				gosocket.TextMessage,
				map[string]string{"status": "connected"},
				gosocket.JSON,
			))
		}),
	)

	if err != nil {
		panic(err)
	}

	r := gin.Default()

	// WebSocket endpoint
	r.GET("/ws", gin.WrapH(ws))

	// HTTP API to send messages to WebSocket clients
	r.POST("/api/broadcast", func(c *gin.Context) {
		var data map[string]interface{}
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}

		// Broadcast to all WebSocket clients
		ws.Dispatcher().Broadcast(gosocket.NewMessage(
			gosocket.TextMessage, data,
		))

		c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
	})

	// Get connected clients count
	r.GET("/api/clients", func(c *gin.Context) {
		clients := ws.GetClients()
		c.JSON(http.StatusOK, gin.H{"count": len(clients)})
	})

	r.Run(":8080")
}
