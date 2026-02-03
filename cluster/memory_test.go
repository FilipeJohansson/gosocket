package cluster

import (
	"testing"
	"time"
)

func TestMemoryManager_RegisterAndPublish(t *testing.T) {
	m := NewMemoryManager()

	ch1 := make(chan *ClusterEvent, 1)
	ch2 := make(chan *ClusterEvent, 1)

	if err := m.RegisterNode("n1", ch1); err != nil {
		t.Fatalf("register n1: %v", err)
	}
	if err := m.RegisterNode("n2", ch2); err != nil {
		t.Fatalf("register n2: %v", err)
	}

	// internal bookkeeping: both nodes should be registered
	if mm, ok := m.(*memoryManager); ok {
		if got := len(mm.nodes); got != 2 {
			t.Fatalf("expected 2 nodes registered, got %d", got)
		}
	} else {
		t.Fatalf("unexpected manager type: %T", m)
	}

	evt := &ClusterEvent{
		Type:         EventBroadcastAll,
		Raw:          []byte("hello"),
		OriginNodeID: "n1",
	}
	m.PublishEvent(evt)

	select {
	case r := <-ch2:
		if string(r.Raw) != "hello" {
			t.Fatalf("unexpected payload: %s", string(r.Raw))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("expected node n2 to receive event")
	}

	// n1 should not receive its own event
	select {
	case <-ch1:
		t.Fatalf("n1 should not receive its own event")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestMemoryManager_Unregister(t *testing.T) {
	m := NewMemoryManager()
	ch := make(chan *ClusterEvent, 1)
	if err := m.RegisterNode("n", ch); err != nil {
		t.Fatalf("register: %v", err)
	}
	m.UnregisterNode("n")

	// ensure internal bookkeeping removed the node
	if mm, ok := m.(*memoryManager); ok {
		if got := len(mm.nodes); got != 0 {
			t.Fatalf("expected 0 nodes after unregister, got %d", got)
		}
	} else {
		t.Fatalf("unexpected manager type: %T", m)
	}

	evt := &ClusterEvent{
		Type:         EventBroadcastAll,
		Raw:          []byte("x"),
		OriginNodeID: "other",
	}
	m.PublishEvent(evt)

	select {
	case <-ch:
		t.Fatalf("unregistered node should not receive events")
	case <-time.After(20 * time.Millisecond):
	}
}
