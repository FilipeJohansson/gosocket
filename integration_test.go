package gosocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          3000,
			PerClientBurst:         3000,
			PerIPRate:              3000,
			PerIPBurst:             3000,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 3000,
		}),
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
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          3000,
			PerClientBurst:         3000,
			PerIPRate:              3000,
			PerIPBurst:             3000,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 3000,
		}),
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
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          3000,
			PerClientBurst:         3000,
			PerIPRate:              3000,
			PerIPBurst:             3000,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 3000,
		}),
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

func TestLargeMessagesEcho(t *testing.T) {
	const payloadSize = 512 * 1024 // 512 KB
	const totalMessages = 10

	var mu sync.Mutex
	received := make([][]byte, 0, totalMessages)

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          100,
			PerClientBurst:         100,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
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

	largePayload := make([]byte, payloadSize)
	pattern := []byte("LARGE_MESSAGE_TEST_PATTERN_")
	for i := 0; i < payloadSize; i += len(pattern) {
		copy(largePayload[i:], pattern)
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalMessages; i++ {
			_, msg, err := ws.ReadMessage()
			require.NoError(t, err)
			mu.Lock()
			received = append(received, msg)
			mu.Unlock()
		}
	}()

	// send messages with delays to avoid rate limiting
	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, largePayload))
		time.Sleep(50 * time.Millisecond) // delay between large messages
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, totalMessages, len(received))
	for i, msg := range received {
		require.Len(t, msg, payloadSize, "Message %d has wrong size", i)
		require.True(t, bytes.HasPrefix(msg, pattern), "Message %d doesn't start with expected pattern", i)
	}
}

func TestLargeMessagesConcurrent(t *testing.T) {
	const clientCount = 3
	const payloadSize = 256 * 1024 // 256 KB
	const messagesPerClient = 5

	var totalReceived int32
	var mu sync.Mutex
	clientMessages := make(map[string]int)

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          50,
			PerClientBurst:         50,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			atomic.AddInt32(&totalReceived, 1)

			clientID := string(m.RawData[:10])
			mu.Lock()
			clientMessages[clientID]++
			mu.Unlock()

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

	var wg sync.WaitGroup

	for i, ws := range clients {
		wg.Add(1)
		go func(clientID int, conn *websocket.Conn) {
			defer wg.Done()

			clientIDStr := fmt.Sprintf("CLIENT_%02d", clientID)
			largePayload := make([]byte, payloadSize)
			copy(largePayload, clientIDStr)
			pattern := []byte("_LARGE_DATA_")
			for j := len(clientIDStr); j < payloadSize; j += len(pattern) {
				copy(largePayload[j:], pattern)
			}

			for j := 0; j < messagesPerClient; j++ {
				err := conn.WriteMessage(websocket.TextMessage, largePayload)
				require.NoError(t, err)
				time.Sleep(200 * time.Millisecond)
			}
		}(i, ws)
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	expectedTotal := int32(clientCount * messagesPerClient)
	require.Equal(t, expectedTotal, atomic.LoadInt32(&totalReceived))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, clientMessages, clientCount)
	for clientID, count := range clientMessages {
		require.Equal(t, messagesPerClient, count, "Client %s sent wrong number of messages", clientID)
	}
}

func TestLargeMessageBroadcast(t *testing.T) {
	const clientCount = 4
	const payloadSize = 128 * 1024 // 128 KB
	const totalBroadcasts = 3

	var mu sync.Mutex
	received := make(map[int][]int)

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          50,
			PerClientBurst:         50,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
		WithMessageBufferSize(2048),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			// broadcast large message to all clients
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

	var wg sync.WaitGroup

	for i, ws := range clients {
		wg.Add(1)
		go func(clientID int, conn *websocket.Conn) {
			defer wg.Done()
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				mu.Lock()
				received[clientID] = append(received[clientID], len(msg))
				mu.Unlock()
			}
		}(i, ws)
	}

	largePayload := make([]byte, payloadSize)
	for i := 0; i < payloadSize; i++ {
		largePayload[i] = byte(i % 256)
	}

	for i := 0; i < totalBroadcasts; i++ {
		require.NoError(t, clients[0].WriteMessage(websocket.BinaryMessage, largePayload))
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)

	for _, ws := range clients {
		ws.Close()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	for clientID := 0; clientID < clientCount; clientID++ {
		sizes := received[clientID]
		require.Len(t, sizes, totalBroadcasts, "Client %d received wrong number of broadcasts", clientID)
		for j, size := range sizes {
			require.Equal(t, payloadSize, size, "Client %d broadcast %d has wrong size", clientID, j)
		}
	}
}

