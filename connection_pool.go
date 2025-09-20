// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package gosocket

import (
	"sync"
)

type ConnectionPool struct {
	maxConnections      int
	maxConnectionsPerIP int
	activeConns         map[string]int // IP -> connection count
	totalActive         int
	mu                  sync.RWMutex
	semaphore           chan struct{} // to limit the number of active connections
}

func NewConnectionPool(maxTotal, maxPerIP int) *ConnectionPool {
	return &ConnectionPool{
		maxConnections:      maxTotal,
		maxConnectionsPerIP: maxPerIP,
		activeConns:         make(map[string]int),
		semaphore:           make(chan struct{}, maxTotal),
	}
}

func (cp *ConnectionPool) AcquireConnection(clientIP string) error {
	// try to acquire a slot (block if limit is reached)
	select {
	case cp.semaphore <- struct{}{}:
	default:
		return ErrMaxConnReached
	}

	cp.mu.Lock()
	defer cp.mu.Unlock()

	// check if the maximum number of connections per IP has been reached
	if cp.activeConns[clientIP] >= cp.maxConnectionsPerIP {
		<-cp.semaphore // release the slot
		return newMaxConnPerIpReachedError(clientIP)
	}

	cp.activeConns[clientIP]++
	cp.totalActive++
	return nil
}

func (cp *ConnectionPool) ReleaseConnection(clientIP string) {
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
