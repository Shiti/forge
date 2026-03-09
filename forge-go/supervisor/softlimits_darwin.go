//go:build darwin

package supervisor

import "github.com/rustic-ai/forge/forge-go/protocol"

// applyResourceLimits applies soft limits on Darwin using setrlimit
func applyResourceLimits(pid int, spec *protocol.AgentSpec) error {
	// Stub implementation. For max processes / files, mac requires setrlimit on launch.
	return nil
}
