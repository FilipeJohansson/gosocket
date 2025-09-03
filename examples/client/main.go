package main

import (
	"fmt"
	"log"

	"github.com/gorilla/websocket"
)

func main() {
	fmt.Println("Starting...")
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Send a message
	err = conn.WriteMessage(websocket.TextMessage, []byte("Hello from Go client!"))
	if err != nil {
		log.Fatal(err)
	}

	// Read reply
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Received: %s\n", message)
}
