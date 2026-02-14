// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package cluster

/**
 * manager.go defines the ClusterManager interface and core cluster
 * coordination behavior.
 *
 * It abstracts the underlying distributed messaging backend and exposes
 * a consistent API for node registration, unregistration, and event
 * publication.
 *
 * This file represents the control plane of distributed GoSocket instances.
 *
 * MUST NOT contain application logic, Hub logic, or transport-specific code.
 */

import (
	"context"

	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
)

type Subscription interface {
	Events() <-chan *Event
	Close() error
}

type Manager interface {
	Publish(evt *Event) error
	Subscribe(ctx context.Context, nodeID string) (Subscription, error)
}

type ClusterConfig struct {
	Manager Manager
	NodeID  string
	State   store.StateStore
}

type NoopManager struct{}

// NewNoopManager returns a ClusterManager that does nothing. Useful as a
// default so the hub can call into a cluster manager without checking for nil.
func NewNoopManager() Manager                   { return &NoopManager{} }
func (n *NoopManager) Publish(evt *Event) error { return nil }
func (n *NoopManager) Subscribe(ctx context.Context, nodeID string) (Subscription, error) {
	return nil, nil
}
