package gosocket

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockWebSocketConn struct {
	mock.Mock
	closed bool
}

func (m *MockWebSocketConn) Close() error {
	args := m.Called()
	m.closed = true
	return args.Error(0)
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	args := m.Called(messageType, data)
	return args.Error(0)
}

func (m *MockWebSocketConn) ReadMessage() (messageType int, p []byte, err error) {
	args := m.Called()
	return args.Int(0), args.Get(1).([]byte), args.Error(2)
}

// MockHub implements a mock Hub for testing
type MockHub struct {
	mock.Mock
	Clients    map[*Client]bool
	Rooms      map[string]map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
	mu         sync.RWMutex
	running    bool
}

func NewMockHub() *MockHub {
	return &MockHub{
		Rooms: make(map[string]map[*Client]bool),
	}
}

func (m *MockHub) Run() {
	m.Called()
}

func (m *MockHub) Stop() {
	m.Called()
}

func (m *MockHub) AddClient(client *Client) {
	m.Called(client)
}

func (m *MockHub) RemoveClient(client *Client) {
	m.Called(client)
}

func (m *MockHub) BroadcastMessage(message *Message) {
	m.Called(message)
}

func (m *MockHub) BroadcastToRoom(room string, message *Message) {
	m.Called(room, message)
}

func (m *MockHub) CreateRoom(name string) error {
	m.Called(name)
	if name == "" {
		return ErrRoomNameEmpty
	}

	m.mu.Lock()
	if m.Rooms[name] == nil {
		m.Rooms[name] = make(map[*Client]bool)
	}
	m.mu.Unlock()

	return nil
}

func (m *MockHub) JoinRoom(client *Client, room string) {
	m.Called(client, room)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Rooms[room] == nil {
		m.Rooms[room] = make(map[*Client]bool)
	}
	m.Rooms[room][client] = true
}

func (m *MockHub) LeaveRoom(client *Client, room string) {
	m.Called(client, room)
	m.mu.Lock()
	defer m.mu.Unlock()
	if roomClients, exists := m.Rooms[room]; exists {
		delete(roomClients, client)
		if len(roomClients) == 0 {
			delete(m.Rooms, room)
		}
	}
}

func (m *MockHub) GetRoomClients(room string) []*Client {
	m.Called(room)
	m.mu.RLock()
	defer m.mu.RUnlock()
	clients := []*Client{}
	if roomClients, exists := m.Rooms[room]; exists {
		for client := range roomClients {
			clients = append(clients, client)
		}
	}
	return clients
}

func (m *MockHub) GetStats() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

func (m *MockHub) GetClients() map[*Client]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	clientsCopy := make(map[*Client]bool)
	for client, value := range m.Clients {
		clientsCopy[client] = value
	}
	return clientsCopy
}

func (m *MockHub) GetRooms() map[string]map[*Client]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	roomsCopy := make(map[string]map[*Client]bool)
	for room, clients := range m.Rooms {
		clientsCopy := make(map[*Client]bool)
		for client, value := range clients {
			clientsCopy[client] = value
		}
		roomsCopy[room] = clientsCopy
	}
	return roomsCopy
}

func (m *MockHub) DeleteRoom(name string) error {
	m.Called(name)
	m.mu.Lock()
	defer m.mu.Unlock()

	if roomClients, exists := m.Rooms[name]; exists {
		// first, remove all clients from the room
		for client := range roomClients {
			delete(roomClients, client)
		}
		delete(m.Rooms, name)
		return nil
	}

	return fmt.Errorf("room not found: %s", name)
}

func (m *MockHub) IsRunning() bool {
	return m.running
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		conn     IWebSocketConn
		hub      *Hub
		expected func(*Client)
	}{
		{
			name: "creates client with valid parameters",
			id:   "test-client-1",
			conn: &MockWebSocketConn{},
			hub:  NewHub(),
			expected: func(c *Client) {
				assert.Equal(t, "test-client-1", c.ID)
				assert.NotNil(t, c.Conn)
				assert.NotNil(t, c.Hub)
				assert.NotNil(t, c.MessageChan)
				assert.NotNil(t, c.UserData)
				assert.Equal(t, 256, cap(c.MessageChan))
			},
		},
		{
			name: "creates client with nil connection",
			id:   "test-client-2",
			conn: nil,
			hub:  NewHub(),
			expected: func(c *Client) {
				assert.Equal(t, "test-client-2", c.ID)
				assert.Nil(t, c.Conn)
				assert.NotNil(t, c.Hub)
			},
		},
		{
			name: "creates client with nil hub",
			id:   "test-client-3",
			conn: &MockWebSocketConn{},
			hub:  nil,
			expected: func(c *Client) {
				assert.Equal(t, "test-client-3", c.ID)
				assert.NotNil(t, c.Conn)
				assert.Nil(t, c.Hub)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.id, tt.conn, tt.hub)
			tt.expected(client)
		})
	}
}

