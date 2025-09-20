package gosocket

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestEcho(t *testing.T) {
	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()

	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	msg := []byte("hello")
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, msg))

	_, resp, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, msg, resp)
}

func TestBroadcast(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string][][]byte)

	server, err := NewServer(
		WithPath("/ws"),
		OnConnect(func(c *Client, ctx *Context) error {
			c.UserData = map[string]interface{}{"id": c.ID}
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	clients := make([]*websocket.Conn, 3)
	for i := 0; i < len(clients); i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		defer ws.Close()
		clients[i] = ws
	}

	msg := []byte("broadcast-test")
	require.NoError(t, clients[0].WriteMessage(websocket.TextMessage, msg))

	var wg sync.WaitGroup
	for i, ws := range clients {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			_, data, err := conn.ReadMessage()
			require.NoError(t, err)
			mu.Lock()
			received[strconv.Itoa(idx)] = append(received[strconv.Itoa(idx)], data)
			mu.Unlock()
		}(i, ws)
	}
	wg.Wait()

	for _, msgs := range received {
		require.Len(t, msgs, 1)
		require.Equal(t, msg, msgs[0])
	}
}

func TestDisconnect(t *testing.T) {
	disconnectedCh := make(chan struct{})

	var disconnected bool
	server, err := NewServer(
		WithPath("/ws"),
		OnDisconnect(func(c *Client, ctx *Context) error {
			disconnected = true
			close(disconnectedCh)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	ws.Close()
	<-disconnectedCh

	require.True(t, disconnected)
}

func TestRapidMessages(t *testing.T) {
	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	for i := 0; i < 1000; i++ {
		msg := []byte("msg" + strconv.Itoa(i))
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, msg))

		_, resp, err := ws.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, msg, resp)
	}
}

func TestConcurrentClients(t *testing.T) {
	const clientsCount = 100
	const messagesPerClient = 20

	var mu sync.Mutex
	received := make(map[string][]string)
	var disconnectWg sync.WaitGroup

	disconnectWg.Add(clientsCount)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			if bytes.HasPrefix(m.RawData, []byte("[ID]")) {
				mu.Lock()
				c.UserData["id"] = string(m.RawData[5:])
				mu.Unlock()
				return nil
			}
			return c.Send(m.RawData)
		}),
		OnDisconnect(func(c *Client, ctx *Context) error {
			mu.Lock()
			id := c.UserData["id"].(string)
			received[id] = append(received[id], "disconnected")
			mu.Unlock()
			disconnectWg.Done()
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, clientsCount)

	for i := 0; i < clientsCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients[i] = ws
	}

	for i, ws := range clients {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()

			clientId := fmt.Sprintf("client-%d", idx)
			msg := fmt.Sprintf("[ID] %s", clientId)
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))

			for j := 0; j < messagesPerClient; j++ {
				msg = fmt.Sprintf("%s-msg-%d", clientId, j)
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))

				_, resp, err := conn.ReadMessage()
				require.NoError(t, err)
				require.Equal(t, msg, string(resp))

				mu.Lock()
				received[clientId] = append(received[clientId], string(resp))
				mu.Unlock()
			}
		}(i, ws)
	}
	wg.Wait()

	for _, ws := range clients {
		ws.Close()
	}

	disconnectWg.Wait()

	for i := 0; i < clientsCount; i++ {
		key := fmt.Sprintf("client-%d", i)
		require.Len(t, received[key], messagesPerClient+1)
		require.Equal(t, "disconnected", received[key][messagesPerClient])
	}
}

func TestConcurrentBroadcast(t *testing.T) {
	const clientsCount = 10
	const messagesPerClient = 100

	var mu sync.Mutex
	received := make(map[string][]string)

	server, err := NewServer(
		WithPath("/ws"),
		WithMessageBufferSize(1024),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	clients := make([]*websocket.Conn, clientsCount)
	for i := 0; i < clientsCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients[i] = ws
		defer ws.Close()
	}

	var wg sync.WaitGroup
	for i, ws := range clients {
		wg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer wg.Done()
			for j := 0; j < messagesPerClient; j++ {
				msg := fmt.Sprintf("client-%d-msg-%d", idx, j)
				require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(msg)))
			}
		}(i, ws)
	}
	wg.Wait()

	var readWg sync.WaitGroup
	for i, ws := range clients {
		readWg.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer readWg.Done()
			totalMessages := clientsCount * messagesPerClient
			for k := 0; k < totalMessages; k++ {
				_, resp, err := conn.ReadMessage()
				require.NoError(t, err)
				mu.Lock()
				received[fmt.Sprintf("client-%d", idx)] = append(received[fmt.Sprintf("client-%d", idx)], string(resp))
				mu.Unlock()
			}
		}(i, ws)
	}
	readWg.Wait()

	expected := clientsCount * messagesPerClient
	for i := 0; i < clientsCount; i++ {
		key := fmt.Sprintf("client-%d", i)
		require.Len(t, received[key], expected)
	}
}

