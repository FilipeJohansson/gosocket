package hub

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockWebSocketConn struct {
	mock.Mock
	closed bool
}

func (m *MockWebSocketConn) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			// If no expectation was set on the mock, recover and return nil
			err = nil
		}
	}()

	args := m.Called()
	m.closed = true
	err = args.Error(0)
	return
}

func (m *MockWebSocketConn) WriteMessage(messageType int, data []byte) error {
	args := m.Called(messageType, data)
	return args.Error(0)
}

func (m *MockWebSocketConn) ReadMessage() (messageType int, p []byte, err error) {
	args := m.Called()
	return args.Int(0), args.Get(1).([]byte), args.Error(2)
}

func TestClient_NewClient(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		conn     WSConn
		connInfo *ConnectionInfo
		expected func(*Client)
	}{
		{
			name: "creates client with valid parameters",
			id:   "test-client-1",
			conn: &MockWebSocketConn{},
			connInfo: &ConnectionInfo{
				ClientIP:  "127.0.0.1",
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/237.84.2.178 Safari/537.36",
				Origin:    "http://localhost:8080",
				Headers:   map[string]string{"header1": "value1", "header2": "value2"},
				RequestID: "test-request-id",
			},
			expected: func(c *Client) {
				assert.Equal(t, "test-client-1", c.id)
				assert.NotNil(t, c.Conn)
				assert.NotNil(t, c.SendChan)
				assert.NotNil(t, c.userData)
				assert.NotNil(t, c.connInfo)
				assert.Equal(t, 256, cap(c.SendChan))
			},
		},
		{
			name: "creates client with nil connection",
			id:   "test-client-2",
			conn: nil,
			connInfo: &ConnectionInfo{
				ClientIP:  "127.0.0.1",
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/237.84.2.178 Safari/537.36",
				Origin:    "http://localhost:8080",
				Headers:   map[string]string{"header1": "value1", "header2": "value2"},
				RequestID: "test-request-id",
			},
			expected: func(c *Client) {
				assert.Equal(t, "test-client-2", c.id)
				assert.Nil(t, c.Conn)
			},
		},
		{
			name:     "creates client with nil connInfo",
			id:       "test-client-3",
			conn:     &MockWebSocketConn{},
			connInfo: nil,
			expected: func(c *Client) {
				assert.Equal(t, "test-client-3", c.id)
				assert.NotNil(t, c.Conn)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.id, tt.conn, tt.connInfo, 256)
			tt.expected(client)
		})
	}
}

func TestClient_GetID(t *testing.T) {
	client := NewClient("test-id-123", &MockWebSocketConn{}, &ConnectionInfo{}, 256)
	assert.Equal(t, "test-id-123", client.GetID())
}

func TestClient_SetUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	client.SetUserData("username", "john_doe")
	client.SetUserData("age", 30)
	client.SetUserData("active", true)

	assert.Equal(t, "john_doe", client.GetUserDataByKey("username"))
	assert.Equal(t, 30, client.GetUserDataByKey("age"))
	assert.Equal(t, true, client.GetUserDataByKey("active"))
}

func TestClient_GetUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	// Set some test data
	client.SetUserData("username", "john_doe")
	client.SetUserData("age", 30)

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
			result := client.GetUserDataByKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClient_RemoveUserData(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	client.SetUserData("key1", "value1")
	client.SetUserData("key2", "value2")

	// Verify keys exist
	assert.Equal(t, "value1", client.GetUserDataByKey("key1"))
	assert.Equal(t, "value2", client.GetUserDataByKey("key2"))

	// Remove key1
	client.RemoveUserData("key1")

	// Verify removal
	assert.Nil(t, client.GetUserDataByKey("key1"))
	assert.Equal(t, "value2", client.GetUserDataByKey("key2"))
}

func TestClient_GetUserDataReturnsEntireMap(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	// Set some test data
	client.SetUserData("username", "john_doe")
	client.SetUserData("age", 30)

	// GetUserData should return the entire map
	allData := client.GetUserData()
	assert.NotNil(t, allData)

	// Verify the map contains the data
	dataMap, ok := allData.(map[string]interface{})
	assert.True(t, ok, "GetUserData should return map[string]interface{}")
	assert.Equal(t, "john_doe", dataMap["username"])
	assert.Equal(t, 30, dataMap["age"])
}

func TestClient_ConcurrentAccess(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

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
				client.GetUserDataByKey(key) // May return nil if not set yet
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

func TestClient_SendChannelOperations(t *testing.T) {
	bufSize := 10
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, bufSize)

	// Test 1: Channel capacity
	assert.Equal(t, bufSize, cap(client.SendChan))

	// Test 2: Send message to channel
	msg := &message.Message{Type: message.TextMessage, RawData: []byte("test")}
	client.SendChan <- msg

	// Test 3: Receive message from channel
	received := <-client.SendChan
	assert.Equal(t, msg, received)

	// Test 4: Fill channel
	for i := 0; i < bufSize; i++ {
		m := &message.Message{Type: message.TextMessage, RawData: []byte(fmt.Sprintf("msg%d", i))}
		select {
		case client.SendChan <- m:
			// Success
		default:
			t.Fatalf("Channel should not be full at message %d", i)
		}
	}

	// Test 5: Channel should be full now
	select {
	case client.SendChan <- &message.Message{}:
		t.Fatal("Channel should be full")
	default:
		// Expected - channel is full
	}
}

