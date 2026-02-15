package backends

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
)

const redisTestAddr = "127.0.0.1:6379"

func TestRedisManager_RegisterAndPublish(t *testing.T) {
	topic := uniqueRedisTestTopic(t)

	m1 := newRedisManagerOrSkip(t, topic, "test-redis-node-1")
	m2 := newRedisManagerOrSkip(t, topic, "test-redis-node-2")
	t.Cleanup(func() {
		closeRedisManager(m1)
		closeRedisManager(m2)
	})

	sub1 := subscribeNode(t, m1, "node-1")
	sub2 := subscribeNode(t, m2, "node-2")

	assertRegisteredNodes(t, m1, 1)
	assertRegisteredNodes(t, m2, 1)

	eventFromNode1 := &cluster.Event{
		Type:    cluster.EventBroadcast,
		Payload: []byte("hello-from-node-1"),
	}
	if err := m1.Publish(eventFromNode1); err != nil {
		t.Fatalf("publish event from node 1 failed: %v", err)
	}

	eventFromNode2 := &cluster.Event{
		Type:    cluster.EventBroadcast,
		Payload: []byte("hello-from-node-2"),
	}
	if err := m2.Publish(eventFromNode2); err != nil {
		t.Fatalf("publish event from node 2 failed: %v", err)
	}

	assertEventWithPayload(t, sub1.Events(), "hello-from-node-1")
	assertEventWithPayload(t, sub1.Events(), "hello-from-node-2")
	assertEventWithPayload(t, sub2.Events(), "hello-from-node-1")
	assertEventWithPayload(t, sub2.Events(), "hello-from-node-2")
}

func TestRedisManager_Unregister(t *testing.T) {
	topic := uniqueRedisTestTopic(t)

	m := newRedisManagerOrSkip(t, topic, "test-redis-node-1")
	t.Cleanup(func() { closeRedisManager(m) })

	ctx1, cancel1 := context.WithCancel(context.Background())
	t.Cleanup(cancel1)
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)

	sub1 := subscribeNodeWithContext(t, m, ctx1, "node-1")
	sub2 := subscribeNodeWithContext(t, m, ctx2, "node-2")

	assertRegisteredNodes(t, m, 2)

	if err := sub1.Close(); err != nil {
		t.Fatalf("close subscription node-1 failed: %v", err)
	}
	if err := sub1.Close(); err != nil {
		t.Fatalf("close subscription node-1 second call failed: %v", err)
	}

	assertRegisteredNodes(t, m, 1)

	if err := m.Publish(&cluster.Event{
		Type:    cluster.EventBroadcast,
		Payload: []byte("event-after-node-1-unregister"),
	}); err != nil {
		t.Fatalf("publish after unregister failed: %v", err)
	}

	assertEventWithPayload(t, sub2.Events(), "event-after-node-1-unregister")

	cancel2()
	assertEventually(t, time.Second, func() bool {
		return registeredNodeCount(m) == 0
	})
}

func TestRedisManager_PublishNilEvent(t *testing.T) {
	m := &RedisManager{}
	if err := m.Publish(nil); err == nil {
		t.Fatal("expected error when publishing nil event, got nil")
	}
}

func newRedisManagerOrSkip(t *testing.T, topic, nodeID string) *RedisManager {
	t.Helper()

	manager, err := NewRedisManager(redisTestAddr, topic, nodeID)
	if err != nil {
		t.Skipf("skipping Redis integration test (Redis unavailable at %s): %v", redisTestAddr, err)
	}

	rm, ok := manager.(*RedisManager)
	if !ok {
		t.Fatalf("unexpected manager type: %T", manager)
	}
	return rm
}

func subscribeNode(t *testing.T, m *RedisManager, nodeID string) cluster.Subscription {
	t.Helper()
	return subscribeNodeWithContext(t, m, context.Background(), nodeID)
}

func subscribeNodeWithContext(t *testing.T, m *RedisManager, ctx context.Context, nodeID string) cluster.Subscription {
	t.Helper()

	sub, err := m.Subscribe(ctx, nodeID)
	if err != nil {
		t.Fatalf("subscribe node %s failed: %v", nodeID, err)
	}
	return sub
}

func assertEventWithPayload(t *testing.T, ch <-chan *cluster.Event, expected string) {
	t.Helper()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				t.Fatal("subscription channel closed before receiving expected event")
			}
			if evt == nil {
				continue
			}
			if string(evt.Payload) == expected {
				return
			}
		case <-timeout:
			t.Fatalf("expected payload %q was not received in time", expected)
		}
	}
}

func assertRegisteredNodes(t *testing.T, m *RedisManager, expected int) {
	t.Helper()

	if got := registeredNodeCount(m); got != expected {
		t.Fatalf("unexpected registered node count: got %d, want %d", got, expected)
	}
}

func registeredNodeCount(m *RedisManager) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.nodes)
}

func assertEventually(t *testing.T, timeout time.Duration, cond func() bool) {
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

func closeRedisManager(m *RedisManager) {
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

func uniqueRedisTestTopic(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-topic-%d", time.Now().UnixNano())
}
