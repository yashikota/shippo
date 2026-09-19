package daemon

import (
	"context"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Verbose enables debug logging when set to true.
var Verbose bool

// Run starts the daemon: loads eBPF, does initial sync, then enters event loop.
func Run() error {
	config := LoadConfig()
	log.Printf("allowed ports: %s", config.String())

	monitor, err := NewMonitor()
	if err != nil {
		return err
	}
	defer monitor.Close()

	reconciler := NewReconciler(config)
	reconciler.InitialSync()

	ctx, cancel := context.WithCancel(context.Background())
	var watcher sync.WaitGroup
	watcher.Add(1)
	go monitor.Run()
	go func() {
		defer watcher.Done()
		watchConfig(ctx, config, reconciler)
	}()
	defer func() {
		cancel()
		watcher.Wait()
		reconciler.Shutdown()
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	for {
		select {
		case ev, ok := <-monitor.Events():
			if !ok {
				return nil
			}
			reconciler.HandleEvent(ev)
		case s := <-sig:
			log.Printf("received %s, shutting down", s)
			return nil
		}
	}
}

// RunOnce does a single sync and returns.
func RunOnce() {
	config := LoadConfig()
	log.Printf("allowed ports: %s", config.String())

	reconciler := NewReconciler(config)
	reconciler.InitialSync()
}
