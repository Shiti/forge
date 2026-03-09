package gateway

import "github.com/rustic-ai/forge/forge-go/protocol"

// initMessageDefaults ensures list-typed message fields are always encoded as []
// (never null) to remain wire-compatible with Python pydantic validation.
// Priority and Timestamp derivation from ID is now handled by Message.Normalize().
func initMessageDefaults(m *protocol.Message) {
	if m == nil {
		return
	}
	m.Normalize()
	if len(m.Thread) == 0 {
		m.Thread = []uint64{m.ID}
	}
}
