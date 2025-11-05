package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	app "GWD/internal/app/server"
	"GWD/internal/logger"
	"GWD/internal/system"
)

func main() {
	log := logger.NewColoredLogger()

	if os.Geteuid() != 0 {
		log.Error("This program requires root privileges to run. Please run with sudo.")
		os.Exit(1)
	}

	cfg, err := system.LoadSystemConfig()
	if err != nil {
		log.Error("System detection failed: %v", err)
		os.Exit(1)
	}

	application, err := app.NewServer(cfg, log)
	if err != nil {
		log.Error("Failed to initialise server: %v", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Received exit signal, shutting down gracefully...")
		cancel()
	}()

	if err := application.Run(ctx); err != nil {
		log.Error("Application failed to run: %v", err)
		os.Exit(1)
	}

	log.Info("GWD deployment tool exited safely")
}
