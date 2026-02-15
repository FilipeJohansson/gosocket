package backends

import (
	"context"
	"testing"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/stretchr/testify/require"
)

func TestMemoryManager_SubscribePublish(t *testing.T) {
	mgr := NewTestMemoryManager()
	require.NotNil(t, mgr)

	sub, err := mgr.Subscribe(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.NoError(t, sub.Close())

	evt := &cluster.Event{Type: cluster.EventBroadcast, Payload: []byte("hello")}

	ready := make(chan struct{})
	got := make(chan *cluster.Event, 1)
	go func() {
		close(ready)
		select {
		case e := <-sub.Events():
			got <- e
		case <-time.After(200 * time.Millisecond):
		}
	}()

	<-ready
	require.NoError(t, mgr.Publish(evt))

	select {
	case received := <-got:
		require.Equal(t, evt, received)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timed out waiting memory backend publish delivery")
	}
}

func TestMemoryManager_PublishWithoutSubscribers(t *testing.T) {
	mgr := NewTestMemoryManager()
	require.NotNil(t, mgr)
	require.NoError(t, mgr.Publish(&cluster.Event{Type: cluster.EventBroadcast}))
}