func TestClient_SendChannelCapacity(t *testing.T) {
	bufSize := 256
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, bufSize)

	// Verify channel capacity
	assert.Equal(t, bufSize, cap(client.SendChan))

	t.Run("fill channel to capacity - 1", func(t *testing.T) {
		// Fill channel to capacity - 1
		for i := 0; i < bufSize-1; i++ {
			msg := &message.Message{
				Type:    message.BinaryMessage,
				RawData: []byte(fmt.Sprintf("message_%d", i)),
			}
			select {
			case client.SendChan <- msg:
				// Success
			default:
				t.Fatalf("Channel should not be full at message %d", i)
			}
		}

		// Channel length should be capacity - 1
		assert.Equal(t, bufSize-1, len(client.SendChan))
	})

	t.Run("one more message fills channel to capacity", func(t *testing.T) {
		freshClient := NewClient("test-fill", &MockWebSocketConn{}, &ConnectionInfo{}, bufSize)

		// Fill channel completely
		for i := 0; i < bufSize; i++ {
			msg := &message.Message{
				Type:    message.BinaryMessage,
				RawData: []byte(fmt.Sprintf("message_%d", i)),
			}
			select {
			case freshClient.SendChan <- msg:
				// Success
			default:
				t.Fatalf("Channel should not be full at message %d", i)
			}
		}

		// Channel should be at capacity
		assert.Equal(t, bufSize, len(freshClient.SendChan))

		// Next send should block (non-blocking select will go to default)
		msg := &message.Message{
			Type:    message.BinaryMessage,
			RawData: []byte("overflow_message"),
		}
		select {
		case freshClient.SendChan <- msg:
			t.Fatal("Channel should be full - send should block")
		default:
			// Expected - channel is full
		}
	})

	t.Run("receive message frees capacity", func(t *testing.T) {
		freshClient := NewClient("test-receive", &MockWebSocketConn{}, &ConnectionInfo{}, bufSize)

		// Fill completely
		for i := 0; i < bufSize; i++ {
			msg := &message.Message{
				Type:    message.BinaryMessage,
				RawData: []byte(fmt.Sprintf("msg_%d", i)),
			}
			freshClient.SendChan <- msg
		}

		assert.Equal(t, bufSize, len(freshClient.SendChan))

		// Receive one
		<-freshClient.SendChan

		// Now length should be one less
		assert.Equal(t, bufSize-1, len(freshClient.SendChan))

		// And we should be able to send again
		msg := &message.Message{
			Type:    message.BinaryMessage,
			RawData: []byte("after_receive"),
		}
		select {
		case freshClient.SendChan <- msg:
			// Success
		default:
			t.Fatal("Should be able to send after receiving")
		}
	})

	t.Run("channel handles different buffer sizes", func(t *testing.T) {
		sizes := []int{1, 10, 64, 256, 512}

		for _, size := range sizes {
			t.Run(fmt.Sprintf("buffer_size_%d", size), func(t *testing.T) {
				c := NewClient(fmt.Sprintf("test-%d", size), &MockWebSocketConn{}, &ConnectionInfo{}, size)
				assert.Equal(t, size, cap(c.SendChan))

				// Fill to capacity
				for i := 0; i < size; i++ {
					c.SendChan <- &message.Message{
						Type:    message.TextMessage,
						RawData: []byte(fmt.Sprintf("msg_%d", i)),
					}
				}

				// Should be full
				assert.Equal(t, size, len(c.SendChan))

				// Next send should block
				select {
				case c.SendChan <- &message.Message{}:
					t.Fatalf("Channel of size %d should be full", size)
				default:
					// Expected
				}
			})
		}
	})
}

func TestClient_UserDataThreadSafety(t *testing.T) {
	client := NewClient("test", &MockWebSocketConn{}, &ConnectionInfo{}, 256)

	var wg sync.WaitGroup
	numGoroutines := 20

	wg.Add(numGoroutines * 3) // writers, readers, removers

	// Set initial data
	for i := 0; i < numGoroutines; i++ {
		client.SetUserData(fmt.Sprintf("initial_%d", i), i)
	}

	// Concurrent writers (overwrite existing data)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				client.SetUserData(fmt.Sprintf("initial_%d", id), j)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = client.GetUserDataByKey(fmt.Sprintf("initial_%d", id))
			}
		}(i)
	}

	// Concurrent removers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				client.RemoveUserData(fmt.Sprintf("initial_%d", id))
			}
		}(i)
	}

	// Wait for completion
	done := make(chan bool)
	go func() {
		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		// Success - no race condition
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

func TestClient_ConnectionInfo(t *testing.T) {
	connInfo := &ConnectionInfo{
		ClientIP:  "192.168.1.100",
		UserAgent: "Mozilla/5.0 (X11; Linux x86_64)",
		Origin:    "https://example.com",
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"Custom-Header": "custom-value",
		},
		RequestID: "req-12345",
	}

	client := NewClient("test-client", &MockWebSocketConn{}, connInfo, 256)

	// Verify ConnectionInfo is stored
	assert.NotNil(t, client.connInfo)
	assert.Equal(t, "192.168.1.100", client.connInfo.ClientIP)
	assert.Equal(t, "Mozilla/5.0 (X11; Linux x86_64)", client.connInfo.UserAgent)
	assert.Equal(t, "https://example.com", client.connInfo.Origin)
	assert.Equal(t, "req-12345", client.connInfo.RequestID)
	assert.Equal(t, "custom-value", client.connInfo.Headers["Custom-Header"])
}

func TestClient_ConnectionInfoNil(t *testing.T) {
	client := NewClient("test-client", &MockWebSocketConn{}, nil, 256)

	// Should not panic and connInfo should be nil
	assert.Nil(t, client.connInfo)

	// Client should still be functional
	client.SetUserData("test", "value")
	assert.Equal(t, "value", client.GetUserDataByKey("test"))
}
