// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package ids

/**
 * ids.go is responsible for generating and managing unique IDs.
 * Generate NodeIDs used to identify GoSocket Runtime instances within a cluster.
 *
 * These identifiers are used for event origin tracking and loop
 * prevention in distributed environments.
 *
 * This file should provide deterministic and collision-safe identifiers.
 *
 * MUST NOT contain cluster logic, persistence logic, or runtime state.
 */

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

var clientCounter atomic.Uint64

// GetNodeID returns a unique node ID as a string.
// If the node ID is not provided, it will be try to fetch it from an environment variable (GOSOCKET_NODE_ID)
// or generate a new one.
// The node ID is used to identify GoSocket Runtime instances within a cluster.
func GetNodeID(id string) string {
	// NodeID precedence: explicit config > env var > generated
	nodeID := id
	if nodeID == "" {
		nodeID = os.Getenv("GOSOCKET_NODE_ID")
	}
	if nodeID == "" {
		uuid, err := uuid.NewV7()
		if err != nil {
			nodeID = fmt.Sprintf("%d", time.Now().UnixNano())
		} else {
			nodeID = uuid.String()
		}
	}

	return nodeID
}

// GenerateClientID returns a unique client ID as a string.
func GenerateClientID() string {
	return fmt.Sprintf("client_%d_%d", time.Now().UnixNano(), clientCounter.Add(1))
}

// GenerateRequestID returns a unique request ID as a string. The request ID
// is a concatenation of the current time in nanoseconds and a random number
// between 0 and 9999. The request ID is used to identify requests and is
// passed to the OnRequest handler if it is not nil. The request ID can be
// used to identify requests in logs, metrics, and other monitoring tools.
func GenerateRequestID() string {
	randN, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("req_%d_%d", time.Now().UnixNano(), randN)
}

// GenerateNodeID returns a unique node ID as a string.
func GenerateNodeID() uuid.UUID {
	uuid, err := uuid.NewV7()
	if err != nil {
		return uuid
	}
	return uuid
}
