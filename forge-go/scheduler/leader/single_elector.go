package leader

import (
	"context"
	"sync/atomic"
)

// SingleNodeElector is a trivial LeaderElector for single-node / embedded
// deployments where no distributed coordination is needed. It immediately
// claims leadership on Acquire and blocks until the context is cancelled.
type SingleNodeElector struct {
	isLeader  atomic.Bool
	watchChan chan bool
}

var _ LeaderElector = (*SingleNodeElector)(nil)

func NewSingleNodeElector() *SingleNodeElector {
	return &SingleNodeElector{
		watchChan: make(chan bool, 1),
	}
}

func (s *SingleNodeElector) Acquire(ctx context.Context) error {
	s.setLeader(true)
	<-ctx.Done()
	s.setLeader(false)
	return ctx.Err()
}

func (s *SingleNodeElector) IsLeader() bool {
	return s.isLeader.Load()
}

func (s *SingleNodeElector) Resign(_ context.Context) error {
	s.setLeader(false)
	return nil
}

func (s *SingleNodeElector) setLeader(state bool) {
	if s.isLeader.CompareAndSwap(!state, state) {
		select {
		case s.watchChan <- state:
		default:
			<-s.watchChan
			s.watchChan <- state
		}
	}
}

func (s *SingleNodeElector) Watch() <-chan bool {
	return s.watchChan
}
