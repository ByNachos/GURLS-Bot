package main

import (
	"GURLS-Bot/internal/bot"
	"GURLS-Bot/internal/config"
	"GURLS-Bot/internal/grpc/client"
	"context"
	lg "log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

func main() {
	cfg := config.MustLoad()
	
	// Initialize logger
	var log *zap.Logger
	var err error
	if cfg.Env == "production" {
		log, err = zap.NewProduction()
	} else {
		log, err = zap.NewDevelopment()
	}
	if err != nil {
		lg.Fatalf("failed to create logger: %v", err)
	}
	defer func() {
		if err := log.Sync(); err != nil {
			lg.Printf("ERROR: failed to sync zap logger: %v\n", err)
		}
	}()

	log.Info("starting GURLS-Bot", zap.String("env", cfg.Env))

	// Initialize gRPC client to backend
	backendClient, err := client.NewBackendClient(
		cfg.GRPCClient.BackendAddress,
		cfg.GRPCClient.Timeout,
		log,
	)
	if err != nil {
		log.Fatal("failed to connect to backend", zap.Error(err))
	}
	defer backendClient.Close()

	// Initialize Telegram bot
	telegramBot, err := bot.New(cfg, log, backendClient)
	if err != nil {
		log.Fatal("failed to initialize bot", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start bot
	telegramBot.Start(ctx)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down GURLS-Bot...")

	cancel()
	log.Info("bot stopped")
}