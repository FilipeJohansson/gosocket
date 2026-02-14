// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package backends

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/redis/go-redis/v9"
)

type RedisSubscription struct {
	ch      <-chan *cluster.Event
	closeFn func() error
	once    sync.Once
}

func (s *RedisSubscription) Events() <-chan *cluster.Event {
	return s.ch
}

func (s *RedisSubscription) Close() error {
	var err error
	s.once.Do(func() {
		if s.closeFn != nil {
			err = s.closeFn()
		}
	})
	return err
}

type RedisManager struct {
	nodes  map[string]chan *cluster.Event
	client *redis.Client
	topic  string
	nodeID string
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewRedisManager creates a new RedisManager instance.
// It is intended for development and tests.
func NewRedisManager(addr, topic, nodeID string) (cluster.Manager, error) {
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	opt := &redis.Options{Addr: addr}
	client := redis.NewClient(opt)
	fmt.Printf("[RedisManager(nodeID=%s)] Connecting to Redis at %s\n", nodeID, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}

	rm := &RedisManager{
		nodes:  make(map[string]chan *cluster.Event),
		client: client,
		topic:  topic,
		nodeID: nodeID,
	}

	subCtx, subCancel := context.WithCancel(context.Background())
	rm.cancel = subCancel
	pubsub := client.Subscribe(subCtx, topic)

	// ensure subscription established
	if _, err := pubsub.Receive(subCtx); err != nil {
		subCancel()
		_ = client.Close()
		return nil, err
	}

	fmt.Printf("[RedisManager(nodeID=%s)] Subscribed to Redis topic %s\n", rm.nodeID, topic)
	ch := pubsub.Channel()
	go func() {
		for {
			select {
			case m, ok := <-ch:
				if !ok {
					return
				}
				var evt cluster.Event
				if err := json.Unmarshal([]byte(m.Payload), &evt); err != nil {
					continue
				}
				fmt.Printf("[RedisManager(nodeID=%s)] Received event %s\n", rm.nodeID, m.Payload)
				rm.deliver(&evt)
			case <-subCtx.Done():
				return
			}
		}
	}()

	return rm, nil
}

func (r *RedisManager) Subscribe(ctx context.Context, nodeID string) (cluster.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Printf("[RedisManager(nodeID=%s)] Registering node %s\n", r.nodeID, nodeID)
	ch := make(chan *cluster.Event, 256)
	r.nodes[nodeID] = ch
	sub := &RedisSubscription{
		ch: ch,
		closeFn: func() error {
			return r.unsubscribe(nodeID)
		},
	}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = sub.Close()
		}()
	}
	return sub, nil
}

func (r *RedisManager) Publish(evt *cluster.Event) error {
	if evt == nil {
		return errors.New("publish: nil event")
	}
	if evt.Origin == "" {
		evt.Origin = r.nodeID
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	fmt.Printf("[RedisManager(nodeID=%s)] Publishing event %s\n", r.nodeID, string(b))
	return r.client.Publish(context.Background(), r.topic, b).Err()
}

func (r *RedisManager) deliver(evt *cluster.Event) {
	if evt == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, ch := range r.nodes {
		select {
		case ch <- evt:
			fmt.Printf("[RedisManager(nodeID=%s)] Delivered event from node %s to node %s\n", r.nodeID, evt.Origin, id)
		default:
			fmt.Printf("[RedisManager(nodeID=%s)] Dropped event from node %s to node %s\n", r.nodeID, evt.Origin, id)
		}
	}
}

func (r *RedisManager) unsubscribe(nodeID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, ok := r.nodes[nodeID]
	if !ok {
		return nil
	}

	delete(r.nodes, nodeID)
	close(ch)
	fmt.Printf("[RedisManager(nodeID=%s)] Unregistered node %s\n", r.nodeID, nodeID)
	return nil
}
