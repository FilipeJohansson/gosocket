package gosocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gsWebsocket "github.com/FilipeJohansson/gosocket/internal/transport/websocket"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func newTestServer(t *testing.T, opts ...gsWebsocket.UniversalOption) *httptest.Server {
	handler, err := NewHandler(opts...)
	require.NoError(t, err)

	ts := httptest.NewServer(handler)
	return ts
}

func TestIntegration_BasicEcho(t *testing.T) {
	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			// Echo back the message
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send message
	testMsg := []byte("hello world")
	err = ws.WriteMessage(websocket.TextMessage, testMsg)
	require.NoError(t, err)

	// Receive echo
	_, resp, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, testMsg, resp)
}

func TestIntegration_MultiClientEcho(t *testing.T) {
	const numClients = 50

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          50,
			PerClientBurst:         50,
			PerIPRate:              50,
			PerIPBurst:             50,
			MaxRateLimitViolations: 50,
			CleanupInterval:        50,
			EntryTTL:               50,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	var clients []*websocket.Conn
	for i := 0; i < numClients; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		require.NoError(t, err)
		clients = append(clients, ws)
	}
	defer func() {
		for _, ws := range clients {
			ws.Close()
		}
	}()

	// Each client sends a message
	for i, ws := range clients {
		testMsg := []byte(fmt.Sprintf("client-%d", i))
		err := ws.WriteMessage(websocket.TextMessage, testMsg)
		require.NoError(t, err)

		_, resp, err := ws.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, testMsg, resp)
	}
}

func TestIntegration_OnConnectOnDisconnect(t *testing.T) {
	connectCalled := atomic.Bool{}
	disconnectCalled := atomic.Bool{}
	disconnectDone := make(chan struct{})

	server := newTestServer(t,
		WithPath("/ws"),
		OnConnect(func(d Dispatcher, ctx *Context) error {
			connectCalled.Store(true)
			return nil
		}),
		OnDisconnect(func(d Dispatcher, ctx *Context) error {
			disconnectCalled.Store(true)
			close(disconnectDone)
			return nil
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)

	// Give handler time to call OnConnect
	time.Sleep(50 * time.Millisecond)
	require.True(t, connectCalled.Load())

	ws.Close()

	// Wait for disconnect
	select {
	case <-disconnectDone:
		require.True(t, disconnectCalled.Load())
	case <-time.After(2 * time.Second):
		t.Fatal("OnDisconnect not called")
	}
}

func TestIntegration_MessageReceived(t *testing.T) {
	messagesCh := make(chan string, 10)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			select {
			case messagesCh <- string(m.RawData):
			default:
			}
			return nil
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	// Send multiple messages
	testMessages := []string{"msg1", "msg2", "msg3"}
	for _, msg := range testMessages {
		err := ws.WriteMessage(websocket.TextMessage, []byte(msg))
		require.NoError(t, err)
	}

	// Receive them
	received := make(map[string]bool)
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case msg := <-messagesCh:
			received[msg] = true
			if len(received) == len(testMessages) {
				goto allReceived
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("timeout waiting for all messages")

allReceived:
	for _, msg := range testMessages {
		require.True(t, received[msg], "message not received: %s", msg)
	}
}

func TestIntegration_ConcurrentConnections(t *testing.T) {
	const clientsCount = 100
	const messagesPerClient = 20

	var mu sync.Mutex
	received := make(map[string][]string)
	var disconnectWg sync.WaitGroup

	disconnectWg.Add(clientsCount)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          1000,
			PerClientBurst:         1000,
			PerIPRate:              1000,
			PerIPBurst:             1000,
			MaxRateLimitViolations: 1000,
			CleanupInterval:        1000,
			EntryTTL:               1000,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}

			if bytes.HasPrefix(m.RawData, []byte("[ID]")) {
				mu.Lock()
				client.SetUserData("id", string(m.RawData[5:]))
				mu.Unlock()
				return nil
			}

			if err := d.SendToClient(client.GetID(), m); err != nil {
				return err
			}
			return nil
		}),
		OnDisconnect(func(d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}

			mu.Lock()
			id := client.GetUserDataByKey("id").(string)
			received[id] = append(received[id], "disconnected")
			mu.Unlock()

			disconnectWg.Done()
			return nil
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	var wg sync.WaitGroup
	clients := make([]*websocket.Conn, clientsCount)

	for i := 0; i < clientsCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
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

func TestIntegration_ConcurrentBroadcast(t *testing.T) {
	const clientsCount = 10
	const messagesPerClient = 100

	var mu sync.Mutex
	received := make(map[string][]string)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          1000,
			PerClientBurst:         1000,
			PerIPRate:              1000,
			PerIPBurst:             1000,
			MaxRateLimitViolations: 1000,
			CleanupInterval:        1000,
			EntryTTL:               1000,
		}),
		WithMessageBufferSize(1024),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			return d.Broadcast(m)
		}),
	)
	defer server.Close()

	wsURL := url.URL{
		Scheme: "ws",
		Host:   server.Listener.Addr().String(),
		Path:   "/ws",
	}

	clients := make([]*websocket.Conn, clientsCount)
	for i := 0; i < clientsCount; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
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

