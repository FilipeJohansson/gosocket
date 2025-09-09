//go:build example

package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket"
	"github.com/FilipeJohansson/gosocket/handler"
	"github.com/FilipeJohansson/gosocket/server"
)

type NotificationMessageData struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon"`
}

type NotificationMessage struct {
	Type string                  `json:"type"`
	Data NotificationMessageData `json:"data"`
}

func main() {
	srv := server.New(
		server.WithPort(8081),
		server.WithPath("/ws"),
		server.WithJSONSerializer(),
		server.OnConnect(func(c *gosocket.Client, hc *handler.HandlerContext) error {
			fmt.Printf("Client connected: %s\n", c.ID)
			return nil
		}),
		server.OnDisconnect(func(c *gosocket.Client, hc *handler.HandlerContext) error {
			fmt.Printf("Client disconnected: %s\n", c.ID)
			return nil
		}),
	)

	// Serve static files for the notifications client
	http.HandleFunc("/", serveHome)

	fmt.Println("Notifications server starting...")
	fmt.Println("Open http://localhost:8080 to access the client")

	go func() {
		// Send periodic notifications to all connected clients
		for {
			notificationMessage := NotificationMessage{
				Type: "push",
				Data: NotificationMessageData{
					Title: "Hello from GoSocket!",
					Body:  "See how simple it is to send push notifications to your users!",
					Icon:  "https://placehold.co/10",
				},
			}
			srv.BroadcastJSON(notificationMessage)
			time.Sleep(10 * time.Second)
		}
	}()

	// Start GoSocket server
	go func() {
		fmt.Println("Starting GoSocket server...")
		if err := srv.Start(); err != nil {
			log.Fatal("Failed to start WebSocket server:", err)
		}
	}()

	// Start HTTP server for serving the client page
	fmt.Println("Starting HTTP server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Push Notifications Demo</title>
</head>
<body>
	<h1>Push Notifications Demo</h1>
	<script>
		let ws;

		function connect() {
            ws = new WebSocket('ws://localhost:8081/ws');
            
            ws.onopen = () => {
				console.log("Connected to WebSocket server");
			};
            
            ws.onmessage = (event) => {
				const data = JSON.parse(event.data);
				if(data.type === "push") {
					if('Notification' in window) {
						new Notification(data.data.title, {
							body: data.data.body,
							icon: data.data.icon
						});
					}
				}
			};
        }

		if ('Notification' in window) {
			Notification.requestPermission().then(permission => {
				if(permission === "granted") {
					connect();
				} else {
					console.warn("User denied notifications");
				}
			});
		} else {
			connect();
		}
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
