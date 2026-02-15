package ids

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetNodeID(t *testing.T) {
	t.Setenv("GOSOCKET_NODE_ID", "from-env")
	assert.Equal(t, "explicit", GetNodeID("explicit"))
	assert.Equal(t, "from-env", GetNodeID(""))

	t.Setenv("GOSOCKET_NODE_ID", "")
	generated := GetNodeID("")
	assert.NotEmpty(t, generated)
}

func TestGenerateClientID(t *testing.T) {
	a := GenerateClientID()
	b := GenerateClientID()
	assert.NotEqual(t, a, b)
	assert.True(t, strings.HasPrefix(a, "client_"))
}

func TestGenerateRequestID(t *testing.T) {
	reqID := GenerateRequestID()
	assert.True(t, strings.HasPrefix(reqID, "req_"))
}

func TestGenerateNodeID(t *testing.T) {
	nodeID := GenerateNodeID()
	assert.NotEqual(t, uuid.Nil, nodeID)
}
