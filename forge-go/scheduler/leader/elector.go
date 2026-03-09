package leader

import "context"

// LeaderElector defines the generic contract for cluster leader election.
type LeaderElector interface {
	// Acquire attempts to become the leader. It blocks until leadership is acquired,
	// the context is cancelled, or a fatal error occurs.
	Acquire(ctx context.Context) error

	// IsLeader returns true if the current node is the active leader.
	IsLeader() bool

	// Resign voluntarily gives up leadership.
	Resign(ctx context.Context) error

	// Watch provides a channel that emits `true` when leadership is gained
	// and `false` when leadership is lost.
	Watch() <-chan bool
}
