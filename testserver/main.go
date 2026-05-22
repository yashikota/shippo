package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := "3000"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	addr := "127.0.0.1:" + port

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello from %s\n", addr)
	})

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("listening on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
	srv.Close()
}
