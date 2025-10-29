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
	log := logger.NewLogger()
	
	if os.Geteuid() != 0 {
		log.Fatal("This program requires root privileges to run. Please run with sudo.")
	}

	cfg, err := system.LoadSystemConfig()
	if err != nil {
		log.Fatal("System detection failed: %v", err)
	}

	application := app.NewServer(cfg, log)

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
		log.Fatal("Application failed to run: %v", err)
	}

	log.Info("GWD deployment tool exited safely")
}
