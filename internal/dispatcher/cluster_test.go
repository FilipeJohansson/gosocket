// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
)

type MockClusterManager struct {
	publishedEvents []*cluster.Event
	subscription    *MockSubscription
}

type MockSubscription struct {
	events chan *cluster.Event
}

func (m *MockSubscription) Events() <-chan *cluster.Event {
	return m.events
}

func (m *MockSubscription) Close() error {
	close(m.events)
	return nil
}

func NewMockClusterManager() *MockClusterManager {
	return &MockClusterManager{
		publishedEvents: []*cluster.Event{},
		subscription: &MockSubscription{
			events: make(chan *cluster.Event, 100),
		},
	}
}

func (m *MockClusterManager) Subscribe(ctx context.Context, nodeID string) (cluster.Subscription, error) {
	return m.subscription, nil
}

func (m *MockClusterManager) Publish(evt *cluster.Event) error {
	m.publishedEvents = append(m.publishedEvents, evt)
	return nil
}

func TestClusterDispatcher_RemoteEventNormalization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup
	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
		message.Raw:  message.NewRawSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cluster dispatcher: %v", err)
	}

	// Start the hub
	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond) // Give hub time to start

	// Create a test client in the hub
	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	time.Sleep(100 * time.Millisecond)

	// TEST CASE 1: EventSendToClient - Verify normalization
	t.Run("EventSendToClient normalization", func(t *testing.T) {
		rawPayload := []byte(`{"test":"data"}`)

		remoteEvent := &cluster.Event{
			Type:         cluster.EventSendToClient,
			MsgType:      message.TextMessage,
			Origin:       "test-node-2", // Different node
			ToClientID:   "test-client-1",
			FromClientID: "remote-sender",
			Payload:      rawPayload,
			Encoding:     int(message.JSON),
		}

		// Apply remote event
		err := clusterDispatcher.applyRemoteEvent(remoteEvent)
		if err != nil {
			t.Fatalf("Failed to apply remote event: %v", err)
		}

		time.Sleep(50 * time.Millisecond) // Give message time to deliver

		// Verify message was delivered
		select {
		case msg := <-testClient.SendChan:
			// VERIFY NORMALIZATION OCCURRED:

			// 1. Message should have RawData from payload
			if msg.RawData == nil {
				t.Error("Message RawData is nil - normalization failed")
			}

			// 2. Message should have encoding set
			if msg.Encoding == 0 {
				t.Error("Message Encoding not set - normalization failed")
			}

			// 3. Message should have Created timestamp
			if msg.Created.IsZero() {
				t.Error("Message Created timestamp not set - normalization failed")
			}

			// 4. Message should have To (recipient) set
			if msg.To != "test-client-1" {
				t.Errorf("Message To field not set correctly: got %s, want test-client-1", msg.To)
			}

			// 5. Message should have From field
			if msg.From != "remote-sender" {
				t.Errorf("Message From field incorrect: got %s, want remote-sender", msg.From)
			}

			// 6. Message Type should match
			if msg.Type != message.TextMessage {
				t.Errorf("Message Type incorrect: got %d, want %d", msg.Type, message.TextMessage)
			}

		case <-time.After(500 * time.Millisecond):
			t.Error("Message not delivered to client - normalization may have failed")
		}
	})

	// TEST CASE 2: EventBroadcast - Verify normalization
	t.Run("EventBroadcast normalization", func(t *testing.T) {
		rawPayload := []byte(`{"broadcast":"message"}`)

		remoteEvent := &cluster.Event{
			Type:     cluster.EventBroadcast,
			MsgType:  message.TextMessage,
			Origin:   "test-node-2",
			Payload:  rawPayload,
			Encoding: int(message.Raw),
		}

		err := clusterDispatcher.applyRemoteEvent(remoteEvent)
		if err != nil {
			t.Fatalf("Failed to apply remote event: %v", err)
		}

		time.Sleep(50 * time.Millisecond)

		// Verify message was delivered to broadcast
		select {
		case msg := <-testClient.SendChan:
			// VERIFY NORMALIZATION OCCURRED:
			if msg.RawData == nil {
				t.Error("Broadcast message RawData is nil - normalization failed")
			}
			if msg.Encoding == 0 {
				t.Error("Broadcast message Encoding not set - normalization failed")
			}
			if msg.Created.IsZero() {
				t.Error("Broadcast message Created not set - normalization failed")
			}

		case <-time.After(500 * time.Millisecond):
			t.Error("Broadcast message not delivered - normalization may have failed")
		}
	})

	// TEST CASE 3: EventBroadcastToRoom - Verify normalization
	t.Run("EventBroadcastToRoom normalization", func(t *testing.T) {
		// Create a room and add client
		room, err := testHub.CreateRoom("test-client-1", "test-room")
		if err != nil {
			t.Fatalf("Failed to create room: %v", err)
		}

		_ = testHub.JoinRoom("test-client-1", room.Name())
		time.Sleep(50 * time.Millisecond)

		rawPayload := []byte(`{"room":"message"}`)

		remoteEvent := &cluster.Event{
			Type:     cluster.EventBroadcastToRoom,
			MsgType:  message.TextMessage,
			Origin:   "test-node-2",
			RoomName: room.Name(),
			Payload:  rawPayload,
			Encoding: int(message.JSON),
		}

		err = clusterDispatcher.applyRemoteEvent(remoteEvent)
		if err != nil {
			t.Fatalf("Failed to apply remote event: %v", err)
		}

		time.Sleep(50 * time.Millisecond)

		// Verify message was delivered to room
		select {
		case msg := <-testClient.SendChan:
			// VERIFY NORMALIZATION OCCURRED:
			if msg.RawData == nil {
				t.Error("Room message RawData is nil - normalization failed")
			}
			if msg.Encoding == 0 {
				t.Error("Room message Encoding not set - normalization failed")
			}
			if msg.Created.IsZero() {
				t.Error("Room message Created not set - normalization failed")
			}
			if msg.Room != room.Name() {
				t.Errorf("Room message Room field incorrect: got %s, want %s", msg.Room, room.Name())
			}

		case <-time.After(500 * time.Millisecond):
			t.Error("Room message not delivered - normalization may have failed")
		}
	})

	_ = clusterDispatcher.Stop()
}

