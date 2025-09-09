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
		gosocket.OnConnect(func(c *gosocket.Client, ctx *gosocket.HandlerContext) error {
			return c.SendJSON(map[string]string{"status": "connected"})
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
		ws.Hub().BroadcastMessage(gosocket.NewMessage(
			gosocket.TextMessage, data,
		))

		c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
	})

	// Get connected clients count
	r.GET("/api/clients", func(c *gin.Context) {
		clients := ws.Hub().GetClients()
		c.JSON(http.StatusOK, gin.H{"count": len(clients)})
	})

	r.Run(":8080")
}
