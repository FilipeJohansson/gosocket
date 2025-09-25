package gosocket

import (
	"context"
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
	Clients    *SharedCollection[*Client, string]
	Rooms      *SharedCollection[*Room, string]
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan *Message
	running    bool
}

func NewMockHub() *MockHub {
	return &MockHub{
		Clients: NewSharedCollection[*Client, string](),
		Rooms:   NewSharedCollection[*Room, string](),
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
	m.Clients.Add(client, client.ID)
}

func (m *MockHub) RemoveClient(client *Client) {
	m.Called(client)
}

func (m *MockHub) SendToClient(fromClientId, toClientId string, message *Message) error {
	m.Called(fromClientId, toClientId, message)
	if fromClientId == "" || toClientId == "" {
		return ErrInvalidClientId
	} else if message == nil || (message.RawData == nil && message.Data == nil) {
		return ErrNoDataToSend
	}

	fromClient := m.GetClient(fromClientId)
	if fromClient == nil {
		m.Log(LogTypeClient, LogLevelError, "SendToClient: From client not found: %s", fromClientId)
		return ErrClientNotFound
	}

	fromClient.mu.Lock()
	if fromClient.Conn == nil {
		fromClient.mu.Unlock()
		fromClient.Hub.Log(LogTypeClient, LogLevelError, "SendToClient (ID: %s): Connection is nil", fromClient.ID)
		return ErrClientConnNil
	}
	fromClient.mu.Unlock()

	var toClient *Client
	if fromClientId == toClientId {
		toClient = fromClient
	} else {
		toClient := m.GetClient(toClientId)
		if toClient == nil {
			m.Log(LogTypeClient, LogLevelError, "SendToClient: To client not found: %s", toClientId)
			return ErrClientNotFound
		}

		toClient.mu.Lock()
		if toClient.Conn == nil {
			toClient.mu.Unlock()
			toClient.Hub.Log(LogTypeClient, LogLevelError, "SendToClient (ID: %s): Connection is nil", toClient.ID)
			return ErrClientConnNil
		}
		toClient.mu.Unlock()
	}

	message.From = fromClientId
	message.To = toClientId
	if message.Created.IsZero() {
		message.Created = time.Now()
	}

	m.Log(LogTypeMessage, LogLevelDebug, "Sending message from %s to %s", fromClientId, toClientId)

	select {
	case toClient.MessageChan <- message:
		m.Log(LogTypeMessage, LogLevelDebug, "Message sent to client %s channel", toClientId)
		return nil
	default:
		m.Log(LogTypeClient, LogLevelError, "SendToClient: Client %s channel is full", toClientId)
		safeGoroutine("Client.Disconnect", func() {
			_ = toClient.Disconnect()
		})
		return ErrClientFull
	}
}

func (m *MockHub) BroadcastMessage(message *Message) {
	m.Called(message)
}

func (m *MockHub) BroadcastToRoom(roomId string, message *Message) error {
	m.Called(roomId, message)
	return nil
}

func (m *MockHub) CreateRoom(ownerId, roomName string, customId ...string) (*Room, error) {
	m.Called(ownerId, roomName)
	if roomName == "" {
		return nil, ErrRoomNameEmpty
	}

	roomId := roomName
	if len(customId) > 0 && customId[0] != "" {
		roomId = customId[0]
	}

	if _, exists := m.Rooms.Get(roomId); exists {
		return nil, ErrRoomAlreadyExists
	}

	room := NewRoom(roomId, ownerId, roomName)
	m.Rooms.Add(room, roomId)

	m.Log(LogTypeRoom, LogLevelInfo, "Room created: %s", roomName)
	return room, nil
}

func (m *MockHub) JoinRoom(client *Client, roomId string) error {
	m.Called(client, roomId)
	room, exists := m.Rooms.Get(roomId)
	if !exists {
		m.Log(LogTypeRoom, LogLevelError, "Room not found: %s", roomId)
		return newRoomNotFoundError(roomId)
	}

	room.AddClient(client)
	m.Log(LogTypeRoom, LogLevelInfo, "Room %s: client %s joined", roomId, client.ID)
	return nil
}

func (m *MockHub) LeaveRoom(client *Client, roomId string) {
	m.Called(client, roomId)
	if room, exists := m.Rooms.Get(roomId); exists {
		if room.RemoveClient(client.ID) {
			m.Log(LogTypeRoom, LogLevelInfo, "Room %s: client %s left", roomId, client.ID)
		}
	}
}

func (m *MockHub) DeleteRoom(roomId string) error {
	m.Called(roomId)
	err := m.LeaveAllFromRoom(roomId)
	if err != nil {
		return err
	}

	if m.Rooms.Remove(roomId) {
		m.Log(LogTypeRoom, LogLevelInfo, "Room deleted: %s", roomId)
	}
	return nil
}

func (m *MockHub) DeleteEmptyRooms() []string {
	m.Called()
	var deletedRooms []string

	m.Rooms.ForEach(func(roomId string, room *Room) {
		if room.IsEmpty() {
			if err := m.DeleteRoom(roomId); err == nil {
				deletedRooms = append(deletedRooms, roomId)
			}
		}
	})

	m.Log(LogTypeRoom, LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

func (m *MockHub) DeleteEmptyRoomsExcluding(excludeIds []string) []string {
	m.Called(excludeIds)
	var deletedRooms []string

	m.Rooms.ForEach(func(roomId string, room *Room) {
		excluded := false
		for _, id := range excludeIds {
			if roomId == id {
				excluded = true
				break
			}
		}

		if !excluded && room.IsEmpty() {
			if err := m.DeleteRoom(roomId); err == nil {
				deletedRooms = append(deletedRooms, roomId)
			}
		}
	})

	m.Log(LogTypeRoom, LogLevelDebug, "Deleted %d empty rooms", len(deletedRooms))
	return deletedRooms
}

func (m *MockHub) LeaveAllFromRoom(roomId string) error {
	m.Called(roomId)
	room, exists := m.Rooms.Get(roomId)
	if !exists {
		return newRoomNotFoundError(roomId)
	}

	var clientsRemoved []string
	for id := range room.Clients() {
		if room.RemoveClient(id) {
			clientsRemoved = append(clientsRemoved, id)
		}
	}

	m.Log(LogTypeRoom, LogLevelDebug, "Removed %d clients from room: %s", len(clientsRemoved), roomId)
	m.Log(LogTypeRoom, LogLevelInfo, "Room %s: all clients left", roomId)
	return nil
}

func (m *MockHub) GetClientsInRoom(roomId string) map[string]*Client {
	m.Called(roomId)
	room, exists := m.Rooms.Get(roomId)
	if !exists {
		return map[string]*Client{}
	}
	return room.Clients()
}

func (m *MockHub) GetStats() *Stats {
	args := m.Called()
	return args.Get(0).(*Stats)
}

func (m *MockHub) GetClients() map[string]*Client {
	m.Called()
	return m.Clients.GetAll()
}

func (m *MockHub) GetClient(id string) *Client {
	m.Called()
	client, exists := m.Clients.Get(id)
	if !exists {
		return nil
	}
	return client
}

func (m *MockHub) GetRooms() map[string]*Room {
	m.Called()
	return m.Rooms.GetAll()
}

func (m *MockHub) GetRoom(roomId string) (*Room, error) {
	m.Called(roomId)
	room, exists := m.Rooms.Get(roomId)
	if !exists {
		return nil, newRoomNotFoundError(roomId)
	}
	return room, nil
}

func (m *MockHub) IsRunning() bool {
	m.Called()
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
		message         *Message
		isExpectedError bool
		expectedError   error
	}{
		{
			name: "sends message successfully",
			setupClient: func() *Client {
				mockHub := NewMockHub()

				mockHub.On("AddClient", mock.Anything)
				mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
				mockHub.On("GetClient", mock.Anything)
				mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

				client := NewClient("test", &MockWebSocketConn{}, mockHub, 256)
				mockHub.AddClient(client)
				return client
			},
			message:         &Message{Type: BinaryMessage, Data: []byte("test message"), RawData: []byte("test message"), Encoding: Raw, Created: time.Now()},
			isExpectedError: false,
		},
		{
			name: "fails when connection is nil",
			setupClient: func() *Client {
				mockHub := NewMockHub()

				mockHub.On("AddClient", mock.Anything)
				mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
				mockHub.On("GetClient", mock.Anything)
				mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

				client := NewClient("test", nil, mockHub, 256)
				mockHub.AddClient(client)
				return client
			},
			message:         NewRawMessage(BinaryMessage, []byte("test message")),
			isExpectedError: true,
			expectedError:   ErrClientConnNil,
		},
		{
			name: "fails when message channel is full",
			setupClient: func() *Client {
				mockHub := NewMockHub()

				mockHub.On("AddClient", mock.Anything)
				mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
				mockHub.On("GetClient", mock.Anything)
				mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

				client := NewClient("test", &MockWebSocketConn{}, mockHub, 256)
				mockHub.AddClient(client)

				// Fill the channel to capacity
				for i := 0; i < cap(client.MessageChan); i++ {
					client.MessageChan <- NewRawMessage(BinaryMessage, []byte("fill"))
				}

				return client
			},
			message:         NewRawMessage(BinaryMessage, []byte("test message")),
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
			mockHub := NewMockHub()

			mockHub.On("AddClient", mock.Anything)
			mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
			mockHub.On("GetClient", mock.Anything)
			mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

			client := NewClient("test", &MockWebSocketConn{}, mockHub, 256)
			mockHub.AddClient(client)

			err := client.Send(tt.message)

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHub := NewMockHub()

			mockHub.On("AddClient", mock.Anything)
			mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
			mockHub.On("GetClient", mock.Anything)
			mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

			client := NewClient("test", &MockWebSocketConn{}, mockHub, 256)
			mockHub.AddClient(client)
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
				_, _ = mockHub.CreateRoom(client.ID, "test-room")
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
			name: "returns empty slice when client has no rooms",
			setupClient: func() *Client {
				hub := NewHub(DefaultLoggerConfig())
				client := NewClient("test", &MockWebSocketConn{}, hub, 256)

				// Manually add client to rooms for testing
				_ = hub.JoinRoom(client, "room1")
				_ = hub.JoinRoom(client, "room2")
				_, _ = hub.CreateRoom("__SERVER__", "room3")

				return client
			},
			expected: []string{},
		},
		{
			name: "returns rooms client is in",
			setupClient: func() *Client {
				hub := NewHub(DefaultLoggerConfig())
				client := NewClient("test", &MockWebSocketConn{}, hub, 256)

				// Manually add client to rooms for testing
				_, _ = hub.CreateRoom("__SERVER__", "room1")
				_, _ = hub.CreateRoom("__SERVER__", "room2")
				_ = hub.JoinRoom(client, "room1")
				_ = hub.JoinRoom(client, "room2")
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
	mockHub := NewMockHub()

	mockHub.On("AddClient", mock.Anything)
	mockHub.On("SendToClient", mock.Anything, mock.Anything, mock.Anything)
	mockHub.On("GetClient", mock.Anything)
	mockHub.On("Log", mock.Anything, mock.Anything, mock.Anything, mock.Anything)

	client := NewClient("test", &MockWebSocketConn{}, mockHub, 256)
	mockHub.AddClient(client)

	// Verify channel capacity
	assert.Equal(t, 256, cap(client.MessageChan))

	// Fill channel to capacity - 1
	for i := 0; i < 255; i++ {
		select {
		case client.MessageChan <- NewRawMessage(BinaryMessage, []byte(fmt.Sprintf("message_%d", i))):
		default:
			t.Fatalf("Channel should not be full at message %d", i)
		}
	}

	// One more should still work
	err := client.Send(&Message{Data: []byte("last_message")})
	assert.NoError(t, err)

	// Now it should be full
	err = client.Send(&Message{Data: []byte("overflow_message")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client message channel is full")
}