func TestUnexpectedDisconnect(t *testing.T) {
	const clientsCount = 5
	const messagesPerClient = 3

	var mu sync.Mutex
	received := make(map[string][]string)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	clients := make([]*websocket.Conn, clientsCount)
	for i := 0; i < clientsCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients[i] = ws
	}

	var readers sync.WaitGroup
	for i, ws := range clients {
		readers.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer readers.Done()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				mu.Lock()
				received[fmt.Sprintf("client-%d", idx)] = append(received[fmt.Sprintf("client-%d", idx)], string(msg))
				mu.Unlock()
			}
		}(i, ws)
	}

	require.NoError(t, clients[0].Close())

	for i := 1; i < clientsCount; i++ {
		for j := 0; j < messagesPerClient; j++ {
			msg := fmt.Sprintf("from-client-%d-msg-%d", i, j)
			require.NoError(t, clients[i].WriteMessage(websocket.TextMessage, []byte(msg)))
		}
	}

	time.Sleep(200 * time.Millisecond)

	for i := 1; i < clientsCount; i++ {
		clients[i].Close()
	}
	readers.Wait()

	_, hasClient0 := received["client-0"]
	require.False(t, hasClient0, "client-0 should be disconnected")

	expected := (clientsCount - 1) * messagesPerClient
	for i := 1; i < clientsCount; i++ {
		key := fmt.Sprintf("client-%d", i)
		require.Len(t, received[key], expected, "client %d did not receive all messages", i)
	}
}

func TestReconnect(t *testing.T) {
	const messagesPerClient = 3

	var mu sync.Mutex
	received := make(map[string][]string)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	ws2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	var readers sync.WaitGroup
	readers.Add(2)

	go func() {
		defer readers.Done()
		for {
			_, msg, err := ws1.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			received["client-1"] = append(received["client-1"], string(msg))
			mu.Unlock()
		}
	}()

	go func() {
		defer readers.Done()
		for {
			_, msg, err := ws2.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			received["client-2"] = append(received["client-2"], string(msg))
			mu.Unlock()
		}
	}()

	for i := 0; i < messagesPerClient; i++ {
		msg := fmt.Sprintf("client-2-msg-%d", i)
		require.NoError(t, ws2.WriteMessage(websocket.TextMessage, []byte(msg)))
	}

	require.NoError(t, ws2.Close())

	ws2New, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			_, msg, err := ws2New.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			received["client-2-reconnected"] = append(received["client-2-reconnected"], string(msg))
			mu.Unlock()
		}
	}()

	for i := 0; i < messagesPerClient; i++ {
		msg := fmt.Sprintf("client-1-msg-%d", i)
		require.NoError(t, ws1.WriteMessage(websocket.TextMessage, []byte(msg)))
	}

	time.Sleep(200 * time.Millisecond)

	_ = ws1.Close()
	_ = ws2New.Close()
	readers.Wait()

	require.GreaterOrEqual(t, len(received["client-1"]), messagesPerClient)
	require.GreaterOrEqual(t, len(received["client-2-reconnected"]), messagesPerClient)
}

func TestLargeMessages(t *testing.T) {
	const payloadSize = 512 * 1024 // 512 KB
	const totalMessages = 3

	var mu sync.Mutex
	received := make([]string, 0, totalMessages)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			mu.Lock()
			received = append(received, string(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	largePayload := bytes.Repeat([]byte("A"), payloadSize)

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, largePayload))
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	receivedCount := len(received)
	mu.Unlock()

	require.Equal(t, totalMessages, receivedCount)
	for _, msg := range received {
		require.Len(t, msg, payloadSize)
	}
}

