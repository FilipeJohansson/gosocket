//go:build example

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/FilipeJohansson/gosocket"
)

// Message types for the chat
type ChatMessage struct {
	Type      string    `json:"type"`
	User      string    `json:"user,omitempty"`
	Room      string    `json:"room,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Users     []string  `json:"users,omitempty"` // for user list updates
}

const (
	MsgTypeJoin         = "join"
	MsgTypeLeave        = "leave"
	MsgTypeChat         = "chat"
	MsgTypeUserList     = "user_list"
	MsgTypeRoomList     = "room_list"
	MsgTypeNotification = "notification"
	MsgTypeError        = "error"
)

func main() {
	server := gosocket.NewServer()

	server.
		WithPort(8081).
		WithPath("/ws").
		WithJSONSerializer().
		OnConnect(handleConnect).
		OnDisconnect(handleDisconnect).
		OnJSONMessage(handleMessage)

	// Serve static files for the chat frontend
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/chat", serveChatPage)

	fmt.Println("Chat server starting...")
	fmt.Println("Open http://localhost:8080 to access the chat")

	// Start WebSocket server in a goroutine
	go func() {
		fmt.Println("Starting GoSocket server...")
		if err := server.Start(); err != nil {
			log.Fatal("Failed to start WebSocket server:", err)
		}
	}()

	// Start HTTP server for serving the chat page
	fmt.Println("Starting HTTP server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleConnect(client *gosocket.Client) error {
	fmt.Printf("Client connected: %s\n", client.ID)

	// Send welcome message
	welcome := ChatMessage{
		Type:      MsgTypeNotification,
		Message:   "Welcome to GoSocket Chat! Type /join <room> to join a room.",
		Timestamp: time.Now(),
	}

	return client.SendJSON(welcome)
}

func handleDisconnect(client *gosocket.Client) error {
	fmt.Printf("Client disconnected: %s\n", client.ID)

	// Get user data
	username := getUsernameFromClient(client)
	if username == "" {
		return nil
	}

	// Remove user from all rooms and notify
	rooms := client.GetRooms()
	for _, room := range rooms {
		client.LeaveRoom(room)

		// Notify room about user leaving
		leaveMsg := ChatMessage{
			Type:      MsgTypeLeave,
			User:      username,
			Room:      room,
			Message:   fmt.Sprintf("%s left the room", username),
			Timestamp: time.Now(),
		}

		// Broadcast to room (you'll need to implement this in your server)
		if server, ok := getServerFromClient(client); ok {
			server.BroadcastToRoomJSON(room, leaveMsg)
			broadcastUserList(server, room)
		}
	}

	return nil
}

func handleMessage(client *gosocket.Client, data interface{}) error {
	// Parse the JSON message
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var msg ChatMessage
	if err := json.Unmarshal(jsonBytes, &msg); err != nil {
		return err
	}

	msg.Timestamp = time.Now()

	switch msg.Type {
	case MsgTypeJoin:
		return handleJoinRoom(client, &msg)
	case MsgTypeLeave:
		return handleLeaveRoom(client, &msg)
	case MsgTypeChat:
		return handleChatMessage(client, &msg)
	default:
		return sendErrorMessage(client, "Unknown message type")
	}
}

func handleJoinRoom(client *gosocket.Client, msg *ChatMessage) error {
	if msg.Room == "" || msg.User == "" {
		return sendErrorMessage(client, "Room and username are required")
	}

	// Validate room name (no spaces, special chars)
	if strings.ContainsAny(msg.Room, " \t\n\r") {
		return sendErrorMessage(client, "Room name cannot contain spaces")
	}

	// Store username in client data
	client.SetUserData("username", msg.User)

	// Join the room
	if err := client.JoinRoom(msg.Room); err != nil {
		return sendErrorMessage(client, "Failed to join room: "+err.Error())
	}

	// Confirm join to user
	joinConfirm := ChatMessage{
		Type:      MsgTypeJoin,
		User:      msg.User,
		Room:      msg.Room,
		Message:   fmt.Sprintf("You joined room: %s", msg.Room),
		Timestamp: time.Now(),
	}
	client.SendJSON(joinConfirm)

	// Notify room about new user
	joinNotify := ChatMessage{
		Type:      MsgTypeNotification,
		User:      msg.User,
		Room:      msg.Room,
		Message:   fmt.Sprintf("%s joined the room", msg.User),
		Timestamp: time.Now(),
	}

	// Broadcast to room
	if server, ok := getServerFromClient(client); ok {
		server.BroadcastToRoomJSON(msg.Room, joinNotify)
		broadcastUserList(server, msg.Room)
	}

	return nil
}

func handleLeaveRoom(client *gosocket.Client, msg *ChatMessage) error {
	username := getUsernameFromClient(client)
	if username == "" {
		return sendErrorMessage(client, "You must set a username first")
	}

	if msg.Room == "" {
		return sendErrorMessage(client, "Room name is required")
	}

	// Leave the room
	if err := client.LeaveRoom(msg.Room); err != nil {
		return sendErrorMessage(client, "Failed to leave room: "+err.Error())
	}

	// Confirm leave to user
	leaveConfirm := ChatMessage{
		Type:      MsgTypeLeave,
		User:      username,
		Room:      msg.Room,
		Message:   fmt.Sprintf("You left room: %s", msg.Room),
		Timestamp: time.Now(),
	}
	client.SendJSON(leaveConfirm)

	// Notify room about user leaving
	leaveNotify := ChatMessage{
		Type:      MsgTypeNotification,
		User:      username,
		Room:      msg.Room,
		Message:   fmt.Sprintf("%s left the room", username),
		Timestamp: time.Now(),
	}

	// Broadcast to room
	if server, ok := getServerFromClient(client); ok {
		server.BroadcastToRoomJSON(msg.Room, leaveNotify)
		broadcastUserList(server, msg.Room)
	}

	return nil
}

func handleChatMessage(client *gosocket.Client, msg *ChatMessage) error {
	username := getUsernameFromClient(client)
	if username == "" {
		return sendErrorMessage(client, "You must join a room first")
	}

	if msg.Room == "" || msg.Message == "" {
		return sendErrorMessage(client, "Room and message are required")
	}

	// Check if user is in the room
	rooms := client.GetRooms()
	inRoom := false
	for _, room := range rooms {
		if room == msg.Room {
			inRoom = true
			break
		}
	}

	if !inRoom {
		return sendErrorMessage(client, "You are not in room: "+msg.Room)
	}

	// Create chat message
	chatMsg := ChatMessage{
		Type:      MsgTypeChat,
		User:      username,
		Room:      msg.Room,
		Message:   msg.Message,
		Timestamp: time.Now(),
	}

	// Broadcast to room
	if server, ok := getServerFromClient(client); ok {
		server.BroadcastToRoomJSON(msg.Room, chatMsg)
	}

	return nil
}

func broadcastUserList(server *gosocket.Server, room string) {
	clients := server.GetClientsInRoom(room)
	var users []string

	for _, client := range clients {
		if username := getUsernameFromClient(client); username != "" {
			users = append(users, username)
		}
	}

	userListMsg := ChatMessage{
		Type:      MsgTypeUserList,
		Room:      room,
		Users:     users,
		Timestamp: time.Now(),
	}

	server.BroadcastToRoomJSON(room, userListMsg)
}

func sendErrorMessage(client *gosocket.Client, message string) error {
	errMsg := ChatMessage{
		Type:      MsgTypeError,
		Message:   message,
		Timestamp: time.Now(),
	}
	return client.SendJSON(errMsg)
}

func getUsernameFromClient(client *gosocket.Client) string {
	if username := client.GetUserData("username"); username != nil {
		if str, ok := username.(string); ok {
			return str
		}
	}
	return ""
}

func getServerFromClient(client *gosocket.Client) (*gosocket.Server, bool) {
	// This is a workaround - in a real implementation, you'd want to
	// store the server reference in the client or use a global variable
	// For now, return nil as this is just an example
	return nil, false
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>GoSocket Chat</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .container { display: flex; flex-direction: column; height: 80vh; }
        .header { text-align: center; margin-bottom: 20px; }
        .chat-container { display: flex; flex: 1; gap: 20px; }
        .sidebar { width: 200px; border: 1px solid #ccc; padding: 10px; }
        .main-chat { flex: 1; display: flex; flex-direction: column; border: 1px solid #ccc; }
        .messages { flex: 1; padding: 10px; overflow-y: auto; border-bottom: 1px solid #eee; }
        .input-area { padding: 10px; display: flex; gap: 10px; }
        .message { margin: 5px 0; padding: 5px; }
        .message.chat { background: #f0f8ff; }
        .message.notification { background: #fff8dc; font-style: italic; }
        .message.error { background: #ffe4e1; color: red; }
        .user-list { list-style: none; padding: 0; }
        .user-list li { padding: 5px; border-bottom: 1px solid #eee; }
        input[type="text"] { flex: 1; padding: 8px; }
        button { padding: 8px 16px; }
        .room-controls { margin-bottom: 10px; }
        .room-controls input { width: 100px; margin-right: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>GoSocket Chat Room</h1>
        </div>
        
        <div class="chat-container">
            <div class="sidebar">
                <div class="room-controls">
                    <input type="text" id="username" placeholder="Username" />
                    <button onclick="setUsername()">Set</button>
                </div>
                
                <div class="room-controls">
                    <input type="text" id="roomName" placeholder="Room name" />
                    <button onclick="joinRoom()">Join</button>
                    <button onclick="leaveRoom()">Leave</button>
                </div>
                
                <h4>Users in Room:</h4>
                <ul class="user-list" id="userList"></ul>
            </div>
            
            <div class="main-chat">
                <div class="messages" id="messages"></div>
                <div class="input-area">
                    <input type="text" id="messageInput" placeholder="Type your message..." 
                           onkeypress="if(event.key==='Enter') sendMessage()" />
                    <button onclick="sendMessage()">Send</button>
                </div>
            </div>
        </div>
    </div>

    <script>
        let ws;
        let currentRoom = '';
        let currentUsername = '';
        
        function connect() {
            ws = new WebSocket('ws://localhost:8081/ws');
            
            ws.onopen = function() {
                addMessage('system', 'Connected to chat server');
            };
            
            ws.onmessage = function(event) {
                const msg = JSON.parse(event.data);
                handleMessage(msg);
            };
            
            ws.onclose = function() {
                addMessage('system', 'Connection closed');
                setTimeout(connect, 3000); // Reconnect after 3 seconds
            };
            
            ws.onerror = function(error) {
                addMessage('error', 'Connection error');
            };
        }
        
        function handleMessage(msg) {
            switch(msg.type) {
                case 'chat':
                    addMessage('chat', msg.user + ': ' + msg.message, msg.timestamp);
                    break;
                case 'notification':
                    addMessage('notification', msg.message, msg.timestamp);
                    break;
                case 'join':
                case 'leave':
                    addMessage('notification', msg.message, msg.timestamp);
                    break;
                case 'user_list':
                    updateUserList(msg.users);
                    break;
                case 'error':
                    addMessage('error', msg.message, msg.timestamp);
                    break;
            }
        }
        
        function addMessage(type, text, timestamp) {
            const messages = document.getElementById('messages');
            const div = document.createElement('div');
            div.className = 'message ' + type;
            
            let timeStr = '';
            if (timestamp) {
                const time = new Date(timestamp);
                timeStr = '[' + time.toLocaleTimeString() + '] ';
            }
            
            div.textContent = timeStr + text;
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }
        
        function updateUserList(users) {
            const userList = document.getElementById('userList');
            userList.innerHTML = '';
            
            users.forEach(user => {
                const li = document.createElement('li');
                li.textContent = user;
                userList.appendChild(li);
            });
        }
        
        function setUsername() {
            currentUsername = document.getElementById('username').value.trim();
            if (!currentUsername) {
                alert('Please enter a username');
                return;
            }
            addMessage('system', 'Username set to: ' + currentUsername);
        }
        
        function joinRoom() {
            const roomName = document.getElementById('roomName').value.trim();
            if (!roomName || !currentUsername) {
                alert('Please set a username and enter a room name');
                return;
            }
            
            currentRoom = roomName;
            const msg = {
                type: 'join',
                user: currentUsername,
                room: roomName
            };
            
            ws.send(JSON.stringify(msg));
        }
        
        function leaveRoom() {
            if (!currentRoom) {
                alert('You are not in a room');
                return;
            }
            
            const msg = {
                type: 'leave',
                room: currentRoom
            };
            
            ws.send(JSON.stringify(msg));
            currentRoom = '';
        }
        
        function sendMessage() {
            const input = document.getElementById('messageInput');
            const message = input.value.trim();
            
            if (!message || !currentRoom || !currentUsername) {
                alert('Please join a room first and set a username');
                return;
            }
            
            const msg = {
                type: 'chat',
                room: currentRoom,
                message: message
            };
            
            ws.send(JSON.stringify(msg));
            input.value = '';
        }
        
        // Connect when page loads
        connect();
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func serveChatPage(w http.ResponseWriter, r *http.Request) {
	// Redirect to home for now
	http.Redirect(w, r, "/", http.StatusFound)
}
