// Stockyard Corral — Webhook relay and debugger.
// Receive, log, replay, and forward webhooks. Self-hosted.
// Single binary, embedded SQLite, zero external dependencies.
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/stockyard-dev/stockyard-corral/internal/server"
	"github.com/stockyard-dev/stockyard-corral/internal/store"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Printf("corral %s\n", version)
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

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./data"
	}

	db, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	log.Printf("")
	log.Printf("  Stockyard Corral %s", version)
	log.Printf("  Webhook relay:  http://localhost:%d/hook/{endpoint_id}", port)
	log.Printf("  API:            http://localhost:%d/api", port)
	log.Printf("  Live stream:    http://localhost:%d/api/endpoints/{id}/stream", port)
	log.Printf("")

	srv := server.New(db, port)
	if err := srv.Start(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