func TestIntegration_UnexpectedDisconnect(t *testing.T) {
	const clientsCount = 5
	const messagesPerClient = 3

	var mu sync.Mutex
	received := make(map[string][]string)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			return d.Broadcast(m)
		}),
	)
	defer server.Close()

	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

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

func TestIntegration_Reconnect(t *testing.T) {
	const messagesPerClient = 3

	var mu sync.Mutex
	received := make(map[string][]string)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			return d.Broadcast(m)
		}),
	)
	defer server.Close()

	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

	_ = ws1.Close()
	_ = ws2New.Close()
	readers.Wait()

	require.GreaterOrEqual(t, len(received["client-1"]), messagesPerClient)
	require.GreaterOrEqual(t, len(received["client-2-reconnected"]), messagesPerClient)
}

func TestIntegration_LargeMessages(t *testing.T) {
	const payloadSize = 512 * 1024 // 512 KB
	const totalMessages = 3

	var mu sync.Mutex
	received := make([]string, 0, totalMessages)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			mu.Lock()
			received = append(received, string(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	largePayload := bytes.Repeat([]byte("A"), payloadSize)

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, largePayload))
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	receivedCount := len(received)
	mu.Unlock()

	require.Equal(t, totalMessages, receivedCount)
	for _, msg := range received {
		require.Len(t, msg, payloadSize)
	}
}

func TestIntegration_LargeMessagesEcho(t *testing.T) {
	const payloadSize = 512 * 1024 // 512 KB
	const totalMessages = 10

	var mu sync.Mutex
	received := make([][]byte, 0, totalMessages)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          100,
			PerClientBurst:         100,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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
		time.Sleep(time.Millisecond) // delay between large messages
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

func TestIntegration_LargeMessagesConcurrent(t *testing.T) {
	const clientCount = 3
	const payloadSize = 256 * 1024 // 256 KB
	const messagesPerClient = 5

	var totalReceived int32
	var mu sync.Mutex
	clientMessages := make(map[string]int)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          50,
			PerClientBurst:         50,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			atomic.AddInt32(&totalReceived, 1)

			clientID := string(m.RawData[:10])
			mu.Lock()
			clientMessages[clientID]++
			mu.Unlock()

			return nil
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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
				time.Sleep(10 * time.Millisecond)
			}
		}(i, ws)
	}
	wg.Wait()

	time.Sleep(200 * time.Millisecond)

	expectedTotal := int32(clientCount * messagesPerClient)
	require.Equal(t, expectedTotal, atomic.LoadInt32(&totalReceived))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, clientMessages, clientCount)
	for clientID, count := range clientMessages {
		require.Equal(t, messagesPerClient, count, "Client %s sent wrong number of messages", clientID)
	}
}

