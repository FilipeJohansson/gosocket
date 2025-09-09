//go:build example

package main

import (
	"net/http"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
	"github.com/gin-gonic/gin"
)

var wsHandler *handler.Handler

func main() {
	// Create GoSocket handler
	wsHandler = handler.New(
		handler.OnConnect(func(c *gosocket.Client, ctx *handler.HandlerContext) error {
			return c.SendJSON(map[string]string{"status": "connected"})
		}),
	)

	r := gin.Default()

	// WebSocket endpoint
	r.GET("/ws", gin.WrapH(wsHandler))

	// HTTP API to send messages to WebSocket clients
	r.POST("/api/broadcast", func(c *gin.Context) {
		var data map[string]interface{}
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}

		// Broadcast to all WebSocket clients
		wsHandler.Hub().BroadcastMessage(gosocket.NewMessage(
			gosocket.TextMessage, data,
		))

		c.JSON(http.StatusOK, gin.H{"status": "broadcasted"})
	})

	// Get connected clients count
	r.GET("/api/clients", func(c *gin.Context) {
		clients := wsHandler.Hub().GetClients()
		c.JSON(http.StatusOK, gin.H{"count": len(clients)})
	})

	r.Run(":8080")
}
