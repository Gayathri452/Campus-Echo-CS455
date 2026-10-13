package triage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

func TestAnthropicProviderUsesStructuredClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("request = %s %s, want POST /v1/messages", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-secret" {
			t.Error("Anthropic API key header was not set")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("Anthropic version header was not set")
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "test-model" || len(request.Messages) != 1 || len(request.Tools) != 1 {
			t.Errorf("request is missing model, review message, or classification tool: %+v", request)
		}
		if request.Messages[0].Content != "The grading was unfair." {
			t.Errorf("review text = %q", request.Messages[0].Content)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"tool_use","name":"classify_review","input":{"decision":"flag","reasonCode":"harassment","explanation":"Contains targeted harassment."}}]}`))
	}))
	defer server.Close()

	provider := NewAnthropicProvider("test-secret", "test-model", server.URL+"/v1/messages", 10*time.Second)
	result, err := provider.Classify(context.Background(), feedback.Feedback{Content: "The grading was unfair."})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if result.Decision != DecisionFlag || result.ReasonCode != "harassment" || result.Explanation != "Contains targeted harassment." {
		t.Fatalf("classification = %+v", result)
	}
}

func TestAnthropicProviderReportsUnavailableAndMalformedResponses(t *testing.T) {
	t.Run("missing credentials", func(t *testing.T) {
		provider := NewAnthropicProvider("", "test-model", "", time.Second)
		if _, err := provider.Classify(context.Background(), feedback.Feedback{Content: "review"}); err != ErrProviderNotConfigured {
			t.Fatalf("error = %v, want ErrProviderNotConfigured", err)
		}
	})

	t.Run("rate limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		}))
		defer server.Close()
		provider := NewAnthropicProvider("test-secret", "test-model", server.URL, time.Second)
		_, err := provider.Classify(context.Background(), feedback.Feedback{Content: "review"})
		if err == nil || !strings.Contains(err.Error(), "HTTP 429") {
			t.Fatalf("error = %v, want HTTP 429", err)
		}
		if strings.Contains(err.Error(), "rate limited") || strings.Contains(err.Error(), "test-secret") {
			t.Fatalf("provider error exposed response or credential: %v", err)
		}
	})

	t.Run("missing tool response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"not a structured result"}]}`))
		}))
		defer server.Close()
		provider := NewAnthropicProvider("test-secret", "test-model", server.URL, time.Second)
		if _, err := provider.Classify(context.Background(), feedback.Feedback{Content: "review"}); err == nil {
			t.Fatal("Classify() succeeded without a classification tool response")
		}
	})

	t.Run("request timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			_, _ = w.Write([]byte(`{"content":[]}`))
		}))
		defer server.Close()
		provider := NewAnthropicProvider("test-secret", "test-model", server.URL, 5*time.Millisecond)
		if _, err := provider.Classify(context.Background(), feedback.Feedback{Content: "review"}); err == nil {
			t.Fatal("Classify() succeeded after its request timeout")
		}
	})
}
