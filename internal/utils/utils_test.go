package utils

import (
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

func TestGetIPFromRequest(t *testing.T) {
	r1, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	r1.Header.Set("X-Real-Ip", "10.0.0.1")
	assert.Equal(t, "10.0.0.1", GetIPFromRequest(r1))

	r2, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	r2.Header.Set("X-Forwarded-For", "10.0.0.2:8080")
	assert.Equal(t, "10.0.0.2", GetIPFromRequest(r2))

	r3, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	r3.RemoteAddr = "10.0.0.3:9090"
	assert.Equal(t, "10.0.0.3", GetIPFromRequest(r3))
}

func TestExtractIP(t *testing.T) {
	assert.Equal(t, "", ExtractIP(nil))

	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	assert.Equal(t, "127.0.0.1", ExtractIP(addr))

	assert.Equal(t, "just-host", ExtractIP(fakeAddr("just-host")))
}

func TestExtractHeaders(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	r.Header.Set("Authorization", "Bearer token")
	r.Header.Set("X-Real-Ip", "10.0.0.1")
	r.Header.Set("X-Custom", "yes")

	headers := ExtractHeaders(r, "X-Custom")
	assert.Equal(t, "Bearer token", headers["Authorization"])
	assert.Equal(t, "10.0.0.1", headers["X-Real-Ip"])
	assert.Equal(t, "yes", headers["X-Custom"])
	assert.NotContains(t, headers, "Accept")
}