func TestClusterDispatcher_LocalOriginNotReapplied(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cluster dispatcher: %v", err)
	}

	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	time.Sleep(100 * time.Millisecond)

	// Create event from local node
	localEvent := &cluster.Event{
		Type:       cluster.EventSendToClient,
		MsgType:    message.TextMessage,
		Origin:     "test-node-1", // SAME as our node
		ToClientID: "test-client-1",
		Payload:    []byte("test"),
		Encoding:   int(message.JSON),
	}

	// Apply local event - should be ignored
	err = clusterDispatcher.applyRemoteEvent(localEvent)
	if err != nil {
		t.Fatalf("Failed to apply local event: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Verify message was NOT delivered (loop prevention)
	select {
	case msg := <-testClient.SendChan:
		t.Errorf("Local origin event was re-applied - loop prevention failed. Got message: %v", msg)
	case <-time.After(100 * time.Millisecond):
		// Correct - message should not be delivered
	}

	_ = clusterDispatcher.Stop()
}

func TestClusterDispatcher_TargetNodeFiltering(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start cluster dispatcher: %v", err)
	}

	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	time.Sleep(100 * time.Millisecond)

	remoteEvent := &cluster.Event{
		Type:       cluster.EventSendToClient,
		MsgType:    message.TextMessage,
		Origin:     "test-node-2",
		TargetNode: "test-node-3", // different from current node (test-node-1)
		ToClientID: "test-client-1",
		Payload:    []byte("should be ignored"),
		Encoding:   int(message.JSON),
	}

	err = clusterDispatcher.applyRemoteEvent(remoteEvent)
	if err != nil {
		t.Fatalf("Failed to apply remote event: %v", err)
	}

	select {
	case msg := <-testClient.SendChan:
		t.Fatalf("Expected event to be ignored due to target node filtering, got message: %v", msg)
	case <-time.After(200 * time.Millisecond):
		// expected: filtered
	}

	_ = clusterDispatcher.Stop()
}

