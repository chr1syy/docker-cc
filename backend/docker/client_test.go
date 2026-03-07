//go:build integration

package docker

import (
    "context"
    "testing"
)

// Simple integration test to ensure New() can create a Docker client
// when the Docker socket is available. This test only runs when the
// 'integration' build tag is provided: `go test -tags=integration ./...`.
func TestNewIntegration(t *testing.T) {
    ctx := context.Background()
    c, err := New()
    if err != nil {
        t.Fatalf("New() returned error: %v", err)
    }
    if c == nil || c.c == nil {
        t.Fatalf("unexpected nil client")
    }
    if err := c.Close(); err != nil {
        t.Fatalf("Close() returned error: %v", err)
    }
    _ = ctx
}
