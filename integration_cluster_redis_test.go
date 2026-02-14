package gosocket

import (
	"fmt"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/hub"
)

const clusterRedisAddr = "127.0.0.1:6379"

func TestIntegrationClusterRedis_NewHandlerBroadcastAcrossNodes(t *testing.T) {
	topic := fmt.Sprintf("gosocket-cluster-broadcast-%d", time.Now().UnixNano())
	state := NewTestMemoryStateStore()

	m1 := newClusterRedisManagerOrSkip(t, topic, "cluster-node-1")
	m2 := newClusterRedisManagerOrSkip(t, topic, "cluster-node-2")

	h1, err := NewHandler(
		WithCluster(ClusterConfig{Manager: m1, NodeID: "cluster-node-1", State: state}),
	)
	if err != nil {
		t.Fatalf("create handler node 1 failed: %v", err)
	}

	h2, err := NewHandler(
		WithCluster(ClusterConfig{Manager: m2, NodeID: "cluster-node-2", State: state}),
	)
	if err != nil {
		t.Fatalf("create handler node 2 failed: %v", err)
	}

	c1 := hub.NewClient("node-1-client", nil, nil, 16)
	c2 := hub.NewClient("node-2-client", nil, nil, 16)

	if err := h1.Dispatcher().RegisterClient(c1); err != nil {
		t.Fatalf("register client on node 1 failed: %v", err)
	}
	if err := h2.Dispatcher().RegisterClient(c2); err != nil {
		t.Fatalf("register client on node 2 failed: %v", err)
	}

	msg := NewRawMessage(TextMessage, []byte("broadcast-from-node-1"))
	if err := h1.Dispatcher().Broadcast(msg); err != nil {
		t.Fatalf("broadcast failed: %v", err)
	}

	assertClientMessagePayload(t, c1, "broadcast-from-node-1")
	assertClientMessagePayload(t, c2, "broadcast-from-node-1")
}

func TestIntegrationClusterRedis_NewHandlerSendToClientAcrossNodes(t *testing.T) {
	topic := fmt.Sprintf("gosocket-cluster-send-to-client-%d", time.Now().UnixNano())

	m1 := newClusterRedisManagerOrSkip(t, topic, "cluster-node-1")
	m2 := newClusterRedisManagerOrSkip(t, topic, "cluster-node-2")
	state := NewTestMemoryStateStore()

	h1, err := NewHandler(
		WithCluster(ClusterConfig{
			Manager: m1,
			NodeID:  "cluster-node-1",
			State:   state,
		}),
	)
	if err != nil {
		t.Fatalf("create handler node 1 failed: %v", err)
	}

	h2, err := NewHandler(
		WithCluster(ClusterConfig{
			Manager: m2,
			NodeID:  "cluster-node-2",
			State:   state,
		}),
	)
	if err != nil {
		t.Fatalf("create handler node 2 failed: %v", err)
	}

	targetClient := hub.NewClient("target-client-node-2", nil, nil, 16)
	if err := h2.Dispatcher().RegisterClient(targetClient); err != nil {
		t.Fatalf("register target client on node 2 failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		nodeID, err := state.ResolveClientNode(t.Context(), "target-client-node-2")
		return err == nil && nodeID == "cluster-node-2"
	})

	msg := NewRawMessage(TextMessage, []byte("direct-to-node-2"))
	msg.From = "sender-node-1"

	if err := h1.Dispatcher().SendToClient("target-client-node-2", msg); err != nil {
		t.Fatalf("send to remote client failed: %v", err)
	}

	assertClientMessagePayload(t, targetClient, "direct-to-node-2")
}

func newClusterRedisManagerOrSkip(t *testing.T, topic, nodeID string) ClusterManager {
	t.Helper()

	manager, err := NewTestRedisManager(clusterRedisAddr, topic, nodeID)
	if err != nil {
		t.Skipf("skipping Redis cluster integration test (Redis unavailable at %s): %v", clusterRedisAddr, err)
	}
	return manager
}

func assertClientMessagePayload(t *testing.T, client *hub.Client, expected string) {
	t.Helper()

	select {
	case msg := <-client.SendChan:
		if msg == nil {
			t.Fatal("received nil message")
		}
		if string(msg.RawData) != expected {
			t.Fatalf("unexpected payload: got %q, want %q", string(msg.RawData), expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for payload %q", expected)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
