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
	"github.com/Kishore200524/CS455-Project/backend/internal/email"
	"github.com/Kishore200524/CS455-Project/backend/internal/httpapi"
	"github.com/Kishore200524/CS455-Project/backend/internal/triage"
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
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
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
	var provider triage.Provider
	switch cfg.TriageProvider {
	case "development-mock":
		provider = triage.NewDevelopmentMockProvider()
		log.Printf("WARNING: TRIAGE_PROVIDER=development-mock; this deterministic local classifier is not a real AI provider")
	case "gemini":
		geminiProvider, err := triage.NewGeminiProvider(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.TriageProcessingTimeout)
		if err != nil {
			return fmt.Errorf("initialize Gemini triage provider: %w", err)
		}
		provider = geminiProvider
		log.Printf("Gemini triage provider configured: model=%s; API key is present", cfg.GeminiModel)
	default:
		provider = triage.NewAnthropicProvider(cfg.AnthropicAPIKey, cfg.AnthropicModel, cfg.AnthropicAPIURL, cfg.TriageProcessingTimeout)
		if cfg.AnthropicAPIKey == "" {
			log.Printf("WARNING: ANTHROPIC_API_KEY is not configured; reviews remain Submitted and triage retries until a provider key is configured")
		}
	}
	log.Printf("WARNING: BRIGADING_MODE=development-noop; FR-8 brigading detection is not implemented")
	triageService := triage.NewService(
		store,
		provider,
		triage.WithWorkers(cfg.TriageWorkers),
		triage.WithQueueSize(cfg.TriageQueueSize),
		triage.WithMaxRetries(cfg.TriageMaxRetries),
		triage.WithRetryBackoff(cfg.TriageRetryBackoff),
		triage.WithProcessingTimeout(cfg.TriageProcessingTimeout),
		triage.WithRecoveryInterval(cfg.TriageRecoveryInterval),
		triage.WithLeaseDuration(cfg.TriageLeaseDuration),
		triage.WithDuplicateThreshold(cfg.TriageDuplicateThreshold),
		triage.WithBrigadingChecker(triage.NewDevelopmentNoopBrigadingChecker()),
	)
	triageService.Start(ctx)
	defer triageService.Stop()

	var emailService auth.EmailService
	switch cfg.EmailMode {
	case "console":
		emailService = email.Console{}
		log.Printf("WARNING: EMAIL_MODE=console; OTPs will be printed in this terminal")
	default:
		emailService = email.SMTP{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}
	}

	go appeals.RunExpiryWorker(ctx, store, appeals.DefaultSweepInterval)

	server := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: httpapi.NewServer(
			store,
			auth.NewService(store, emailService),
			httpapi.WithCookieSecure(cfg.CookieSecure),
			httpapi.WithAppealStore(store),
			httpapi.WithTriageDispatcher(triageService),
		),
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