func TestLargeMessagesWithFailures(t *testing.T) {
	const payloadSize = 500 * 1024 // 500 KB
	const totalMessages = 5

	var successCount int32
	var errorCount int32

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          20,
			PerClientBurst:         20,
			PerIPRate:              40,
			PerIPBurst:             40,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 30,
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			atomic.AddInt32(&successCount, 1)
			return nil
		}),
		OnError(func(c *Client, err error, ctx *Context) error {
			atomic.AddInt32(&errorCount, 1)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	largePayload := make([]byte, payloadSize)
	for i := 0; i < payloadSize; i++ {
		largePayload[i] = byte(i % 256)
	}

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, largePayload))
		time.Sleep(300 * time.Millisecond)
	}

	// abruptly close connection to simulate failure
	ws.Close()

	// try to connect again and send more messages
	ws2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws2.Close()

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws2.WriteMessage(websocket.BinaryMessage, largePayload))
		time.Sleep(300 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)

	require.Greater(t, atomic.LoadInt32(&successCount), int32(0))
}

func TestProgressiveMessageSizes(t *testing.T) {
	sizes := []int{
		1 * 1024,   // 1 KB
		10 * 1024,  // 10 KB
		100 * 1024, // 100 KB
		500 * 1024, // 500 KB
		// 1 * 1024 * 1024, // 1 MB // TODO: implement fragmentation for large messages
		// 2 * 1024 * 1024, // 2 MB
	}

	var receivedCount int32
	var receivedSizes []int
	var mu sync.Mutex

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          10,
			PerClientBurst:         20,
			PerIPRate:              20,
			PerIPBurst:             40,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 50,
		}),
		WithMessageBufferSize(1024),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			atomic.AddInt32(&receivedCount, 1)
			mu.Lock()
			receivedSizes = append(receivedSizes, len(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		ReadBufferSize:   2048,
		WriteBufferSize:  2048,
	}

	ws, _, err := dialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	for i, size := range sizes {
		t.Logf("Sending message of size %d bytes (%d KB)", size, size/1024)

		payload := make([]byte, size)
		for j := 0; j < size; j++ {
			payload[j] = byte(j % 256)
		}

		timeout := 5*time.Second + time.Duration(size/1024)*time.Millisecond
		_ = ws.SetWriteDeadline(time.Now().Add(timeout))

		err := ws.WriteMessage(websocket.BinaryMessage, payload)
		require.NoError(t, err, "Failed to send message %d of size %d", i, size)

		delay := 100*time.Millisecond + time.Duration(size/1024)*time.Millisecond
		time.Sleep(delay)
	}

	time.Sleep(3 * time.Second)

	require.Equal(t, int32(len(sizes)), atomic.LoadInt32(&receivedCount))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedSizes, len(sizes))
	for i, expectedSize := range sizes {
		require.Equal(t, expectedSize, receivedSizes[i], "Message %d has wrong size", i)
	}
}

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