func TestIntegration_LargeMessageBroadcast(t *testing.T) {
	const clientCount = 4
	const payloadSize = 128 * 1024 // 128 KB
	const totalBroadcasts = 3

	var mu sync.Mutex
	received := make(map[int][]int)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          50,
			PerClientBurst:         50,
			PerIPRate:              200,
			PerIPBurst:             200,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 100,
		}),
		WithMessageBufferSize(2048),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			return d.Broadcast(m)
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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
		time.Sleep(3 * time.Millisecond)
	}

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for clientID := 0; clientID < clientCount; clientID++ {
			if len(received[clientID]) < totalBroadcasts {
				return false
			}
		}
		return true
	}, 2*time.Second, 10*time.Millisecond, "timed out waiting all clients to receive broadcasts")

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

func TestIntegration_LargeMessagesWithFailures(t *testing.T) {
	const payloadSize = 500 * 1024 // 500 KB
	const totalMessages = 5

	var successCount int32
	var errorCount int32

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          20,
			PerClientBurst:         20,
			PerIPRate:              40,
			PerIPBurst:             40,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 30,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			atomic.AddInt32(&successCount, 1)
			return nil
		}),
		OnError(func(err error, d Dispatcher, ctx *gsWebsocket.Context) error {
			atomic.AddInt32(&errorCount, 1)
			return nil
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	largePayload := make([]byte, payloadSize)
	for i := 0; i < payloadSize; i++ {
		largePayload[i] = byte(i % 256)
	}

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, largePayload))
		time.Sleep(2 * time.Millisecond)
	}

	// abruptly close connection to simulate failure
	ws.Close()

	// try to connect again and send more messages
	ws2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws2.Close()

	for i := 0; i < totalMessages; i++ {
		require.NoError(t, ws2.WriteMessage(websocket.BinaryMessage, largePayload))
		time.Sleep(time.Millisecond)
	}

	time.Sleep(time.Millisecond)

	require.Greater(t, atomic.LoadInt32(&successCount), int32(0))
}

func TestIntegration_ProgressiveMessageSizes(t *testing.T) {
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

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          10,
			PerClientBurst:         20,
			PerIPRate:              20,
			PerIPBurst:             40,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 50,
		}),
		WithMessageBufferSize(1024),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			atomic.AddInt32(&receivedCount, 1)
			mu.Lock()
			receivedSizes = append(receivedSizes, len(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

		timeout := time.Duration(size/1024) * time.Millisecond
		_ = ws.SetWriteDeadline(time.Now().Add(timeout))

		err := ws.WriteMessage(websocket.BinaryMessage, payload)
		require.NoError(t, err, "Failed to send message %d of size %d", i, size)

		delay := time.Duration(size/1024) * time.Millisecond
		time.Sleep(delay)
	}

	time.Sleep(time.Millisecond)

	require.Equal(t, int32(len(sizes)), atomic.LoadInt32(&receivedCount))

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, receivedSizes, len(sizes))
	for i, expectedSize := range sizes {
		require.Equal(t, expectedSize, receivedSizes[i], "Message %d has wrong size", i)
	}
}

func TestIntegration_MessageOrder(t *testing.T) {
	const totalMessages = 20

	var mu sync.Mutex
	var received []string

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			mu.Lock()
			received = append(received, string(m.RawData))
			mu.Unlock()
			return nil
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	for i := 0; i < totalMessages; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte(msg)))
	}

	time.Sleep(2 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, totalMessages, len(received))
	for i := 0; i < totalMessages; i++ {
		expected := fmt.Sprintf("msg-%d", i)
		require.Equal(t, expected, received[i])
	}
}

