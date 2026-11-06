package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidateAllowsDocumentedLocalTriageSettings(t *testing.T) {
	cfg := Config{
		AppEnv:                   "development",
		TriageProvider:           "development-mock",
		BrigadingMode:            "development-noop",
		TriageWorkers:            2,
		TriageQueueSize:          64,
		TriageMaxRetries:         3,
		TriageRetryBackoff:       2 * time.Second,
		TriageProcessingTimeout:  25 * time.Second,
		TriageRecoveryInterval:   15 * time.Second,
		TriageLeaseDuration:      time.Minute,
		TriageDuplicateThreshold: 0.82,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestConfigValidateRejectsDevelopmentAdaptersOutsideDevelopment(t *testing.T) {
	cfg := Config{
		AppEnv:                   "production",
		TriageProvider:           "development-mock",
		BrigadingMode:            "development-noop",
		TriageWorkers:            1,
		TriageQueueSize:          1,
		TriageMaxRetries:         1,
		TriageRetryBackoff:       time.Second,
		TriageProcessingTimeout:  5 * time.Second,
		TriageRecoveryInterval:   time.Second,
		TriageLeaseDuration:      10 * time.Second,
		TriageDuplicateThreshold: 0.82,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() succeeded with development-only adapters in production")
	}
}

func TestConfigValidateAllowsGeminiWithCredentials(t *testing.T) {
	cfg := validConfig("gemini")
	cfg.GeminiAPIKey = "test-key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
}

func TestConfigValidateRequiresGeminiCredentials(t *testing.T) {
	cfg := validConfig("gemini")
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY is required") {
		t.Fatalf("Validate() error = %v, want explicit missing Gemini credential error", err)
	}
}

func TestConfigValidateRequiresGeminiKeyForSemanticBrigading(t *testing.T) {
	cfg := validConfig("development-mock")
	cfg.BrigadingMode = "gemini"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY is required") {
		t.Fatalf("Validate() error = %v, want missing key error for Gemini brigading", err)
	}
	cfg.GeminiAPIKey = "test-key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with Gemini brigading key = %v", err)
	}
}

func TestLoadDefaultsToGeminiTriage(t *testing.T) {
	t.Setenv("TRIAGE_PROVIDER", "")

	if cfg := Load(); cfg.TriageProvider != "gemini" {
		t.Fatalf("Load() TriageProvider = %q, want gemini", cfg.TriageProvider)
	}
}

func TestConfigValidateRejectsUnsupportedTriageProvider(t *testing.T) {
	cfg := validConfig("unsupported")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "TRIAGE_PROVIDER") {
		t.Fatalf("Validate() error = %v, want unsupported-provider error", err)
	}
}

func TestConfigValidateRejectsInvalidRuntimeLimits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"zero workers", func(cfg *Config) { cfg.TriageWorkers = 0 }},
		{"zero queue", func(cfg *Config) { cfg.TriageQueueSize = 0 }},
		{"negative retries", func(cfg *Config) { cfg.TriageMaxRetries = -1 }},
		{"zero backoff", func(cfg *Config) { cfg.TriageRetryBackoff = 0 }},
		{"zero processing timeout", func(cfg *Config) { cfg.TriageProcessingTimeout = 0 }},
		{"zero recovery interval", func(cfg *Config) { cfg.TriageRecoveryInterval = 0 }},
		{"lease not longer than timeout", func(cfg *Config) { cfg.TriageLeaseDuration = cfg.TriageProcessingTimeout }},
		{"threshold too low", func(cfg *Config) { cfg.TriageDuplicateThreshold = 0 }},
		{"threshold too high", func(cfg *Config) { cfg.TriageDuplicateThreshold = 1.1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig("development-mock")
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() succeeded for invalid runtime limit")
			}
		})
	}
}

func TestLoadReadsGeminiSettingsFromEnvironment(t *testing.T) {
	t.Setenv("TRIAGE_PROVIDER", "gemini")
	t.Setenv("BRIGADING_MODE", "gemini")
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("GEMINI_MODEL", "test-model")
	t.Setenv("GEMINI_EMBEDDING_MODEL", "test-embedding-model")

	cfg := Load()
	if cfg.TriageProvider != "gemini" || cfg.BrigadingMode != "gemini" ||
		cfg.GeminiAPIKey != "test-key" || cfg.GeminiModel != "test-model" ||
		cfg.GeminiEmbeddingModel != "test-embedding-model" {
		t.Fatalf("Load() Gemini configuration = provider %q, brigading mode %q, key present %t, model %q, embedding model %q",
			cfg.TriageProvider, cfg.BrigadingMode, cfg.GeminiAPIKey != "", cfg.GeminiModel, cfg.GeminiEmbeddingModel)
	}
}

func TestLoadParsesTypedEnvironmentValuesAndFallsBackOnInvalidValues(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("TRIAGE_WORKERS", "4")
	t.Setenv("TRIAGE_QUEUE_SIZE", "128")
	t.Setenv("TRIAGE_RETRY_BACKOFF", "3s")
	t.Setenv("TRIAGE_DUPLICATE_THRESHOLD", "0.91")

	cfg := Load()
	if !cfg.CookieSecure || cfg.TriageWorkers != 4 || cfg.TriageQueueSize != 128 ||
		cfg.TriageRetryBackoff != 3*time.Second || cfg.TriageDuplicateThreshold != 0.91 {
		t.Fatalf("Load() parsed values incorrectly: %+v", cfg)
	}

	t.Setenv("COOKIE_SECURE", "not-a-bool")
	t.Setenv("TRIAGE_WORKERS", "not-an-int")
	t.Setenv("TRIAGE_RETRY_BACKOFF", "not-a-duration")
	cfg = Load()
	if cfg.CookieSecure || cfg.TriageWorkers != 2 || cfg.TriageRetryBackoff != 2*time.Second {
		t.Fatalf("Load() fallback values incorrect: secure=%t workers=%d backoff=%s", cfg.CookieSecure, cfg.TriageWorkers, cfg.TriageRetryBackoff)
	}
}

func validConfig(provider string) Config {
	return Config{
		AppEnv:                   "development",
		TriageProvider:           provider,
		BrigadingMode:            "development-noop",
		TriageWorkers:            2,
		TriageQueueSize:          64,
		TriageMaxRetries:         3,
		TriageRetryBackoff:       2 * time.Second,
		TriageProcessingTimeout:  25 * time.Second,
		TriageRecoveryInterval:   15 * time.Second,
		TriageLeaseDuration:      time.Minute,
		TriageDuplicateThreshold: 0.82,
	}
}
