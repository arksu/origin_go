package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"origin/internal/config"
	"origin/internal/game"
	"origin/internal/persistence"
	"origin/internal/restapi"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	cfg := config.MustLoad(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := persistence.NewPostgres(ctx, &cfg.Database, logger)
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	objectFactory := game.NewObjectFactory()
	objectFactory.RegisterBuilder(&game.TreeBuilder{})

	g := game.NewGame(cfg, db, objectFactory, logger)

	mux := http.NewServeMux()
	httpHandler := restapi.NewHandler(db, g.EntityIDManager(), logger)
	httpHandler.RegisterRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	if err := g.NetworkServer().Start(addr, mux); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}

	g.StartGameLoop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("Shutdown signal received")

	g.Stop()
}