func TestRateLimitingEnforced(t *testing.T) {
	const clientCount = 3
	const messagesPerClient = 50

	rlConfig := DefaultRateLimiterConfig()
	rlConfig.PerClientRate = 5
	rlConfig.PerClientBurst = 5
	rlConfig.PerIPRate = 10
	rlConfig.PerIPBurst = 10

	server, err := NewServer(
		WithRateLimit(rlConfig),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			// echo msg
			return c.SendMessage(m)
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

	var wg sync.WaitGroup
	var rateLimitHits int32

	for _, ws := range clients {
		ws.SetCloseHandler(func(code int, text string) error {
			if code == websocket.CloseTryAgainLater {
				atomic.AddInt32(&rateLimitHits, 1)
			}
			return nil
		})
	}

	for _, ws := range clients {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					return
				}
			}
		}(ws)
	}

	for _, ws := range clients {
		wg.Add(1)
		go func(conn *websocket.Conn) {
			defer wg.Done()
			for i := 0; i < messagesPerClient; i++ {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("msg-%d", i)))
			}
		}(ws)
	}

	wg.Wait()

	require.Greater(t, atomic.LoadInt32(&rateLimitHits), int32(0), "Rate limite not enforced")
}

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

// Test different message types (Text, Binary, Ping, Pong)
func TestMessageTypes(t *testing.T) {
	var mu sync.Mutex
	received := make(map[int][]byte)

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			mu.Lock()
			received[int(m.Type)] = m.RawData
			mu.Unlock()
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

	textMsg := []byte("hello text")
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, textMsg))
	_, resp, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, textMsg, resp)

	binaryMsg := []byte{0x00, 0x01, 0x02, 0xFF}
	require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, binaryMsg))
	_, resp, err = ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, binaryMsg, resp)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, textMsg, received[websocket.TextMessage])
	require.Equal(t, binaryMsg, received[websocket.BinaryMessage])
}

// Test JSON message handling
func TestJSONMessages(t *testing.T) {
	type TestMessage struct {
		Type string `json:"type"`
		Data string `json:"data"`
		ID   int    `json:"id"`
	}

	var mu sync.Mutex
	var received []TestMessage

	server, err := NewServer(
		WithPath("/ws"),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			var msg TestMessage
			if err := json.Unmarshal(m.RawData, &msg); err != nil {
				return err
			}

			mu.Lock()
			received = append(received, msg)
			mu.Unlock()

			msg.Data = "processed: " + msg.Data
			response, _ := json.Marshal(msg)
			return c.Send(response)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	testMsg := TestMessage{Type: "test", Data: "hello", ID: 123}
	msgBytes, _ := json.Marshal(testMsg)

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, msgBytes))

	_, resp, err := ws.ReadMessage()
	require.NoError(t, err)

	var respMsg TestMessage
	require.NoError(t, json.Unmarshal(resp, &respMsg))
	require.Equal(t, "test", respMsg.Type)
	require.Equal(t, "processed: hello", respMsg.Data)
	require.Equal(t, 123, respMsg.ID)

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	require.Len(t, received, 1)
	require.Equal(t, testMsg, received[0])
	mu.Unlock()
}

// Test error handling in callbacks
func TestErrorHandling(t *testing.T) {
	var errorsCaught int32

	server, err := NewServer(
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Fail-Connect"}),
		OnBeforeConnect(func(r *http.Request, ctx *Context) error {
			if ctx.connInfo.Headers["Fail-Connect"] == "true" {
				return fmt.Errorf("connection rejected")
			}
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			if string(m.RawData) == "error" {
				atomic.AddInt32(&errorsCaught, 1)
				return fmt.Errorf("message error")
			}
			return c.Send(m.RawData)
		}),
		OnError(func(c *Client, err error, ctx *Context) error {
			atomic.AddInt32(&errorsCaught, 1)
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	headers := http.Header{"Fail-Connect": []string{"true"}}
	_, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("error")))
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("normal")))
	_, resp_msg, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, []byte("normal"), resp_msg)

	require.Greater(t, atomic.LoadInt32(&errorsCaught), int32(0))
}

