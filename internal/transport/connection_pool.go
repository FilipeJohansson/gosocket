// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package transport

import (
	"sync"

	"github.com/FilipeJohansson/gosocket/internal/errors"
)

type ConnectionPoolConfig struct {
	MaxTotal int
	MaxPerIP int
}

func DefaultConnectionPoolConfig() ConnectionPoolConfig {
	return ConnectionPoolConfig{MaxTotal: 1000, MaxPerIP: 10}
}

type ConnectionPool struct {
	maxConnections      int
	maxConnectionsPerIP int
	activeConns         map[string]int // IP -> connection count
	totalActive         int
	mu                  sync.RWMutex
	semaphore           chan struct{} // to limit the number of active connections
}

func NewConnectionPool(cfg ConnectionPoolConfig) *ConnectionPool {
	return &ConnectionPool{
		maxConnections:      cfg.MaxTotal,
		maxConnectionsPerIP: cfg.MaxPerIP,
		activeConns:         make(map[string]int),
		semaphore:           make(chan struct{}, cfg.MaxTotal),
	}
}

func (cp *ConnectionPool) Acquire(clientIP string) error {
	// try to acquire a slot (block if limit is reached)
	select {
	case cp.semaphore <- struct{}{}:
	default:
		return errors.ErrMaxConnReached
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	// check if the maximum number of connections per IP has been reached
	if cp.activeConns[clientIP] >= cp.maxConnectionsPerIP {
		<-cp.semaphore // release the slot
		return errors.NewMaxConnPerIpReachedError(clientIP)
	}

	cp.activeConns[clientIP]++
	cp.totalActive++
	return nil
}

func (cp *ConnectionPool) Release(clientIP string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if count := cp.activeConns[clientIP]; count > 0 {
		cp.activeConns[clientIP]--
		if cp.activeConns[clientIP] == 0 {
			delete(cp.activeConns, clientIP)
		}
		cp.totalActive--
	}

	// release the slot
	<-cp.semaphore
}

func (cp *ConnectionPool) GetStats() (total int, perIP map[string]int) {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	ipCopy := make(map[string]int)
	for ip, count := range cp.activeConns {
		ipCopy[ip] = count
	}

	return cp.totalActive, ipCopy
}