func TestIntegration_BroadcastMessageOrder(t *testing.T) {
	const totalMessages = 15

	var mu sync.Mutex
	received := make(map[string][]string)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			return d.Broadcast(m)
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

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

func TestIntegration_RateLimitingEnforced(t *testing.T) {
	const clientCount = 3
	const messagesPerClient = 50

	rlConfig := DefaultRateLimiterConfig()
	rlConfig.PerClientRate = 5
	rlConfig.PerClientBurst = 5
	rlConfig.PerIPRate = 10
	rlConfig.PerIPBurst = 10

	server := newTestServer(t,
		WithRateLimit(rlConfig),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			// echo msg
			return d.SendToClient(client.GetID(), m)
		}),
	)
	defer server.Close()

	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

// Test different message types (Text, Binary, Ping, Pong)
func TestIntegration_MessageTypes(t *testing.T) {
	var mu sync.Mutex
	received := make(map[int][]byte)

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			mu.Lock()
			received[int(m.Type)] = m.RawData
			mu.Unlock()
			return d.SendToClient(client.GetID(), m)
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, textMsg, received[websocket.TextMessage])
	require.Equal(t, binaryMsg, received[websocket.BinaryMessage])
}

// Test JSON message handling
func TestIntegration_JSONMessages(t *testing.T) {
	type TestMessage struct {
		Type string `json:"type"`
		Data string `json:"data"`
		ID   int    `json:"id"`
	}

	var mu sync.Mutex
	var received []TestMessage

	server := newTestServer(t,
		WithPath("/ws"),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			var msg TestMessage
			if err := json.Unmarshal(m.RawData, &msg); err != nil {
				return err
			}

			mu.Lock()
			received = append(received, msg)
			mu.Unlock()

			msg.Data = "processed: " + msg.Data
			response, _ := json.Marshal(msg)
			return d.SendToClient(client.GetID(), &Message{Type: m.Type, RawData: response})
		}),
	)
	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)
	mu.Lock()
	require.Len(t, received, 1)
	require.Equal(t, testMsg, received[0])
	mu.Unlock()
}

// Test error handling in callbacks
func TestIntegration_ErrorHandling(t *testing.T) {
	var errorsCaught int32

	server := newTestServer(t,
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Fail-Connect"}),
		OnBeforeConnect(func(r *http.Request, ctx *Context) error {
			if ctx.Headers()["Fail-Connect"] == "true" {
				return fmt.Errorf("connection rejected")
			}
			return nil
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			if string(m.RawData) == "error" {
				atomic.AddInt32(&errorsCaught, 1)
				return fmt.Errorf("message error")
			}
			return d.SendToClient(client.GetID(), m)
		}),
		OnError(func(err error, d Dispatcher, ctx *Context) error {
			atomic.AddInt32(&errorsCaught, 1)
			return nil
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

	headers := http.Header{"Fail-Connect": []string{"true"}}
	_, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("error")))
	time.Sleep(2 * time.Millisecond)

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("normal")))
	_, resp_msg, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, []byte("normal"), resp_msg)

	require.Greater(t, atomic.LoadInt32(&errorsCaught), int32(0))
}