// Test memory management under load
func TestMemoryLeaks(t *testing.T) {
	const rounds = 5
	const clientsPerRound = 10
	const messagesPerClient = 20

	server, err := NewServer(
		WithPath("/ws"),
		WithRateLimit(RateLimiterConfig{
			PerClientRate:          10000,
			PerClientBurst:         10000,
			PerIPRate:              50000,
			PerIPBurst:             50000,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 10000,
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	for round := 0; round < rounds; round++ {
		var clients []*websocket.Conn
		var wg sync.WaitGroup

		for i := 0; i < clientsPerRound; i++ {
			ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			require.NoError(t, err)
			clients = append(clients, ws)
		}

		for i, ws := range clients {
			wg.Add(1)
			go func(clientID int, conn *websocket.Conn) {
				defer wg.Done()
				for j := 0; j < messagesPerClient; j++ {
					msg := fmt.Sprintf("round-%d-client-%d-msg-%d", round, clientID, j)
					err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
					if err != nil {
						return
					}
					_, _, err = conn.ReadMessage()
					if err != nil {
						return
					}

					if j%5 == 0 {
						time.Sleep(time.Millisecond)
					}
				}
			}(i, ws)
		}
		wg.Wait()

		for _, ws := range clients {
			ws.Close()
		}

		time.Sleep(50 * time.Millisecond)
	}

	// server should still be responsive
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("final-test")))
	_, resp, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, []byte("final-test"), resp)
}

