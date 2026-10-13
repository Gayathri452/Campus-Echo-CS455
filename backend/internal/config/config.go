package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                   string
	Port                     string
	MongoURI                 string
	MongoDatabase            string
	EmailMode                string
	SMTPHost                 string
	SMTPPort                 string
	SMTPUsername             string
	SMTPPassword             string
	SMTPFrom                 string
	CookieSecure             bool
	TriageProvider           string
	AnthropicAPIKey          string
	AnthropicModel           string
	AnthropicAPIURL          string
	GeminiAPIKey             string
	GeminiModel              string
	TriageWorkers            int
	TriageQueueSize          int
	TriageMaxRetries         int
	TriageRetryBackoff       time.Duration
	TriageProcessingTimeout  time.Duration
	TriageRecoveryInterval   time.Duration
	TriageLeaseDuration      time.Duration
	TriageDuplicateThreshold float64
	BrigadingMode            string
}

func Load() Config {
	loadDotEnv(".env")
	loadDotEnv("..\\.env")
	return Config{
		AppEnv:                   getEnv("APP_ENV", "development"),
		Port:                     getEnv("PORT", "8080"),
		MongoURI:                 getEnv("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase:            getEnv("MONGODB_DATABASE", "campusecho"),
		EmailMode:                getEnv("EMAIL_MODE", "smtp"),
		SMTPHost:                 getEnv("SMTP_HOST", ""),
		SMTPPort:                 getEnv("SMTP_PORT", "587"),
		SMTPUsername:             getEnv("SMTP_USERNAME", ""),
		SMTPPassword:             getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                 getEnv("SMTP_FROM", ""),
		CookieSecure:             getBoolEnv("COOKIE_SECURE", false),
		TriageProvider:           getEnv("TRIAGE_PROVIDER", "anthropic"),
		AnthropicAPIKey:          getEnv("ANTHROPIC_API_KEY", ""),
		AnthropicModel:           getEnv("ANTHROPIC_MODEL", "claude-haiku-5-5"),
		AnthropicAPIURL:          getEnv("ANTHROPIC_API_URL", "https://api.anthropic.com/v1/messages"),
		GeminiAPIKey:             getEnv("GEMINI_API_KEY", ""),
		GeminiModel:              getEnv("GEMINI_MODEL", "gemini-2.5-flash-lite"),
		TriageWorkers:            getIntEnv("TRIAGE_WORKERS", 2),
		TriageQueueSize:          getIntEnv("TRIAGE_QUEUE_SIZE", 64),
		TriageMaxRetries:         getIntEnv("TRIAGE_MAX_RETRIES", 3),
		TriageRetryBackoff:       getDurationEnv("TRIAGE_RETRY_BACKOFF", 2*time.Second),
		TriageProcessingTimeout:  getDurationEnv("TRIAGE_PROCESSING_TIMEOUT", 25*time.Second),
		TriageRecoveryInterval:   getDurationEnv("TRIAGE_RECOVERY_INTERVAL", 15*time.Second),
		TriageLeaseDuration:      getDurationEnv("TRIAGE_LEASE_DURATION", 1*time.Minute),
		TriageDuplicateThreshold: getFloatEnv("TRIAGE_DUPLICATE_THRESHOLD", 0.82),
		BrigadingMode:            getEnv("BRIGADING_MODE", "development-noop"),
	}
}

func (c Config) Validate() error {
	if c.TriageProvider != "anthropic" && c.TriageProvider != "gemini" && c.TriageProvider != "development-mock" {
		return fmt.Errorf("TRIAGE_PROVIDER must be anthropic, gemini, or development-mock")
	}
	if c.TriageProvider == "development-mock" && c.AppEnv != "development" {
		return fmt.Errorf("development-mock triage provider is only allowed when APP_ENV=development")
	}
	if c.TriageProvider == "gemini" && strings.TrimSpace(c.GeminiAPIKey) == "" {
		return fmt.Errorf("GEMINI_API_KEY is required when TRIAGE_PROVIDER=gemini")
	}
	if c.BrigadingMode != "development-noop" {
		return fmt.Errorf("BRIGADING_MODE must be development-noop until the brigading implementation is integrated")
	}
	if c.BrigadingMode == "development-noop" && c.AppEnv != "development" {
		return fmt.Errorf("development-noop brigading mode is only allowed when APP_ENV=development")
	}
	if c.TriageWorkers <= 0 || c.TriageQueueSize <= 0 || c.TriageMaxRetries < 0 {
		return fmt.Errorf("triage workers/queue must be positive and retries cannot be negative")
	}
	if c.TriageRetryBackoff <= 0 || c.TriageProcessingTimeout <= 0 || c.TriageRecoveryInterval <= 0 {
		return fmt.Errorf("triage backoff, timeout, and recovery interval must be positive")
	}
	if c.TriageLeaseDuration <= c.TriageProcessingTimeout {
		return fmt.Errorf("TRIAGE_LEASE_DURATION must exceed TRIAGE_PROCESSING_TIMEOUT")
	}
	if c.TriageDuplicateThreshold <= 0 || c.TriageDuplicateThreshold > 1 {
		return fmt.Errorf("TRIAGE_DUPLICATE_THRESHOLD must be in the range (0, 1]")
	}
	return nil
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"")
		if key != "" {
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getIntEnv(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
