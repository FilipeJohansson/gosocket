// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package dispatcher

import (
	"context"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/stretchr/testify/require"
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
	require.Eventually(t, func() bool {
		return testHub.Running()
	}, 2*time.Second, 10*time.Millisecond, "hub did not start")

	// Create a test client in the hub
	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	require.Eventually(t, func() bool {
		return testHub.GetClient("test-client-1") != nil
	}, 2*time.Second, 10*time.Millisecond, "client not registered")

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
	require.Eventually(t, func() bool { return testHub.Running() }, 2*time.Second, 10*time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	require.Eventually(t, func() bool { return testHub.GetClient("test-client-1") != nil }, 2*time.Second, 10*time.Millisecond)

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
	require.Eventually(t, func() bool { return testHub.Running() }, 2*time.Second, 10*time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	require.Eventually(t, func() bool { return testHub.GetClient("test-client-1") != nil }, 2*time.Second, 10*time.Millisecond)

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
	require.Eventually(t, func() bool { return testHub.Running() }, 2*time.Second, 10*time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	require.Eventually(t, func() bool { return testHub.GetClient("test-client-1") != nil }, 2*time.Second, 10*time.Millisecond)

	// Send message locally - should be published to cluster
	msg := message.NewMessage(message.TextMessage, "test data")
	err = clusterDispatcher.SendToClient("test-client-1", msg)
	if err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Verify event was published
	require.Eventually(t, func() bool {
		return len(mockCluster.publishedEvents) > 0
	}, 2*time.Second, 10*time.Millisecond, "no events were published to cluster")

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
	require.Eventually(t, func() bool { return testHub.Running() }, 2*time.Second, 10*time.Millisecond)

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
	require.Eventually(t, func() bool { return testHub.Running() }, 2*time.Second, 10*time.Millisecond)

	testClient := hub.NewClient("test-client-1", nil, nil, 256)
	_ = testHub.AddClient(testClient)
	require.Eventually(t, func() bool { return testHub.GetClient("test-client-1") != nil }, 2*time.Second, 10*time.Millisecond)

	// Send a LOCAL message and capture it
	localMsg := message.NewMessage(message.TextMessage, "local data")
	localMsg.Encoding = message.JSON
	localErr := clusterDispatcher.SendToClient("test-client-1", localMsg)
	if localErr != nil {
		t.Fatalf("Failed to send local: %v", localErr)
	}

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

type mockStateStore struct {
	rooms       []store.RoomInfo
	locations   map[string]string
	roomMembers map[string][]store.ClientPresence
}

func newMockStateStore() *mockStateStore {
	return &mockStateStore{
		rooms:       make([]store.RoomInfo, 0),
		locations:   make(map[string]string),
		roomMembers: make(map[string][]store.ClientPresence),
	}
}

func (m *mockStateStore) UpsertRoom(ctx context.Context, room store.RoomInfo) error {
	_ = ctx
	for i := range m.rooms {
		if m.rooms[i].Name == room.Name {
			m.rooms[i] = room
			return nil
		}
	}
	m.rooms = append(m.rooms, room)
	return nil
}

func (m *mockStateStore) DeleteRoom(ctx context.Context, roomName string) error {
	_ = ctx
	out := m.rooms[:0]
	for _, r := range m.rooms {
		if r.Name != roomName {
			out = append(out, r)
		}
	}
	m.rooms = out
	delete(m.roomMembers, roomName)
	return nil
}

func (m *mockStateStore) GetRooms(ctx context.Context) ([]store.RoomInfo, error) {
	_ = ctx
	cp := make([]store.RoomInfo, len(m.rooms))
	copy(cp, m.rooms)
	return cp, nil
}

func (m *mockStateStore) AddClientToRoom(ctx context.Context, p store.ClientPresence) error {
	_ = ctx
	m.roomMembers[p.RoomName] = append(m.roomMembers[p.RoomName], p)
	return nil
}

func (m *mockStateStore) RemoveClientFromRoom(ctx context.Context, clientID, roomName string) error {
	_ = ctx
	members := m.roomMembers[roomName]
	out := members[:0]
	for _, p := range members {
		if p.ClientID != clientID {
			out = append(out, p)
		}
	}
	m.roomMembers[roomName] = out
	return nil
}

func (m *mockStateStore) GetClientsInRoom(ctx context.Context, roomName string) ([]store.ClientPresence, error) {
	_ = ctx
	members := m.roomMembers[roomName]
	cp := make([]store.ClientPresence, len(members))
	copy(cp, members)
	return cp, nil
}

func (m *mockStateStore) SetClientNode(ctx context.Context, clientID, nodeID string) error {
	_ = ctx
	m.locations[clientID] = nodeID
	return nil
}

func (m *mockStateStore) RemoveClientNode(ctx context.Context, clientID string) error {
	_ = ctx
	delete(m.locations, clientID)
	return nil
}

func (m *mockStateStore) ResolveClientNode(ctx context.Context, clientID string) (string, error) {
	_ = ctx
	return m.locations[clientID], nil
}

func (m *mockStateStore) GetClients(ctx context.Context) ([]store.ClientLocation, error) {
	_ = ctx
	out := make([]store.ClientLocation, 0, len(m.locations))
	for id, node := range m.locations {
		out = append(out, store.ClientLocation{ClientID: id, NodeID: node})
	}
	return out, nil
}

func startHubForClusterDispatcherTest(t *testing.T, h *hub.Hub, ctx context.Context) {
	t.Helper()
	go h.Run(ctx)
	require.Eventually(t, func() bool { return h.Running() }, 2*time.Second, 10*time.Millisecond)
}

func TestClusterDispatcher_MethodsAndState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.NewHub(hub.DefaultHubConfig())
	startHubForClusterDispatcherTest(t, h, ctx)

	local := NewLocalDispatcher(h, map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
		message.Raw:  message.NewRawSerializer(message.DefaultSerializerConfig()),
	})

	mockCluster := NewMockClusterManager()
	mockState := newMockStateStore()
	d := NewClusterDispatcher(local, mockCluster, mockState, "node-1")

	require.NoError(t, d.Start(ctx))
	defer func() { _ = d.Stop() }()

	require.NoError(t, d.Start(ctx))

	c1 := hub.NewClient("c1", nil, nil, 16)
	c2 := hub.NewClient("c2", nil, nil, 16)

	require.NoError(t, d.RegisterClient(c1))
	require.NoError(t, d.RegisterClient(c2))

	require.Eventually(t, func() bool {
		return h.GetClient("c1") != nil && h.GetClient("c2") != nil
	}, 2*time.Second, 10*time.Millisecond)

	_, err := d.CreateRoom("c1", "room-1")
	require.NoError(t, err)
	require.NoError(t, d.JoinRoom("c1", "room-1"))

	rooms := d.GetRooms()
	require.Contains(t, rooms, "room-1")
	require.Contains(t, d.GetClients(), "c1")
	require.Contains(t, d.GetClientsInRoom("room-1"), "c1")

	direct := message.NewRawMessage(message.TextMessage, []byte("direct"))
	require.NoError(t, d.SendToClient("c1", direct))
	select {
	case msg := <-c1.SendChan:
		require.Equal(t, []byte("direct"), msg.RawData)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting send-to-client")
	}

	all := message.NewRawMessage(message.TextMessage, []byte("all"))
	require.NoError(t, d.Broadcast(all))
	select {
	case <-c1.SendChan:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting broadcast c1")
	}
	select {
	case <-c2.SendChan:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting broadcast c2")
	}

	roomOnly := message.NewRawMessage(message.TextMessage, []byte("room-only"))
	require.NoError(t, d.BroadcastToRoom("room-1", roomOnly))
	select {
	case <-c1.SendChan:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting room broadcast")
	}

	require.NoError(t, d.LeaveRoom("c1", "room-1"))
	require.NoError(t, d.DeleteRoom("c1", "room-1"))

	globalRooms, err := d.GetRoomsGlobal(context.Background())
	require.NoError(t, err)
	require.Len(t, globalRooms, 0)

	globalClients, err := d.GetClientsGlobal(context.Background())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(globalClients), 2)

	roomMembers, err := d.GetClientsInRoomGlobal(context.Background(), "room-1")
	require.NoError(t, err)
	require.Len(t, roomMembers, 0)

	require.NoError(t, d.UnregisterClient("c1"))
	require.NoError(t, d.DisconnectClient("c2"))
	require.Eventually(t, func() bool {
		return h.GetClient("c1") == nil && h.GetClient("c2") == nil
	}, 2*time.Second, 10*time.Millisecond)

	require.NoError(t, d.DisconnectAll())
	require.Equal(t, Stats{}, d.GetStats())

	require.NotEmpty(t, mockCluster.publishedEvents)
}

func TestClusterDispatcher_ErrorsAndFallbacks(t *testing.T) {
	h := hub.NewHub(hub.DefaultHubConfig())
	local := NewLocalDispatcher(h, map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
		message.Raw:  message.NewRawSerializer(message.DefaultSerializerConfig()),
	})
	mockCluster := NewMockClusterManager()

	d := NewClusterDispatcher(local, mockCluster, nil, "node-1")

	require.NoError(t, d.Start(t.Context()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startHubForClusterDispatcherTest(t, h, ctx)
	require.NoError(t, d.Start(ctx))
	defer func() { _ = d.Stop() }()

	require.Error(t, d.RegisterClient(nil))
	require.Error(t, d.UnregisterClient(""))
	require.Error(t, d.JoinRoom("", "room"))
	require.Error(t, d.LeaveRoom("", "room"))
	_, err := d.CreateRoom("", "room")
	require.Error(t, err)
	require.Error(t, d.DeleteRoom("", "room"))
	require.Error(t, d.Broadcast(nil))
	require.Error(t, d.BroadcastToRoom("", message.NewRawMessage(message.TextMessage, []byte("x"))))

	msg := message.NewRawMessage(message.TextMessage, []byte("remote"))
	err = d.SendToClient("missing-client", msg)
	require.Error(t, err)

	require.NoError(t, d.Stop())

	require.Error(t, d.RegisterClient(hub.NewClient("late", nil, nil, 1)))
	require.Error(t, d.UnregisterClient("late"))
	require.Error(t, d.JoinRoom("late", "room"))
	require.Error(t, d.LeaveRoom("late", "room"))
	_, err = d.CreateRoom("late", "room")
	require.Error(t, err)
	require.Error(t, d.DeleteRoom("late", "room"))
	require.Error(t, d.SendToClient("late", message.NewRawMessage(message.TextMessage, []byte("x"))))
	require.Error(t, d.Broadcast(message.NewRawMessage(message.TextMessage, []byte("x"))))
	require.Error(t, d.BroadcastToRoom("room", message.NewRawMessage(message.TextMessage, []byte("x"))))

	dNoState := NewClusterDispatcher(local, mockCluster, nil, "node-1")
	_, err = dNoState.GetRoomsGlobal(context.Background())
	require.Error(t, err)
	_, err = dNoState.GetClientsGlobal(context.Background())
	require.Error(t, err)
	_, err = dNoState.GetClientsInRoomGlobal(context.Background(), "room")
	require.Error(t, err)
}

func TestClusterDispatcher_InternalHelpers(t *testing.T) {
	d := &ClusterDispatcher{nodeID: "node-1"}
	require.NoError(t, d.Stop())
	require.True(t, d.isLocalOrigin(&cluster.Event{Origin: "node-1"}))
	require.False(t, d.isLocalOrigin(&cluster.Event{Origin: "node-2"}))
	require.NoError(t, d.publish(nil))
}
