package agent

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WaitForShutdown blocks until a termination signal is received.
// On the first signal, it cancels the context and gracefully stops the agent.
// A subsequent signal or a timeout forces a hard exit.
func WaitForShutdown(cancel context.CancelFunc, a *Agent) {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received
	sig := <-sigs
	log.Printf("\nReceived signal: %s. Initiating graceful shutdown...", sig)

	// Issue cancellation to all downstream routines mapping to the primary context
	cancel()

	done := make(chan struct{})
	go func() {
		a.Stop()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Graceful shutdown complete.")
	case sig2 := <-sigs:
		log.Printf("Received second signal: %s. Forcing exit.", sig2)
		os.Exit(1)
	case <-time.After(15 * time.Second):
		log.Println("Graceful shutdown timed out after 15s. Forcing exit.")
		os.Exit(1)
	}
}
