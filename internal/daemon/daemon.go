package daemon

import (
	"log"
	"os"
	"os/signal"
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

	go monitor.Run()
	go watchConfig(config, reconciler)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case ev, ok := <-monitor.Events():
			if !ok {
				return nil
			}
			reconciler.HandleEvent(ev)
		case s := <-sig:
			log.Printf("received %s, shutting down", s)
			reconciler.Shutdown()
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
