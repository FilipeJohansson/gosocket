package backends

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/message"
)

const integrationRedisAddr = "127.0.0.1:6379"

func TestIntegrationRedis_ClusterBroadcastAcrossNodes(t *testing.T) {
	topic := integrationTopic(t)

	m1 := newIntegrationRedisManagerOrSkip(t, topic, "integration-node-1")
	m2 := newIntegrationRedisManagerOrSkip(t, topic, "integration-node-2")
	t.Cleanup(func() {
		closeIntegrationRedisManager(m1)
		closeIntegrationRedisManager(m2)
	})

	d1, cancelNode1 := startIntegrationClusterNode(t, m1, "integration-node-1", nil)
	d2, cancelNode2 := startIntegrationClusterNode(t, m2, "integration-node-2", nil)
	t.Cleanup(cancelNode1)
	t.Cleanup(cancelNode2)

	c1 := hub.NewClient("client-node-1", nil, nil, 16)
	c2 := hub.NewClient("client-node-2", nil, nil, 16)

	if err := d1.RegisterClient(c1); err != nil {
		t.Fatalf("register client on node 1 failed: %v", err)
	}
	if err := d2.RegisterClient(c2); err != nil {
		t.Fatalf("register client on node 2 failed: %v", err)
	}

	msg := message.NewRawMessage(message.TextMessage, []byte("cluster-broadcast"))
	if err := d1.Broadcast(msg); err != nil {
		t.Fatalf("broadcast from node 1 failed: %v", err)
	}

	assertMessagePayload(t, c1.SendChan, "cluster-broadcast")
	assertMessagePayload(t, c2.SendChan, "cluster-broadcast")
}

func TestIntegrationRedis_ClusterSendToClientAcrossNodes(t *testing.T) {
	topic := integrationTopic(t)

	m1 := newIntegrationRedisManagerOrSkip(t, topic, "integration-node-1")
	m2 := newIntegrationRedisManagerOrSkip(t, topic, "integration-node-2")
	t.Cleanup(func() {
		closeIntegrationRedisManager(m1)
		closeIntegrationRedisManager(m2)
	})

	sharedState := store.NewMemoryStateStore()

	d1, cancelNode1 := startIntegrationClusterNode(t, m1, "integration-node-1", sharedState)
	d2, cancelNode2 := startIntegrationClusterNode(t, m2, "integration-node-2", sharedState)
	t.Cleanup(cancelNode1)
	t.Cleanup(cancelNode2)

	remoteClient := hub.NewClient("remote-client", nil, nil, 16)
	if err := d2.RegisterClient(remoteClient); err != nil {
		t.Fatalf("register remote client failed: %v", err)
	}

	directMsg := message.NewRawMessage(message.TextMessage, []byte("direct-message"))
	directMsg.From = "sender-node-1"

	if err := d1.SendToClient("remote-client", directMsg); err != nil {
		t.Fatalf("send to remote client failed: %v", err)
	}

	assertMessagePayload(t, remoteClient.SendChan, "direct-message")
}

func startIntegrationClusterNode(
	t *testing.T,
	m *RedisManager,
	nodeID string,
	state store.StateStore,
) (*dispatcher.ClusterDispatcher, context.CancelFunc) {
	t.Helper()

	serializers := map[message.EncodingType]message.Serializer{
		message.JSON: message.NewJSONSerializer(message.DefaultSerializerConfig()),
		message.Raw:  message.NewRawSerializer(message.DefaultSerializerConfig()),
	}

	h := hub.NewHub(hub.DefaultHubConfig())
	local := dispatcher.NewLocalDispatcher(h, serializers)
	d := dispatcher.NewClusterDispatcher(local, m, state, nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)

	if err := d.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start cluster dispatcher %s failed: %v", nodeID, err)
	}

	return d, func() {
		_ = d.Stop()
		cancel()
	}
}

func newIntegrationRedisManagerOrSkip(t *testing.T, topic, nodeID string) *RedisManager {
	t.Helper()

	manager, err := NewRedisManager(integrationRedisAddr, topic, nodeID)
	if err != nil {
		t.Skipf("skipping Redis integration test (Redis unavailable at %s): %v", integrationRedisAddr, err)
	}

	rm, ok := manager.(*RedisManager)
	if !ok {
		t.Fatalf("unexpected manager type: %T", manager)
	}
	return rm
}

func assertMessagePayload(t *testing.T, ch <-chan *message.Message, expected string) {
	t.Helper()

	select {
	case got := <-ch:
		if got == nil {
			t.Fatal("received nil message")
		}
		if string(got.RawData) != expected {
			t.Fatalf("unexpected payload: got %q, want %q", string(got.RawData), expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for payload %q", expected)
	}
}

func closeIntegrationRedisManager(m *RedisManager) {
	if m == nil {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	if m.client != nil {
		_ = m.client.Close()
	}
}

func integrationTopic(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("integration-redis-topic-%d", time.Now().UnixNano())
}
