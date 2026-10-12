package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/config"
	"github.com/Kishore200524/CS455-Project/backend/internal/database"
	"github.com/Kishore200524/CS455-Project/backend/internal/httpapi"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	connectCtx, cancelConnect := context.WithTimeout(ctx, 10*time.Second)
	store, err := database.Connect(connectCtx, cfg.MongoURI, cfg.MongoDatabase)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("connect to MongoDB: %w", err)
	}
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := store.Disconnect(disconnectCtx); err != nil {
			log.Printf("disconnect from MongoDB: %v", err)
		}
	}()

	go appeals.RunExpiryWorker(ctx, store, appeals.DefaultSweepInterval)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewServer(store, auth.NewService(store), httpapi.WithAppealStore(store)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("CampusEcho API listening on %s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
	}

	return nil
}
