package cluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoopManager(t *testing.T) {
	mgr := NewNoopManager()
	require.NotNil(t, mgr)

	require.NoError(t, mgr.Publish(&Event{}))

	sub, err := mgr.Subscribe(context.Background(), "node-1")
	require.NoError(t, err)
	require.Nil(t, sub)
}