// Alternative memory leak test without rate limiting concerns
func TestMemoryLeaksWithoutMessages(t *testing.T) {
	const rounds = 10
	const clientsPerRound = 20

	server, err := NewServer(
		WithPath("/ws"),
		OnConnect(func(c *Client, ctx *Context) error {
			return nil
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	for round := 0; round < rounds; round++ {
		var clients []*websocket.Conn

		for i := 0; i < clientsPerRound; i++ {
			ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			require.NoError(t, err)
			clients = append(clients, ws)
		}

		time.Sleep(10 * time.Millisecond)

		for _, ws := range clients {
			ws.Close()
		}

		time.Sleep(50 * time.Millisecond)
	}

	// server should still be responsive after many connection cycles
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	time.Sleep(10 * time.Millisecond)
}

func TestCustomHeadersValidation(t *testing.T) {
	headersCh := make(chan map[string]string, 1)

	server, err := NewServer(
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Authorization", "User-Id", "Custom-Header"}),
		OnConnect(func(c *Client, ctx *Context) error {
			headers := make(map[string]string)
			for _, header := range []string{"Authorization", "User-Id", "Custom-Header"} {
				headers[header] = ctx.connInfo.Headers[header]
			}
			headersCh <- headers
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	headers := http.Header{
		"Authorization":  []string{"Bearer token123"},
		"User-Id":        []string{"user456"},
		"Custom-Header":  []string{"custom-value"},
		"Ignored-Header": []string{"this-should-be-ignored"},
	}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	require.NoError(t, err)
	defer ws.Close()

	select {
	case receivedHeaders := <-headersCh:
		require.Equal(t, "Bearer token123", receivedHeaders["Authorization"])
		require.Equal(t, "user456", receivedHeaders["User-Id"])
		require.Equal(t, "custom-value", receivedHeaders["Custom-Header"])
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for headers")
	}
}

func TestMultipleClientsWithDifferentHeaders(t *testing.T) {
	type clientHeaders struct {
		clientID string
		headers  map[string]string
	}

	var mu sync.Mutex
	var allReceivedHeaders []clientHeaders

	server, err := NewServer(
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Client-Id", "Authorization", "User-Role"}),
		OnConnect(func(c *Client, ctx *Context) error {
			clientID := ctx.connInfo.Headers["Client-Id"]
			headers := map[string]string{
				"Client-Id":     ctx.connInfo.Headers["Client-Id"],
				"Authorization": ctx.connInfo.Headers["Authorization"],
				"User-Role":     ctx.connInfo.Headers["User-Role"],
			}

			mu.Lock()
			allReceivedHeaders = append(allReceivedHeaders, clientHeaders{
				clientID: clientID,
				headers:  headers,
			})
			mu.Unlock()

			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	clients := []struct {
		id      string
		headers http.Header
	}{
		{
			id: "client1",
			headers: http.Header{
				"Client-Id":     []string{"client1"},
				"Authorization": []string{"Bearer token1"},
				"User-Role":     []string{"admin"},
			},
		},
		{
			id: "client2",
			headers: http.Header{
				"Client-Id":     []string{"client2"},
				"Authorization": []string{"Bearer token2"},
				"User-Role":     []string{"user"},
			},
		},
		{
			id: "client3",
			headers: http.Header{
				"Client-Id":     []string{"client3"},
				"Authorization": []string{"Basic dGVzdA=="},
				"User-Role":     []string{"guest"},
			},
		},
	}

	for _, client := range clients {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), client.headers)
		require.NoError(t, err)
		defer ws.Close()
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, allReceivedHeaders, len(clients))

	headersByClient := make(map[string]map[string]string)
	for _, received := range allReceivedHeaders {
		headersByClient[received.clientID] = received.headers
	}

	for _, expectedClient := range clients {
		actualHeaders, exists := headersByClient[expectedClient.id]
		require.True(t, exists, "Headers for client %s not found", expectedClient.id)

		require.Equal(t, expectedClient.headers["Client-Id"][0], actualHeaders["Client-Id"])
		require.Equal(t, expectedClient.headers["Authorization"][0], actualHeaders["Authorization"])
		require.Equal(t, expectedClient.headers["User-Role"][0], actualHeaders["User-Role"])
	}
}

func TestIgnoredHeaders(t *testing.T) {
	headersCh := make(chan map[string]string, 1)

	server, err := NewServer(
		WithPath("/ws"),
		WithRelevantHeaders([]string{"User-Id"}),
		OnConnect(func(c *Client, ctx *Context) error {
			headers := map[string]string{
				"User-Id":        ctx.connInfo.Headers["User-Id"],
				"Authorization":  ctx.connInfo.Headers["Authorization"],
				"Custom-Header":  ctx.connInfo.Headers["Custom-Header"],
				"Ignored-Header": ctx.connInfo.Headers["Ignored-Header"],
			}
			headersCh <- headers
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	headers := http.Header{
		"User-Id":        []string{"user456"},
		"Authorization":  []string{"Bearer token123"},
		"Custom-Header":  []string{"custom-value"},
		"Ignored-Header": []string{"this-should-be-ignored"},
	}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
	require.NoError(t, err)
	defer ws.Close()

	select {
	case receivedHeaders := <-headersCh:
		require.Equal(t, "user456", receivedHeaders["User-Id"])
		require.Equal(t, "Bearer token123", receivedHeaders["Authorization"])

		require.Empty(t, receivedHeaders["Custom-Header"])
		require.Empty(t, receivedHeaders["Ignored-Header"])

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for headers")
	}
}

// Test room management edge cases
func TestRoomEdgeCases(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0)

	server, err := NewServer(
		WithPath("/ws"),
		OnConnect(func(c *Client, ctx *Context) error {
			mu.Lock()
			events = append(events, "connect")
			mu.Unlock()
			return nil
		}),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			command := string(m.RawData)
			parts := strings.Split(command, ":")

			switch parts[0] {
			case "join":
				if len(parts) > 1 {
					c.Hub.JoinRoom(c, parts[1])
					mu.Lock()
					events = append(events, fmt.Sprintf("joined-%s", parts[1]))
					mu.Unlock()
				}
			case "leave":
				if len(parts) > 1 {
					c.Hub.LeaveRoom(c, parts[1])
					mu.Lock()
					events = append(events, fmt.Sprintf("left-%s", parts[1]))
					mu.Unlock()
				}
			case "broadcast":
				if len(parts) > 2 {
					c.Hub.BroadcastToRoom(parts[1], &Message{
						Type:    websocket.TextMessage,
						RawData: []byte(parts[2]),
					})
				}
			case "list":
				rooms := c.Hub.GetRooms()
				roomNames := make([]string, 0, len(rooms))
				for name := range rooms {
					roomNames = append(roomNames, name)
				}
				response := strings.Join(roomNames, ",")
				return c.Send([]byte(response))
			}
			return nil
		}),
	)
	require.NoError(t, err)

	_ = server.CreateRoom("room1")
	_ = server.CreateRoom("room2")

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	ws1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws1.Close()

	ws2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws2.Close()

	var readers sync.WaitGroup
	var received1, received2 []string
	var mu1, mu2 sync.Mutex

	readers.Add(2)
	go func() {
		defer readers.Done()
		for {
			_, msg, err := ws1.ReadMessage()
			if err != nil {
				return
			}
			mu1.Lock()
			received1 = append(received1, string(msg))
			mu1.Unlock()
		}
	}()

	go func() {
		defer readers.Done()
		for {
			_, msg, err := ws2.ReadMessage()
			if err != nil {
				return
			}
			mu2.Lock()
			received2 = append(received2, string(msg))
			mu2.Unlock()
		}
	}()

	// test joining rooms
	require.NoError(t, ws1.WriteMessage(websocket.TextMessage, []byte("join:room1")))
	require.NoError(t, ws2.WriteMessage(websocket.TextMessage, []byte("join:room2")))
	time.Sleep(50 * time.Millisecond)

	// test broadcasting to specific rooms
	require.NoError(t, ws1.WriteMessage(websocket.TextMessage, []byte("broadcast:room1:hello-room1")))
	require.NoError(t, ws2.WriteMessage(websocket.TextMessage, []byte("broadcast:room2:hello-room2")))
	time.Sleep(50 * time.Millisecond)

	// test joining same room
	require.NoError(t, ws2.WriteMessage(websocket.TextMessage, []byte("join:room1")))
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, ws1.WriteMessage(websocket.TextMessage, []byte("broadcast:room1:both-should-receive")))
	time.Sleep(50 * time.Millisecond)

	// test leaving room
	require.NoError(t, ws2.WriteMessage(websocket.TextMessage, []byte("leave:room1")))
	time.Sleep(50 * time.Millisecond)

	require.NoError(t, ws1.WriteMessage(websocket.TextMessage, []byte("broadcast:room1:only-ws1-should-receive")))
	time.Sleep(100 * time.Millisecond)

	ws1.Close()
	ws2.Close()
	readers.Wait()

	mu1.Lock()
	mu2.Lock()
	defer mu1.Unlock()
	defer mu2.Unlock()

	// check that messages were properly isolated by rooms
	require.Contains(t, received1, "hello-room1")
	require.Contains(t, received1, "both-should-receive")
	require.Contains(t, received1, "only-ws1-should-receive")

	require.Contains(t, received2, "hello-room2")
	require.Contains(t, received2, "both-should-receive")
	require.NotContains(t, received2, "only-ws1-should-receive")
}

// Test connection limits and cleanup
func TestConnectionLimits(t *testing.T) {
	const maxConnections = 5

	server, err := NewServer(
		WithPath("/ws"),
		WithMaxConnections(maxConnections),
		OnMessage(func(c *Client, m *Message, ctx *Context) error {
			return c.Send(m.RawData)
		}),
	)
	require.NoError(t, err)

	ts := httptest.NewServer(server.handler)
	defer ts.Close()
	u := url.URL{Scheme: "ws", Host: ts.Listener.Addr().String(), Path: "/ws"}

	var clients []*websocket.Conn

	// connect up to the limit
	for i := 0; i < maxConnections; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		require.NoError(t, err)
		clients = append(clients, ws)
	}

	// try to connect one more (should fail or be rejected)
	ws, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		require.Error(t, err)
		require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	} else {
		ws.Close()
	}

	for _, client := range clients {
		client.Close()
	}

	time.Sleep(100 * time.Millisecond)

	// should be able to connect again after cleanup
	ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("test-after-cleanup")))
	_, resp_msg, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, []byte("test-after-cleanup"), resp_msg)
}
