//go:build example

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/FilipeJohansson/gosocket"
)

type Message struct {
	Type   string      `json:"type"`
	Player string      `json:"player,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Time   time.Time   `json:"time"`
}

var (
	players = make(map[string]string)   // clientID -> playerName
	games   = make(map[string][]string) // gameID -> playerIDs
	mu      sync.RWMutex
)

func main() {
	ws, err := gosocket.NewServer(
		gosocket.WithPort(8081),
		gosocket.WithPath("/ws"),
		gosocket.OnStart(onStart),
		gosocket.OnConnect(onConnect),
		gosocket.OnDisconnect(onDisconnect),
		gosocket.OnJSONMessage(onMessage),
	)

	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", servePage)

	go func() {
		log.Fatal(ws.Start())
	}()

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func onStart(ctx *gosocket.Context) error {
	ctx.CreateRoom("__SERVER__", "lobby")
	return nil
}

func onConnect(d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}
	ctx.JoinRoom(client.GetID(), "lobby")
	err := d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(
		gosocket.TextMessage,
		Message{
			Type: "welcome",
			Data: "Welcome to Game Lobby!",
			Time: time.Now(),
		},
		gosocket.JSON,
	))
	if err != nil {
		return err
	}

	return nil
}

func onDisconnect(d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}

	mu.Lock()
	delete(players, client.GetID())

	// Remove from any games
	for gameID, playerIDs := range games {
		for i, id := range playerIDs {
			if id == client.GetID() {
				games[gameID] = append(playerIDs[:i], playerIDs[i+1:]...)
				if len(games[gameID]) == 0 {
					delete(games, gameID)
				}
				break
			}
		}
	}
	mu.Unlock()

	broadcastLobbyState(d, ctx)
	return nil
}

func onMessage(data interface{}, d gosocket.Dispatcher, ctx *gosocket.Context) error {
	jsonBytes, _ := json.Marshal(data)
	var msg Message
	json.Unmarshal(jsonBytes, &msg)

	switch msg.Type {
	case "set_name":
		return handleSetName(msg.Data.(string), d, ctx)
	case "create_game":
		return handleCreateGame(d, ctx)
	case "join_game":
		return handleJoinGame(msg.Data.(string), d, ctx)
	case "leave_game":
		return handleLeaveGame(d, ctx)
	}

	return nil
}

func handleSetName(name string, d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}

	mu.Lock()
	players[client.GetID()] = name
	mu.Unlock()

	err := d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(
		gosocket.TextMessage,
		Message{
			Type: "name_set",
			Data: name,
			Time: time.Now(),
		},
		gosocket.JSON,
	))
	if err != nil {
		return err
	}

	broadcastLobbyState(d, ctx)
	return nil
}

func handleCreateGame(d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}

	mu.Lock()
	gameID := fmt.Sprintf("game_%d", time.Now().Unix())
	games[gameID] = []string{client.GetID()}
	mu.Unlock()

	_, err := ctx.CreateRoom(client.GetID(), gameID)
	if err != nil {
		return err
	}

	err = ctx.JoinRoom(client.GetID(), gameID)
	if err != nil {
		return err
	}

	err = d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(
		gosocket.TextMessage,
		Message{
			Type: "game_created",
			Data: gameID,
			Time: time.Now(),
		},
		gosocket.JSON,
	))
	if err != nil {
		return err
	}

	broadcastLobbyState(d, ctx)
	return nil
}

func handleJoinGame(gameID string, d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}

	mu.Lock()
	if playerIDs, exists := games[gameID]; exists && len(playerIDs) < 4 {
		err := ctx.JoinRoom(client.GetID(), gameID)
		if err != nil {
			mu.Unlock()
			return err
		}

		games[gameID] = append(playerIDs, client.GetID())
		mu.Unlock()

		err = d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(
			gosocket.TextMessage,
			Message{
				Type: "game_joined",
				Data: gameID,
				Time: time.Now(),
			},
			gosocket.JSON,
		))
		if err != nil {
			return err
		}

		broadcastLobbyState(d, ctx)
		broadcastGameState(gameID, d, ctx)
		return nil
	}
	mu.Unlock()

	return d.SendToClient(client.GetID(), gosocket.NewMessageWithEncoding(gosocket.TextMessage,
		Message{
			Type: "error",
			Data: "Cannot join game",
			Time: time.Now(),
		},
		gosocket.JSON,
	))
}

func handleLeaveGame(d gosocket.Dispatcher, ctx *gosocket.Context) error {
	client, exists := ctx.Client()
	if !exists {
		return nil
	}

	mu.Lock()
	for gameID, playerIDs := range games {
		for i, id := range playerIDs {
			if id == client.GetID() {
				games[gameID] = append(playerIDs[:i], playerIDs[i+1:]...)
				if len(games[gameID]) == 0 {
					delete(games, gameID)
				}
				ctx.LeaveRoom(client.GetID(), gameID)
				break
			}
		}
	}
	mu.Unlock()

	broadcastLobbyState(d, ctx)
	return nil
}

func broadcastLobbyState(d gosocket.Dispatcher, ctx *gosocket.Context) {
	mu.RLock()

	gameList := make([]map[string]interface{}, 0)
	for gameID, playerIDs := range games {
		gameNames := make([]string, len(playerIDs))
		for i, id := range playerIDs {
			gameNames[i] = players[id]
		}
		gameList = append(gameList, map[string]interface{}{
			"id":      gameID,
			"players": gameNames,
			"count":   len(playerIDs),
		})
	}

	playerList := make([]string, 0, len(players))
	for _, name := range players {
		playerList = append(playerList, name)
	}
	mu.RUnlock()

	msg := gosocket.NewMessage(gosocket.TextMessage, Message{
		Type: "lobby_update",
		Data: map[string]interface{}{
			"players": playerList,
			"games":   gameList,
		},
		Time: time.Now(),
	})
	msg.Encoding = gosocket.JSON
	d.BroadcastToRoom("lobby", msg)
}

func broadcastGameState(gameID string, d gosocket.Dispatcher, ctx *gosocket.Context) {
	mu.RLock()
	playerIDs := games[gameID]
	gameNames := make([]string, len(playerIDs))
	for i, id := range playerIDs {
		gameNames[i] = players[id]
	}
	mu.RUnlock()

	msg := gosocket.NewMessage(gosocket.TextMessage, Message{
		Type: "game_update",
		Data: map[string]interface{}{
			"game_id": gameID,
			"players": gameNames,
		},
		Time: time.Now(),
	})
	msg.Encoding = gosocket.JSON
	d.BroadcastToRoom(gameID, msg)
}

func servePage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Simple Game Lobby</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .section { margin: 20px 0; padding: 15px; border: 1px solid #ccc; }
        button { margin: 5px; padding: 8px 16px; }
        input { margin: 5px; padding: 8px; }
        .game { background: #f0f8ff; margin: 5px 0; padding: 10px; }
        .players { background: #f8f8f0; margin: 5px 0; padding: 5px; }
    </style>
</head>
<body>
    <h1>Simple Game Lobby</h1>
    
    <div class="section">
        <input type="text" id="playerName" placeholder="Your name">
        <button onclick="setName()">Set Name</button>
    </div>
    
    <div class="section">
        <h3>Games</h3>
        <button onclick="createGame()">Create Game</button>
        <div id="games"></div>
    </div>
    
    <div class="section">
        <h3>Players Online</h3>
        <div id="players"></div>
    </div>
    
    <div class="section">
        <h3>Messages</h3>
        <div id="messages" style="height: 200px; overflow-y: auto; border: 1px solid #eee; padding: 10px;"></div>
    </div>

    <script>
        const ws = new WebSocket('ws://localhost:8081/ws');
        let currentGame = null;
        
        ws.onmessage = function(event) {
            const msg = JSON.parse(event.data);
            addMessage(msg.type + ': ' + JSON.stringify(msg.data));
            
            if (msg.type === 'lobby_update') {
                updateLobby(msg.data);
            } else if (msg.type === 'game_created') {
                currentGame = msg.data;
            } else if (msg.type === 'game_joined') {
                currentGame = msg.data;
            }
        };
        
        function setName() {
            const name = document.getElementById('playerName').value;
            if (name) {
                ws.send(JSON.stringify({ type: 'set_name', data: name }));
            }
        }
        
        function createGame() {
            ws.send(JSON.stringify({ type: 'create_game' }));
        }
        
        function joinGame(gameId) {
            ws.send(JSON.stringify({ type: 'join_game', data: gameId }));
        }
        
        function leaveGame() {
            ws.send(JSON.stringify({ type: 'leave_game' }));
            currentGame = null;
        }
        
        function updateLobby(data) {
            // Update games list
            const gamesDiv = document.getElementById('games');
            gamesDiv.innerHTML = '';
            
            if (currentGame) {
                const leaveBtn = document.createElement('button');
                leaveBtn.textContent = 'Leave Current Game';
                leaveBtn.onclick = leaveGame;
                gamesDiv.appendChild(leaveBtn);
            }
            
            data.games.forEach(game => {
                const gameDiv = document.createElement('div');
                gameDiv.className = 'game';
                gameDiv.innerHTML = '<strong>' + game.id + '</strong> (' + game.count + '/4 players)<br>' +
                    'Players: ' + game.players.join(', ') + 
                    '<button onclick="joinGame(\'' + game.id + '\')">Join</button>';
                gamesDiv.appendChild(gameDiv);
            });
            
            // Update players list
            const playersDiv = document.getElementById('players');
            playersDiv.innerHTML = '<div class="players">Online: ' + data.players.join(', ') + '</div>';
        }
        
        function addMessage(text) {
            const div = document.createElement('div');
            div.textContent = new Date().toLocaleTimeString() + ' - ' + text;
            document.getElementById('messages').appendChild(div);
            div.scrollIntoView();
        }
        
        ws.onopen = () => addMessage('Connected to lobby');
        ws.onclose = () => addMessage('Disconnected');
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
