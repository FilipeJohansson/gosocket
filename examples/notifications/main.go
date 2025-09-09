//go:build example

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/FilipeJohansson/gosocket"
)

type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Icon  string `json:"icon"`
}

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8081),
		gosocket.WithPath("/ws"),
		gosocket.OnConnect(onConnect),
		gosocket.OnJSONMessage(onMessage),
	)

	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", servePage)

	// Send periodic notifications
	go func() {
		for {
			time.Sleep(15 * time.Second)
			ws.BroadcastJSON(Message{
				Type: "notification",
				Data: Notification{
					Title: "System Alert",
					Body:  "Server is running perfectly!",
					Icon:  "https://placehold.co/50",
				},
			})
		}
	}()

	go func() {
		log.Fatal(ws.Start())
	}()

	log.Println("Notifications server at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func onConnect(client *gosocket.Client, ctx *gosocket.HandlerContext) error {
	return client.SendJSON(Message{
		Type: "notification",
		Data: Notification{
			Title: "Welcome!",
			Body:  "You're now connected to GoSocket notifications",
			Icon:  "https://placehold.co/50",
		},
	})
}

func onMessage(client *gosocket.Client, data interface{}, ctx *gosocket.HandlerContext) error {
	jsonBytes, _ := json.Marshal(data)
	var msg Message
	json.Unmarshal(jsonBytes, &msg)

	if msg.Type == "send" {
		// Broadcast user's notification to all clients
		ctx.BroadcastJSONToAll(Message{
			Type: "notification",
			Data: Notification{
				Title: "User Alert",
				Body:  msg.Data.(string),
				Icon:  "https://placehold.co/50",
			},
		})
	}

	return nil
}

func servePage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Simple Notifications</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 500px; margin: 50px auto; padding: 20px; }
        .box { padding: 20px; border: 1px solid #ccc; margin: 20px 0; }
        input { padding: 10px; width: 300px; margin-right: 10px; }
        button { padding: 10px 20px; }
        .status { background: #f0f8ff; }
        .quick-buttons button { margin: 5px; }
    </style>
</head>
<body>
    <h1>Simple Notifications</h1>
    
    <div class="box status">
        <div id="status">Connecting...</div>
    </div>
    
    <div class="box">
        <h3>Send Notification</h3>
        <input type="text" id="message" placeholder="Type your message">
        <button onclick="send()">Send</button>
    </div>
    
    <div class="box quick-buttons">
        <h3>Quick Send</h3>
        <button onclick="quickSend('Coffee break time!')">Coffee Break</button>
        <button onclick="quickSend('Meeting in 5 minutes')">Meeting Alert</button>
        <button onclick="quickSend('Lunch time!')">Lunch Time</button>
    </div>

    <script>
        const ws = new WebSocket('ws://localhost:8081/ws');
        
        // Request notification permission on load
        if ('Notification' in window && Notification.permission === 'default') {
            Notification.requestPermission();
        }
        
        ws.onopen = () => {
            document.getElementById('status').textContent = 'Connected - Ready to send/receive notifications';
        };
        
        ws.onclose = () => {
            document.getElementById('status').textContent = 'Disconnected';
        };
        
        ws.onmessage = function(event) {
            const msg = JSON.parse(event.data);
            
            if (msg.type === 'notification' && msg.data) {
                // Show browser notification
                if ('Notification' in window && Notification.permission === 'granted') {
                    new Notification(msg.data.title, {
                        body: msg.data.body,
                        icon: msg.data.icon
                    });
                }
            }
        };
        
        function send() {
            const message = document.getElementById('message').value.trim();
            if (message) {
                ws.send(JSON.stringify({
                    type: 'send',
                    data: message
                }));
                document.getElementById('message').value = '';
            }
        }
        
        function quickSend(message) {
            ws.send(JSON.stringify({
                type: 'send',
                data: message
            }));
        }
        
        // Send on Enter key
        document.getElementById('message').addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                send();
            }
        });
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