// Test memory management under load
func TestIntegration_MemoryLeaks(t *testing.T) {
	const rounds = 5
	const clientsPerRound = 10
	const messagesPerClient = 20

	server := newTestServer(t,
		WithPath("/ws"),
		WithRateLimit(&RateLimiterConfig{
			PerClientRate:          10000,
			PerClientBurst:         10000,
			PerIPRate:              50000,
			PerIPBurst:             50000,
			CleanupInterval:        time.Minute,
			EntryTTL:               time.Minute,
			MaxRateLimitViolations: 10000,
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

		time.Sleep(2 * time.Millisecond)
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

func TestIntegration_CustomHeadersValidation(t *testing.T) {
	headersCh := make(chan map[string]string, 1)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Authorization", "User-Id", "Custom-Header"}),
		OnConnect(func(d Dispatcher, ctx *Context) error {
			headers := make(map[string]string)
			for _, header := range []string{"Authorization", "User-Id", "Custom-Header"} {
				headers[header] = ctx.Headers()[header]
			}
			headersCh <- headers
			return nil
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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
	case <-time.After(2 * time.Millisecond):
		t.Fatal("Timeout waiting for headers")
	}
}

func TestIntegration_MultipleClientsWithDifferentHeaders(t *testing.T) {
	type clientHeaders struct {
		clientID string
		headers  map[string]string
	}

	var mu sync.Mutex
	var allReceivedHeaders []clientHeaders

	server := newTestServer(t,
		WithPath("/ws"),
		WithRelevantHeaders([]string{"Client-Id", "Authorization", "User-Role"}),
		OnConnect(func(d Dispatcher, ctx *Context) error {
			clientID := ctx.Headers()["Client-Id"]
			headers := map[string]string{
				"Client-Id":     ctx.Headers()["Client-Id"],
				"Authorization": ctx.Headers()["Authorization"],
				"User-Role":     ctx.Headers()["User-Role"],
			}

			mu.Lock()
			allReceivedHeaders = append(allReceivedHeaders, clientHeaders{
				clientID: clientID,
				headers:  headers,
			})
			mu.Unlock()

			return nil
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

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

func TestIntegration_IgnoredHeaders(t *testing.T) {
	headersCh := make(chan map[string]string, 1)

	server := newTestServer(t,
		WithPath("/ws"),
		WithRelevantHeaders([]string{"User-Id"}),
		OnConnect(func(d Dispatcher, ctx *Context) error {
			headers := map[string]string{
				"User-Id":        ctx.Headers()["User-Id"],
				"Authorization":  ctx.Headers()["Authorization"],
				"Custom-Header":  ctx.Headers()["Custom-Header"],
				"Ignored-Header": ctx.Headers()["Ignored-Header"],
			}
			headersCh <- headers
			return nil
		}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for headers")
	}
}

// Test connection limits and cleanup
func TestIntegration_ConnectionLimits(t *testing.T) {
	const maxConnections = 5

	server := newTestServer(t,
		WithPath("/ws"),
		WithMaxConnections(ConnectionPoolConfig{MaxTotal: maxConnections, MaxPerIP: maxConnections}),
		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
			client, exists := ctx.Client()
			if !exists {
				return nil
			}
			return d.SendToClient(client.GetID(), m)
		}),
	)

	defer server.Close()
	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

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

	time.Sleep(2 * time.Millisecond)

	// should be able to connect again after cleanup
	ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer ws.Close()

	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("test-after-cleanup")))
	_, resp_msg, err := ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, []byte("test-after-cleanup"), resp_msg)
}

// func TestIntegration_AttachToCtx(t *testing.T) {
// 	const testKey ctxKey = "test_key"

// 	server := newTestServer(t,
// 		WithPath("/ws"),
// 		WithMiddleware(func(h http.Handler) http.Handler {
// 			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 				ctx := context.WithValue(r.Context(), testKey, "test_val")
// 				h.ServeHTTP(w, r.WithContext(ctx))
// 			})
// 		}),
// 		OnMessage(func(m *Message, d Dispatcher, ctx *Context) error {
// 			client, exists := ctx.Client()
// 			if !exists {
// 				return nil
// 			}
// 			require.Equal(t, "test_val", ctx.Context().Value(testKey))
// 			return d.SendToClient(client.GetID(), m)
// 		}),
// 	)

// 	defer server.Close()
// 	u := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/ws"}

// 	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
// 	require.NoError(t, err)
// 	defer ws.Close()

// 	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("test")))
// 	_, resp_msg, err := ws.ReadMessage()
// 	require.NoError(t, err)
// 	require.Equal(t, []byte("test"), resp_msg)
// }