/* func TestLargeMessagesStress(t *testing.T) {
	const payloadSize = 1 * 1024 * 1024 // 1 MB
	const totalMessages = 50

	var mu sync.Mutex
	received := make([]string, 0, totalMessages)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			mu.Lock()
			received = append(received, string(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	largePayload := bytes.Repeat([]byte("X"), payloadSize)

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, largePayload))
	}

	time.Sleep(2 * time.Second)

	mu.Lock()
	receivedCount := len(received)
	mu.Unlock()

	require.Equal(t, totalMessages, receivedCount)
	for _, msg := range received {
		require.Len(t, msg, payloadSize)
	}
} */

func TestMessageOrder(t *testing.T) {
	const totalMessages = 20

	var mu sync.Mutex
	var received []string

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			mu.Lock()
			received = append(received, string(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	for i := 0; i < totalMessages; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(msg)))
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, totalMessages, len(received))
	for i := 0; i < totalMessages; i++ {
		expected := fmt.Sprintf("msg-%d", i)
		require.Equal(t, expected, received[i])
	}
}

func TestBroadcastMessageOrder(t *testing.T) {
	const totalMessages = 15

	var mu sync.Mutex
	received := make(map[string][]string)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	wsSender, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer wsSender.Close()

	wsReceiver1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer wsReceiver1.Close()

	wsReceiver2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer wsReceiver2.Close()

	var readers sync.WaitGroup
	readers.Add(2)

	go func() {
		defer readers.Done()
		for {
			_, msg, err := wsReceiver1.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			received["r1"] = append(received["r1"], string(msg))
			mu.Unlock()
		}
	}()

	go func() {
		defer readers.Done()
		for {
			_, msg, err := wsReceiver2.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			received["r2"] = append(received["r2"], string(msg))
			mu.Unlock()
		}
	}()

	for i := 0; i < totalMessages; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		require.NoError(t, wsSender.WriteMessage(websocket.TextMessage, []byte(msg)))
	}

	time.Sleep(300 * time.Millisecond)

	_ = wsSender.Close()
	_ = wsReceiver1.Close()
	_ = wsReceiver2.Close()
	readers.Wait()

	mu.Lock()
	defer mu.Unlock()

	for client, msgs := range received {
		require.Equal(t, totalMessages, len(msgs), "client=%s", client)
		for i := 0; i < totalMessages; i++ {
			expected := fmt.Sprintf("msg-%d", i)
			require.Equal(t, expected, msgs[i], "client=%s index=%d", client, i)
		}
	}
}

