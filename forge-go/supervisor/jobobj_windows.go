//go:build windows

package supervisor

import "github.com/rustic-ai/forge/forge-go/protocol"

// applyResourceLimits applies Job Object limits on Windows.
func applyResourceLimits(pid int, spec *protocol.AgentSpec) error {
	// Stub implementation for Job Objects
	return nil
}
