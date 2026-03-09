package logging

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "info")

	// Baseline
	logger.Info("Baseline log")
	assert.Contains(t, buf.String(), "Baseline log")
	buf.Reset()

	// With Context Variables
	ctx := context.Background()
	ctx = WithField(ctx, "guild_id", "test-guild-001")
	ctx = WithField(ctx, "agent_id", "test-agent-002")

	log := FromContext(ctx, logger)
	log.Info("Hello context")

	out := buf.String()
	assert.Contains(t, out, "Hello context")
	assert.Contains(t, out, "test-guild-001")
	assert.Contains(t, out, "test-agent-002")
}