func TestMultipleHubsIsolation(t *testing.T) {
	var mu sync.Mutex
	receivedHub1 := make([]string, 0)
	receivedHub2 := make([]string, 0)

	server1, err := NewServer(
		WithPath("/ws1"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	server2, err := NewServer(
		WithPath("/ws2"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.Handle("/ws1", server1.handler)
	mux.Handle("/ws2", server2.handler)

	ts := httptest.NewServer(mux)
	defer ts.Close()
	host := ts.Listener.Addr().String()

	u1 := url.URL{Scheme: "ws", Host: host, Path: "/ws1"}
	u2 := url.URL{Scheme: "ws", Host: host, Path: "/ws2"}

	wsHub1, _, err := websocket.DefaultDialer.Dial(u1.String(), nil)
	require.NoError(t, err)
	defer wsHub1.Close()

	wsHub2, _, err := websocket.DefaultDialer.Dial(u2.String(), nil)
	require.NoError(t, err)
	defer wsHub2.Close()

	var readers sync.WaitGroup
	readers.Add(2)

	go func() {
		defer readers.Done()
		for {
			_, msg, err := wsHub1.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			receivedHub1 = append(receivedHub1, string(msg))
			mu.Unlock()
		}
	}()

	go func() {
		defer readers.Done()
		for {
			_, msg, err := wsHub2.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			receivedHub2 = append(receivedHub2, string(msg))
			mu.Unlock()
		}
	}()

	require.NoError(t, wsHub1.WriteMessage(websocket.TextMessage, []byte("hub1-msg")))
	require.NoError(t, wsHub2.WriteMessage(websocket.TextMessage, []byte("hub2-msg")))

	time.Sleep(300 * time.Millisecond)

	_ = wsHub1.Close()
	_ = wsHub2.Close()
	readers.Wait()

	mu.Lock()
	defer mu.Unlock()

	require.Contains(t, receivedHub1, "hub1-msg")
	require.NotContains(t, receivedHub1, "hub2-msg")

	require.Contains(t, receivedHub2, "hub2-msg")
	require.NotContains(t, receivedHub2, "hub1-msg")
}

func TestRoomsIsolation(t *testing.T) {
	var mu sync.Mutex
	received := make(map[string][]string)

	server, err := NewServer(
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Room"}),
		OnConnect(func(c *Client, ctx *Context) error {
			c.Hub.JoinRoom(c, ctx.connInfo.Headers["Room"])
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			roomName := ctx.connInfo.Headers["Room"]
			room := c.Hub.GetRooms()[roomName]
			if room != nil {
				c.Hub.BroadcastToRoom(roomName, m)
			}
			return nil
		}),
	)
	require.NoError(t, err)

	_ = server.CreateRoom("room1")
	_ = server.CreateRoom("room2")

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	host := ts.Listener.Addr().String()
	u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}

	joinRoom := func(room string) *websocket.Conn {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{"Room": []string{room}})
		require.NoError(t, err)
		return ws
	}

	ws1a := joinRoom("room1")
	defer ws1a.Close()
	ws1b := joinRoom("room1")
	defer ws1b.Close()

	ws2a := joinRoom("room2")
	defer ws2a.Close()
	ws2b := joinRoom("room2")
	defer ws2b.Close()

	var readers sync.WaitGroup
	readers.Add(4)

	startReader := func(name string, ws *websocket.Conn) {
		go func() {
			defer readers.Done()
			for {
				_, msg, err := ws.ReadMessage()
				if err != nil {
					return
				}
				mu.Lock()
				received[name] = append(received[name], string(msg))
				mu.Unlock()
			}
		}()
	}

	startReader("r1a", ws1a)
	startReader("r1b", ws1b)
	startReader("r2a", ws2a)
	startReader("r2b", ws2b)

	require.NoError(t, ws1a.WriteMessage(websocket.TextMessage, []byte("hello-room1")))
	require.NoError(t, ws2a.WriteMessage(websocket.TextMessage, []byte("hello-room2")))

	time.Sleep(300 * time.Millisecond)

	_ = ws1a.Close()
	_ = ws1b.Close()
	_ = ws2a.Close()
	_ = ws2b.Close()
	readers.Wait()

	mu.Lock()
	defer mu.Unlock()

	require.Contains(t, received["r1a"], "hello-room1")
	require.Contains(t, received["r1b"], "hello-room1")
	require.NotContains(t, received["r1a"], "hello-room2")
	require.NotContains(t, received["r1b"], "hello-room2")

	require.Contains(t, received["r2a"], "hello-room2")
	require.Contains(t, received["r2b"], "hello-room2")
	require.NotContains(t, received["r2a"], "hello-room1")
	require.NotContains(t, received["r2b"], "hello-room1")
}

// TODO: uncomment this test once rate limiter is implemented
/* func TestRateLimiting(t *testing.T) {
	const clientCount = 5
	const messagesToSend = 1000

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(100, time.Second),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	var clients []*websocket.Conn
	for i := 0; i < clientCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients = append(clients, ws)
		defer ws.Close()
	}

	var readers sync.WaitGroup
	for i, ws := range clients {
		readers.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer readers.Done()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}(i, ws)
	}

	var writers sync.WaitGroup
	for i, ws := range clients {
		writers.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer writers.Done()
			for j := 0; j < messagesToSend; j++ {
				msg := fmt.Sprintf("client-%d-msg-%d", idx, j)
				_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
			}
		}(i, ws)
	}

	writers.Wait()
	time.Sleep(500 * time.Millisecond)
	for _, ws := range clients {
		_ = ws.Close()
	}
	readers.Wait()
} */

func TestHubShutdown(t *testing.T) {
	const clientCount = 5
	const messagesPerClient = 10

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			c.Hub.BroadcastMessage(m)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	var clients []*websocket.Conn
	for i := 0; i < clientCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients = append(clients, ws)
	}

	var readers sync.WaitGroup
	for i, ws := range clients {
		readers.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer readers.Done()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}(i, ws)
	}

	var writers sync.WaitGroup
	for i, ws := range clients {
		writers.Add(1)
		go func(idx int, conn *websocket.Conn) {
			defer writers.Done()
			for j := 0; j < messagesPerClient; j++ {
				msg := fmt.Sprintf("client-%d-msg-%d", idx, j)
				_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
			}
		}(i, ws)
	}

	writers.Wait()

	_ = server.Stop() // stop the hub and all goroutines

	for _, ws := range clients {
		_ = ws.Close()
	}
	readers.Wait()
}
