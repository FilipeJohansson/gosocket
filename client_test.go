package gosocket

import (
	"context"
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
	Clients    *SharedCollection[*Client]
	Rooms      *SharedCollection[*Room]
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
	mu         sync.RWMutex
	running    bool
}

func NewMockHub() *MockHub {
	return &MockHub{
		Clients: NewSharedCollection[*Client](),
		Rooms:   NewSharedCollection[*Room](),
	}
}

func (m *MockHub) Run(ctx context.Context) {
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

func (m *MockHub) BroadcastToRoom(roomName string, message *Message) {
	m.Called(roomName, message)
}

func (m *MockHub) CreateRoom(ownerId, roomName string) (*Room, error) {
	m.Called(ownerId, roomName)
	if roomName == "" {
		return nil, ErrRoomNameEmpty
	}

	if _, exists := m.Rooms.GetByStringId(roomName); exists {
		return nil, ErrRoomAlreadyExists
	}

	room := NewRoom(ownerId, roomName)
	m.mu.Lock()
	m.Rooms.AddWithStringId(room, roomName)
	m.mu.Unlock()
	m.Log(LogTypeOther, LogLevelDebug, "Room created: %s", roomName)

	return room, nil
}

func (m *MockHub) JoinRoom(client *Client, roomName string) {
	m.Called(client, roomName)
	if _, exists := m.Rooms.GetByStringId(roomName); !exists {
		_, err := m.CreateRoom(client.ID, roomName)
		if err != nil {
			m.Log(LogTypeOther, LogLevelError, "Error creating room: %s", err)
			return
		}
	}

	room, found := m.Rooms.GetByStringId(roomName)
	if !found {
		m.Log(LogTypeOther, LogLevelError, "Room not found: %s", roomName)
		return
	}

	room.Clients.AddWithStringId(client, client.ID)
	m.Log(LogTypeOther, LogLevelDebug, "Client %s joined room: %s", client.ID, roomName)
}

func (m *MockHub) LeaveRoom(client *Client, roomName string) {
	m.Called(client, roomName)
	if room, exists := m.Rooms.GetByStringId(roomName); exists {
		if room.Clients.RemoveByStringId(client.ID) {
			m.Log(LogTypeOther, LogLevelDebug, "Client %s left room: %s", client.ID, roomName)

			// remove room if empty
			if room.Clients.Len() == 0 {
				m.Log(LogTypeOther, LogLevelDebug, "Room %s is empty, removing it", roomName)
				err := m.DeleteRoom(roomName)
				if err != nil {
					m.Log(LogTypeOther, LogLevelError, "Error deleting room: %s", err)
				}
			}
		}
	}
}

func (m *MockHub) GetRoomClients(roomName string) []*Client {
	m.Called(roomName)
	room, exists := m.Rooms.GetByStringId(roomName)
	if !exists {
		return []*Client{}
	}

	return room.Clients.GetAll()
}

func (m *MockHub) GetStats() map[string]interface{} {
	args := m.Called()
	return args.Get(0).(map[string]interface{})
}

func (m *MockHub) GetClients() []*Client {
	m.Called()
	return m.Clients.GetAll()
}

func (m *MockHub) GetRooms() []*Room {
	m.Called()
	return m.Rooms.GetAll()
}

func (m *MockHub) GetRoom(roomName string) *Room {
	m.Called(roomName)
	room, exists := m.Rooms.GetByStringId(roomName)
	if !exists {
		return nil
	}
	return room
}

func (m *MockHub) DeleteRoom(roomName string) error {
	m.Called(roomName)
	room, exists := m.Rooms.GetByStringId(roomName)
	if !exists {
		return newRoomNotFoundError(roomName)
	}

	var clientsToRemove []*Client
	room.Clients.ForEach(func(id uint64, client *Client) {
		clientsToRemove = append(clientsToRemove, client)
	})

	for _, client := range clientsToRemove {
		room.Clients.RemoveByStringId(client.ID)
		m.Log(LogTypeOther, LogLevelDebug, "Client %s left room: %s", client.ID, roomName)
	}

	m.Rooms.RemoveByStringId(roomName)
	m.Log(LogTypeOther, LogLevelDebug, "Room deleted: %s", roomName)
	return nil
}

func (m *MockHub) IsRunning() bool {
	return m.running
}

func (m *MockHub) Log(logType LogType, level LogLevel, format string, args ...interface{}) {
	m.Called(logType, level, format, args)
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
			hub:  NewHub(DefaultLoggerConfig()),
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
			hub:  NewHub(DefaultLoggerConfig()),
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
			client := NewClient(tt.id, tt.conn, tt.hub, 256)
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
				return NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
			},
			message:         []byte("test message"),
			isExpectedError: false,
		},
		{
			name: "fails when connection is nil",
			setupClient: func() *Client {
				return NewClient("test", nil, NewHub(DefaultLoggerConfig()), 256)
			},
			message:         []byte("test message"),
			isExpectedError: true,
			expectedError:   ErrClientConnNil,
		},
		{
			name: "fails when message channel is full",
			setupClient: func() *Client {
				client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
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
			client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
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
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)

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
			client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
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
			client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
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
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)
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
				mockHub.On("CreateRoom", mock.AnythingOfType("string"), "test-room")
				mockHub.On("Log", mock.AnythingOfType("LogType"), mock.AnythingOfType("LogLevel"), mock.AnythingOfType("string"), mock.AnythingOfType("[]interface {}"))
				client := NewClient("test", &MockWebSocketConn{}, nil, 256)
				client.Hub = mockHub // Type assertion bypass for testing
				return client
			},
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails when hub is nil",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, nil, 256)
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
				client := NewClient("test", &MockWebSocketConn{}, nil, 256)
				client.Hub = mockHub // Type assertion bypass for testing
				return client
			},
			room:            "test-room",
			isExpectedError: false,
		},
		{
			name: "fails when hub is nil",
			setupClient: func() *Client {
				return NewClient("test", &MockWebSocketConn{}, nil, 256)
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
				return NewClient("test", &MockWebSocketConn{}, nil, 256)
			},
			expected: []string{},
		},
		{
			name: "returns rooms client is in",
			setupClient: func() *Client {
				hub := NewHub(DefaultLoggerConfig())
				client := NewClient("test", &MockWebSocketConn{}, hub, 256)

				// Manually add client to rooms for testing
				hub.JoinRoom(client, "room1")
				hub.JoinRoom(client, "room2")
				_, _ = hub.CreateRoom("__SERVER__", "room3")

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

				client := NewClient("test", mockConn, nil, 256)
				client.Hub = mockHub

				return client, mockConn, mockHub
			},
		},
		{
			name: "handles nil hub gracefully",
			setupClient: func() (*Client, *MockWebSocketConn, *MockHub) {
				mockConn := &MockWebSocketConn{}
				mockConn.On("Close").Return(nil)

				client := NewClient("test", mockConn, nil, 256)

				return client, mockConn, nil
			},
		},
		{
			name: "handles nil connection gracefully",
			setupClient: func() (*Client, *MockWebSocketConn, *MockHub) {
				mockHub := NewMockHub()
				mockHub.On("RemoveClient", mock.AnythingOfType("*gosocket.Client"))

				client := NewClient("test", nil, nil, 256)
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
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)

	client.SetUserData("username", "john_doe")
	client.SetUserData("age", 30)
	client.SetUserData("active", true)

	assert.Equal(t, "john_doe", client.UserData["username"])
	assert.Equal(t, 30, client.UserData["age"])
	assert.Equal(t, true, client.UserData["active"])
}

func TestClient_GetUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)

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
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)

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
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Test timed out - possible deadlock")
	}
}

func TestClient_MessageChannelCapacity(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, NewHub(DefaultLoggerConfig()), 256)

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
