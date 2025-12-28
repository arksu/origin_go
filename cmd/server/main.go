package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"origin/internal/config"
	"origin/internal/game"
	"origin/internal/network"
	"origin/internal/persistence"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting server with shard ID: %d", cfg.RegionID)

	// Initialize persistence layer
	db, err := persistence.NewPostgres(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer db.Close()
	log.Println("Connected to Postgres")

	// Create World
	world := game.NewWorld(cfg, db)

	// Start game loop
	world.Start()
	log.Printf("Game world started (tick rate: %v)", cfg.TickRate)

	// Create and start WebSocket server
	server := network.NewServer(cfg, world)

	// Start server in goroutine
	go server.Start()
	log.Printf("WebSocket server listening on %s", cfg.ListenAddr)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop WebSocket server
	if err := server.Stop(ctx); err != nil {
		log.Printf("Error stopping WebSocket server: %v", err)
	}

	// Stop game world (also persists entity ID state)
	world.Stop()

	log.Println("Server stopped")
}