func TestClient_Send(t *testing.T) {
	tests := []struct {
		name            string
		setupClient     func() *Client
		message         []byte
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "sends message successfully",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, NewHub())
			},
			message:         []byte("test message"),
			isExpectedError: false,
		},
		{
			name: "fails when connection is nil",
			setupClient: func() *Client {
				return NewClient("test", nil, NewHub())
			},
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrClientConnNil,
		},
		{
			name: "fails when message channel is full",
			setupClient: func() *Client {
				client := NewClient("test", &MockWebSocketConn{}, NewHub())
				// Fill the channel to capacity
				for i := 0; i < cap(client.MessageChan); i++ {
					client.MessageChan <- []byte("fill")
				}
				return client
			},
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrClientFull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			err := client.Send(tt.message)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				// Verify message was sent to channel
				select {
				case msg := <-client.MessageChan:
					assert.Equal(t, tt.message, msg)
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected message in channel")
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_SendMessage(t *testing.T) {
	tests := []struct {
		name            string
		message         *Message
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "sends message with raw data",
			message: &Message{
				RawData: []byte("raw data"),
			},
			isExpectedError: false,
		},
		{
			name: "sends JSON message",
			message: &Message{
				Data:     map[string]string{"key": "value"},
				Encoding: JSON,
			},
			isExpectedError: false,
		},
		{
			name: "sends JSON message with default encoding",
			message: &Message{
				Data: map[string]string{"key": "value"},
			},
			isExpectedError: false,
		},
		{
			name: "sends raw message with byte data",
			message: &Message{
				Data:     []byte("raw bytes"),
				Encoding: Raw,
			},
			isExpectedError: false,
		},
		{
			name: "fails with raw encoding and non-byte data",
			message: &Message{
				Data:     "string data",
				Encoding: Raw,
			},
			isExpectedError: true,
			expectedError:   ErrRawEncoding,
		},
		{
			name: "fails with unsupported encoding",
			message: &Message{
				Data:     "test data",
				Encoding: EncodingType(999),
			},
			isExpectedError: true,
			expectedError:   newUnsupportedEncodingError(EncodingType(999)),
		},
		{
			name: "fails with no data",
			message: &Message{
				Type: TextMessage,
			},
			isExpectedError: true,
			expectedError:   ErrNoDataToSend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("test", &MockWebSocketConn{}, NewHub())
			err := client.SendMessage(tt.message)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				// Verify message was sent to channel
				select {
				case <-client.MessageChan:
					// Message successfully queued
				case <-time.After(100 * time.Millisecond):
					t.Fatal("Expected message in channel")
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_SendData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())

	testData := map[string]interface{}{
		"message": "hello",
		"count":   42,
	}

	err := client.SendData(testData)
	assert.NoError(t, err)

	// Verify JSON data was sent
	select {
	case msg := <-client.MessageChan:
		var result map[string]interface{}
		err := json.Unmarshal(msg, &result)
		assert.NoError(t, err)
		assert.Equal(t, "hello", result["message"])
		assert.Equal(t, float64(42), result["count"]) // JSON unmarshals numbers as float64
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected message in channel")
	}
}

func TestClient_SendDataWithEncoding(t *testing.T) {
	tests := []struct {
		name            string
		data            interface{}
		encoding        EncodingType
		isExpectedError bool
		expectedError   error
	}{
		{
			name:            "sends JSON data",
			data:            map[string]string{"key": "value"},
			encoding:        JSON,
			isExpectedError: false,
		},
		{
			name:            "sends raw byte data",
			data:            []byte("raw data"),
			encoding:        Raw,
			isExpectedError: false,
		},
		{
			name:            "fails with raw encoding and non-byte data",
			data:            "string data",
			encoding:        Raw,
			isExpectedError: true,
			expectedError:   ErrRawEncoding,
		},
		{
			name:            "fails with unsupported encoding",
			data:            "test data",
			encoding:        EncodingType(999),
			isExpectedError: true,
			expectedError:   newUnsupportedEncodingError(EncodingType(999)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("test", &MockWebSocketConn{}, NewHub())
			err := client.SendDataWithEncoding(tt.data, tt.encoding)

			if !tt.isExpectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_SendJSON(t *testing.T) {
	tests := []struct {
		name            string
		data            interface{}
		isExpectedError bool
		expectedError   error
	}{
		{
			name:            "sends valid JSON data",
			data:            map[string]string{"key": "value"},
			isExpectedError: false,
		},
		{
			name:            "fails with invalid JSON data",
			data:            make(chan int), // channels can't be marshaled to JSON
			isExpectedError: true,
			expectedError:   ErrSerializeData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient("test", &MockWebSocketConn{}, NewHub())
			err := client.SendJSON(tt.data)

			if !tt.isExpectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_SendProtobuf(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())
	err := client.SendProtobuf("test data")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "protobuf serialization not yet implemented")
}

func TestClient_JoinRoom(t *testing.T) {
	tests := []struct {
		name            string
		setupClient     func() *Client
		room            string
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "joins room successfully",
			setupClient: func() *Client {
				mockHub := NewMockHub()
				mockHub.On("JoinRoom", mock.AnythingOfType("*gosocket.Client"), "test-room")
				client := NewClient("test", &MockWebSocketConn{}, nil)
				client.Hub = mockHub // Type assertion bypass for testing
				return client
			},
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails when hub is nil",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, nil)
			},
			room:            "test-room",
			isExpectedError: true,
			expectedError:   ErrHubIsNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			err := client.JoinRoom(tt.room)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := client.Hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_LeaveRoom(t *testing.T) {
	tests := []struct {
		name            string
		setupClient     func() *Client
		room            string
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "leaves room successfully",
			setupClient: func() *Client {
				mockHub := NewMockHub()
				mockHub.On("LeaveRoom", mock.AnythingOfType("*gosocket.Client"), "test-room")
				client := NewClient("test", &MockWebSocketConn{}, nil)
				client.Hub = mockHub // Type assertion bypass for testing
				return client
			},
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails when hub is nil",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, nil)
			},
			room:            "test-room",
			isExpectedError: true,
			expectedError:   ErrHubIsNil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			err := client.LeaveRoom(tt.room)

			if !tt.isExpectedError {
				assert.NoError(t, err)
				if mockHub, ok := client.Hub.(*MockHub); ok {
					mockHub.AssertExpectations(t)
				}
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			}
		})
	}
}

func TestClient_GetRooms(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() *Client
		expected    []string
	}{
		{
			name: "returns empty slice when hub is nil",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, nil)
			},
			expected: []string{},
		},
		{
			name: "returns rooms client is in",
			setupClient: func() *Client {
				hub := NewHub()
				client := NewClient("test", &MockWebSocketConn{}, hub)

				// Manually add client to rooms for testing
				hub.mu.Lock()
				hub.Rooms["room1"] = map[*Client]bool{client: true}
				hub.Rooms["room2"] = map[*Client]bool{client: true}
				hub.Rooms["room3"] = map[*Client]bool{} // Client not in this room
				hub.mu.Unlock()

				return client
			},
			expected: []string{"room1", "room2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			rooms := client.GetRooms()

			assert.ElementsMatch(t, tt.expected, rooms)
		})
	}
}

func TestClient_Disconnect(t *testing.T) {
	tests := []struct {
		name        string
		setupClient func() (*Client, *MockWebSocketConn, *MockHub)
	}{
		{
			name: "disconnects successfully with hub and connection",
			setupClient: func() (*Client, *MockWebSocketConn, *MockHub) {
				mockConn := &MockWebSocketConn{}
				mockHub := NewMockHub()

				mockHub.On("RemoveClient", mock.AnythingOfType("*gosocket.Client"))
				mockConn.On("Close").Return(nil)

				client := NewClient("test", mockConn, nil)
				client.Hub = mockHub

				return client, mockConn, mockHub
			},
		},
		{
			name: "handles nil hub gracefully",
			setupClient: func() (*Client, *MockWebSocketConn, *MockHub) {
				mockConn := &MockWebSocketConn{}
				mockConn.On("Close").Return(nil)

				client := NewClient("test", mockConn, nil)

				return client, mockConn, nil
			},
		},
		{
			name: "handles nil connection gracefully",
			setupClient: func() (*Client, *MockWebSocketConn, *MockHub) {
				mockHub := NewMockHub()
				mockHub.On("RemoveClient", mock.AnythingOfType("*gosocket.Client"))

				client := NewClient("test", nil, nil)
				client.Hub = mockHub

				return client, nil, mockHub
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, mockConn, mockHub := tt.setupClient()
			err := client.Disconnect()

			assert.NoError(t, err)

			if mockHub != nil {
				mockHub.AssertExpectations(t)
			}

			if mockConn != nil {
				mockConn.AssertExpectations(t)
			}
		})
	}
}

func TestClient_SetUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())

	client.SetUserData("username", "john_doe")
	client.SetUserData("age", 30)
	client.SetUserData("active", true)

	assert.Equal(t, "john_doe", client.UserData["username"])
	assert.Equal(t, 30, client.UserData["age"])
	assert.Equal(t, true, client.UserData["active"])
}

func TestClient_GetUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())

	// Set some test data
	client.UserData["username"] = "john_doe"
	client.UserData["age"] = 30

	tests := []struct {
		key      string
		expected interface{}
	}{
		{"username", "john_doe"},
		{"age", 30},
		{"nonexistent", nil},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := client.GetUserData(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_ConcurrentAccess(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())

	// Test concurrent access to UserData
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	wg.Add(numGoroutines * 2) // writers and readers

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				client.SetUserData(key, value)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				client.GetUserData(key) // May return nil if not set yet
			}
		}(i)
	}

	// Wait for all goroutines to complete
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Test completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

func TestClient_MessageChannelCapacity(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub())

	// Verify channel capacity
	assert.Equal(t, 256, cap(client.MessageChan))

	// Fill channel to capacity - 1
	for i := 0; i < 255; i++ {
		select {
		case client.MessageChan <- []byte(fmt.Sprintf("message_%d", i)):
		default:
			t.Fatalf("Channel should not be full at message %d", i)
		}
	}

	// One more should still work
	err := client.Send([]byte("last_message"))
	assert.NoError(t, err)

	// Now it should be full
	err = client.Send([]byte("overflow_message"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client message channel is full")
}
