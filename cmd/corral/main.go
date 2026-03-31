// Stockyard Corral — Webhook relay and debugger.
// Receive, log, replay, and forward webhooks. Self-hosted.
// Single binary. Embedded SQLite. Zero dependencies.
//
// License: Apache 2.0
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stockyard-dev/stockyard-corral/internal/server"
	"github.com/stockyard-dev/stockyard-corral/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Printf("stockyard-corral %s\n", version)
		os.Exit(0)
	}

	if len(os.Args) > 1 && (os.Args[1] == "--health" || os.Args[1] == "health") {
		fmt.Println("ok")
		os.Exit(0)
	}

	log.SetFlags(log.Ltime | log.Lshortfile)

	port := 8760
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	dataDir := "./data"
	if d := strings.TrimSpace(os.Getenv("DATA_DIR")); d != "" {
		dataDir = d
	}

	db, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	srv := server.New(db.Conn(), port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("server: %v", err)
		}
	}()

	log.Printf("")
	log.Printf("  Stockyard Corral (Webhook Relay)")
	log.Printf("  Capture:  http://localhost:%d/hook/{endpoint_id}", port)
	log.Printf("  API:      http://localhost:%d/api", port)
	log.Printf("  Live:     http://localhost:%d/api/live (SSE)", port)
	log.Printf("")

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
}
