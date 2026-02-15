// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package runtime

/**
 * runtime.go defines the Runtime, which represents a single live GoSocket
 * instance.
 *
 * The Runtime is responsible for owning the Hub, Dispatcher, ClusterManager,
 * and coordinating message flow and distributed behavior.
 *
 * It is the core execution unit of GoSocket and exists independently of
 * how the system is exposed (standalone server or embedded handler).
 *
 * MUST NOT perform network I/O, accept connections, or depend on transport-
 * specific concerns such as HTTP or WebSocket.
 */

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/FilipeJohansson/gosocket/internal/cluster"
	"github.com/FilipeJohansson/gosocket/internal/cluster/store"
	"github.com/FilipeJohansson/gosocket/internal/dispatcher"
	"github.com/FilipeJohansson/gosocket/internal/hub"
	"github.com/FilipeJohansson/gosocket/internal/ids"
	"github.com/FilipeJohansson/gosocket/internal/message"
	"github.com/FilipeJohansson/gosocket/internal/utils"
)

type Runtime struct {
	hub        *hub.Hub              // pure domain (clients, rooms)
	dispatcher dispatcher.Dispatcher // single message output path

	// clustering
	cluster      cluster.Manager  // distributed infrastructure (optional)
	clusterState store.StateStore // distributed global state (optional)
	nodeID       string           // instance identity in cluster

	ctx     context.Context
	cancel  context.CancelFunc
	started atomic.Bool // lifecycle protection
}

type Config struct {
	HubConfig *hub.HubConfig

	Cluster cluster.Manager
	State   store.StateStore
	NodeID  string
}

func NewRuntime(cfg Config) (*Runtime, error) {
	var nodeID string
	if cfg.NodeID == "" {
		nodeID = ids.GenerateNodeID().String()
		if nodeID == "" {
			return nil, errors.New("nodeID is required") // TODO: return better error
		}
	} else {
		nodeID = cfg.NodeID
	}

	if cfg.Cluster == nil {
		cfg.Cluster = cluster.NewNoopManager()
	}
	if cfg.State == nil {
		cfg.State = store.NewNoopStateStore()
	}

	defaultHubCfg := hub.DefaultHubConfig()
	if cfg.HubConfig == nil {
		cfg.HubConfig = defaultHubCfg
	}
	if cfg.HubConfig.Logger == nil {
		cfg.HubConfig.Logger = defaultHubCfg.Logger
	}

	if cfg.HubConfig.BackpressurePolicy == 0 {
		cfg.HubConfig.BackpressurePolicy = defaultHubCfg.BackpressurePolicy
	}

	return &Runtime{
		hub:          hub.NewHub(cfg.HubConfig),
		cluster:      cfg.Cluster,
		clusterState: cfg.State,
		nodeID:       nodeID,
		started:      atomic.Bool{},
	}, nil
}

func (r *Runtime) Start(
	serializers map[message.EncodingType]message.Serializer,
	parent context.Context,
	cancel context.CancelFunc,
) error {
	if r.started.Load() {
		return nil
	}

	r.ctx = parent
	r.cancel = cancel

	utils.SafeGoroutine("HubRun", func() {
		r.hub.Run(r.ctx)
	})

	dispatcher, err := r.initDispatcher(serializers)
	if err != nil {
		return err
	}
	r.dispatcher = dispatcher

	r.started.Store(true)
	return nil
}

func (r *Runtime) Stop() error {
	if !r.started.Load() {
		return nil
	}

	if r.cancel != nil {
		r.cancel()
	}

	if r.dispatcher != nil {
		err := r.dispatcher.Stop()
		if err != nil {
			return err
		}
	}

	r.started.Store(false)
	return nil
}

func (r *Runtime) Hub() *hub.Hub {
	return r.hub
}

func (r *Runtime) Dispatcher() dispatcher.Dispatcher {
	return r.dispatcher
}

func (r *Runtime) initDispatcher(serializers map[message.EncodingType]message.Serializer) (dispatcher.Dispatcher, error) {
	localDispatcher := dispatcher.NewLocalDispatcher(r.hub, serializers)

	if r.cluster != nil {
		if _, isNoop := r.cluster.(*cluster.NoopManager); !isNoop {
			dispatcher := dispatcher.NewClusterDispatcher(localDispatcher, r.cluster, r.clusterState, r.nodeID)
			err := dispatcher.Start(r.ctx)
			if err != nil {
				return nil, err
			}

			return dispatcher, nil
		}
	}

	err := localDispatcher.Start(r.ctx)
	if err != nil {
		return nil, err
	}

	return localDispatcher, nil
}
