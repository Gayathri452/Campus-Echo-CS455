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