func TestClusterDispatcher_EventPublishing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	time.Sleep(100 * time.Millisecond)

	// Send message locally - should be published to cluster
	msg := message.NewMessage(message.TextMessage, "test data")
	err = clusterDispatcher.SendToClient("test-client-1", msg)
	if err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify event was published
	if len(mockCluster.publishedEvents) == 0 {
		t.Fatal("No events were published to cluster")
	}

	publishedEvent := mockCluster.publishedEvents[0]

	// Verify event properties
	if publishedEvent.Type != cluster.EventSendToClient {
		t.Errorf("Wrong event type: got %d, want %d", publishedEvent.Type, cluster.EventSendToClient)
	}

	if publishedEvent.Origin != "test-node-1" {
		t.Errorf("Wrong origin: got %s, want test-node-1", publishedEvent.Origin)
	}

	if publishedEvent.ToClientID != "test-client-1" {
		t.Errorf("Wrong recipient: got %s, want test-client-1", publishedEvent.ToClientID)
	}

	_ = clusterDispatcher.Stop()
}

func TestClusterDispatcher_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	// Test with invalid client ID - should return error
	msg := message.NewMessage(message.TextMessage, "test")
	err = clusterDispatcher.SendToClient("nonexistent-client", msg)
	if err == nil {
		t.Error("Expected error when sending to nonexistent client, got nil")
	}

	_ = clusterDispatcher.Stop()
}

func TestClusterDispatcher_NormalizationConsistency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubCfg := hub.DefaultHubConfig()
	testHub := hub.NewHub(hubCfg)

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
	}

	localDispatcher := NewLocalDispatcher(testHub, serializers)
	mockCluster := NewMockClusterManager()
	clusterDispatcher := NewClusterDispatcher(localDispatcher, mockCluster, nil, "test-node-1")

	err := clusterDispatcher.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	go testHub.Run(ctx)
	time.Sleep(100 * time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	time.Sleep(100 * time.Millisecond)

	// Send a LOCAL message and capture it
	localMsg := message.NewMessage(message.TextMessage, "local data")
	localMsg.Encoding = message.JSON
	localErr := clusterDispatcher.SendToClient("test-client-1", localMsg)
	if localErr != nil {
		t.Fatalf("Failed to send local: %v", localErr)
	}

	time.Sleep(100 * time.Millisecond)

	var localDelivered *message.Message
	select {
	case localDelivered = <-testClient.SendChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Local message not delivered")
	}

	// Now send a REMOTE message and capture it
	remoteEvent := &cluster.Event{
		Type:       cluster.EventSendToClient,
		MsgType:    message.TextMessage,
		Origin:     "other-node",
		ToClientID: "test-client-1",
		Payload:    []byte("remote data"),
		Encoding:   int(message.JSON),
	}

	remoteErr := clusterDispatcher.applyRemoteEvent(remoteEvent)
	if remoteErr != nil {
		t.Fatalf("Failed to apply remote: %v", remoteErr)
	}

	time.Sleep(100 * time.Millisecond)

	var remoteDelivered *message.Message
	select {
	case remoteDelivered = <-testClient.SendChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Remote message not delivered")
	}

	// VERIFY BOTH FOLLOW SAME NORMALIZATION:

	// 1. Both should have RawData set
	if localDelivered.RawData == nil {
		t.Error("Local message missing RawData")
	}
	if remoteDelivered.RawData == nil {
		t.Error("Remote message missing RawData")
	}

	// 2. Both should have Encoding set
	if localDelivered.Encoding == 0 {
		t.Error("Local message missing Encoding")
	}
	if remoteDelivered.Encoding == 0 {
		t.Error("Remote message missing Encoding")
	}

	// 3. Both should have Created timestamp
	if localDelivered.Created.IsZero() {
		t.Error("Local message missing Created")
	}
	if remoteDelivered.Created.IsZero() {
		t.Error("Remote message missing Created")
	}

	// 4. Both should have To field set
	if localDelivered.To != "test-client-1" {
		t.Error("Local message To not set")
	}
	if remoteDelivered.To != "test-client-1" {
		t.Error("Remote message To not set")
	}

	_ = clusterDispatcher.Stop()
}
