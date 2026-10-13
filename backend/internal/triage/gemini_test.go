package triage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"google.golang.org/genai"
)

type fakeGeminiClient struct {
	response *genai.GenerateContentResponse
	err      error
	model    string
	contents []*genai.Content
	config   *genai.GenerateContentConfig
	generate func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

func (f *fakeGeminiClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.model = model
	f.contents = contents
	f.config = config
	if f.generate != nil {
		return f.generate(ctx, model, contents, config)
	}
	return f.response, f.err
}

func geminiTextResponse(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: genai.NewContentFromText(text, genai.RoleModel),
		}},
	}
}

func TestGeminiProviderUsesStructuredClassification(t *testing.T) {
	client := &fakeGeminiClient{response: geminiTextResponse(
		`{"decision":"publish","reasonCode":"acceptable","explanation":"Offers strong criticism of the grading policy."}`,
	)}
	provider := newGeminiProviderWithClient(client, "test-model", time.Second)

	result, err := provider.Classify(context.Background(), feedback.Feedback{
		Content: "The grading was unfair.",
		Rating:  1,
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if result.Decision != DecisionPublish || result.ReasonCode != "acceptable" {
		t.Fatalf("classification = %+v", result)
	}
	if client.model != "test-model" || len(client.contents) != 1 || client.contents[0].Parts[0].Text != "The grading was unfair." {
		t.Fatalf("request did not contain the selected model and review text: model=%q contents=%+v", client.model, client.contents)
	}
	if client.config.ResponseMIMEType != "application/json" || client.config.ResponseSchema == nil || client.config.ResponseSchema.Type != genai.TypeObject {
		t.Fatalf("request did not configure a JSON object schema: %+v", client.config)
	}
	minimalThinking := genai.ThinkingLevelMinimal
	client = &fakeGeminiClient{response: geminiTextResponse(
		`{"decision":"publish","reasonCode":"acceptable","explanation":"A negative opinion about the course."}`,
	)}
	provider = newGeminiProviderWithClient(client, "gemini-3-flash-lite", time.Second)
	if _, err := provider.Classify(context.Background(), feedback.Feedback{Content: "The course is badly organized."}); err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if client.config.ThinkingConfig == nil || client.config.ThinkingConfig.ThinkingLevel != minimalThinking {
		t.Fatalf("Gemini 3 thinking level = %+v, want minimal", client.config.ThinkingConfig)
	}
}

func TestGeminiProviderValidatesClassification(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "valid flag",
			response: `{"decision":"flag","reasonCode":"harassment","explanation":"Contains targeted harassment."}`,
		},
		{
			name:     "invalid decision",
			response: `{"decision":"reject","reasonCode":"other","explanation":"Invalid decision."}`,
			wantErr:  true,
		},
		{
			name:     "invalid publish reason",
			response: `{"decision":"publish","reasonCode":"","explanation":"Permissible."}`,
			wantErr:  true,
		},
		{
			name:     "invalid flag reason",
			response: `{"decision":"flag","reasonCode":"unfair_rating","explanation":"Low rating."}`,
			wantErr:  true,
		},
		{
			name:     "missing explanation",
			response: `{"decision":"publish","reasonCode":"acceptable","explanation":" "}`,
			wantErr:  true,
		},
		{
			name:     "unknown property",
			response: `{"decision":"publish","reasonCode":"acceptable","explanation":"Permissible.","extra":true}`,
			wantErr:  true,
		},
		{
			name:     "trailing JSON",
			response: `{"decision":"publish","reasonCode":"acceptable","explanation":"Permissible."} {}`,
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := newGeminiProviderWithClient(&fakeGeminiClient{
				response: geminiTextResponse(test.response),
			}, "", time.Second)
			_, err := provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
			if (err != nil) != test.wantErr {
				t.Fatalf("Classify() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestGeminiProviderReportsMissingConfigurationAndProviderErrors(t *testing.T) {
	if _, err := NewGeminiProvider("", "", time.Second); !errors.Is(err, ErrGeminiProviderNotConfigured) {
		t.Fatalf("NewGeminiProvider() error = %v, want missing-configuration error", err)
	}

	secret := "AIza0123456789012345678901234567890123"
	provider := newGeminiProviderWithClient(&fakeGeminiClient{
		err: errors.New(secret),
	}, "", time.Second)
	_, err := provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
	if err == nil || !strings.Contains(err.Error(), "Gemini API request failed") {
		t.Fatalf("Classify() error = %v, want a clear Gemini API error", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("provider error exposed sensitive information: %v", err)
	}

	provider = &GeminiProvider{
		client:  &fakeGeminiClient{err: genai.APIError{Code: 429, Message: secret}},
		model:   defaultGeminiModel,
		apiKey:  secret,
		timeout: time.Second,
	}
	_, err = provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), secret) {
		t.Fatalf("Gemini API error = %v, want sanitized HTTP 429", err)
	}
}

func TestGeminiProviderClassifiesUpstreamHTTPFailures(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: 503, retryable: true},
		{status: 504, retryable: true},
		{status: 401, retryable: false},
		{status: 400, retryable: false},
	} {
		status := test.status
		t.Run(fmt.Sprintf("HTTP_%d", status), func(t *testing.T) {
			reviewText := "Sensitive review content"
			provider := newGeminiProviderWithClient(&fakeGeminiClient{
				err: genai.APIError{Code: status, Message: "temporary failure " + reviewText},
			}, "test-model", time.Second)
			_, err := provider.Classify(context.Background(), feedback.Feedback{Content: reviewText})
			var providerFailure *ProviderFailure
			if !errors.As(err, &providerFailure) || providerFailure.Status != status || providerFailure.IsRetryable() != test.retryable {
				t.Fatalf("Classify() error = %v, want upstream HTTP %d (retryable=%t)", err, status, test.retryable)
			}
			if strings.Contains(err.Error(), reviewText) {
				t.Fatalf("provider error exposed review content: %v", err)
			}
		})
	}
}

func TestGeminiProviderDistinguishesDeadlineAndNetworkTimeout(t *testing.T) {
	t.Run("context deadline", func(t *testing.T) {
		client := &fakeGeminiClient{
			generate: func(ctx context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		provider := newGeminiProviderWithClient(client, "test-model", time.Millisecond)
		_, err := provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
		var providerFailure *ProviderFailure
		if !errors.As(err, &providerFailure) || providerFailure.Kind != "deadline exceeded" || !providerFailure.IsRetryable() {
			t.Fatalf("Classify() error = %v, want retryable local deadline", err)
		}
	})

	t.Run("network timeout", func(t *testing.T) {
		client := &fakeGeminiClient{
			generate: func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
				return nil, &net.DNSError{Err: "lookup timeout", IsTimeout: true, IsTemporary: true}
			},
		}
		provider := newGeminiProviderWithClient(client, "test-model", time.Second)
		_, err := provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
		var providerFailure *ProviderFailure
		if !errors.As(err, &providerFailure) || providerFailure.Kind != "network error" || !providerFailure.IsRetryable() {
			t.Fatalf("Classify() error = %v, want retryable network timeout", err)
		}
	})
}

func TestGeminiProviderSmoke(t *testing.T) {
	if testing.Short() || strings.TrimSpace(getenv("GEMINI_SMOKE_TEST")) != "1" {
		t.Skip("set GEMINI_SMOKE_TEST=1 to make a real Gemini API request")
	}
	apiKey := getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY must be set in the process environment for the smoke test")
	}
	model := getenv("GEMINI_MODEL")
	if model == "" {
		model = defaultGeminiModel
	}
	provider, err := NewGeminiProvider(apiKey, model, 90*time.Second)
	if err != nil {
		t.Fatalf("initialize Gemini provider: %v", err)
	}
	result, err := provider.Classify(context.Background(), feedback.Feedback{
		Content: "The course materials were clear, but the exam was unnecessarily difficult.",
	})
	if err != nil {
		t.Fatalf("Gemini smoke classification failed: %v", err)
	}
	if result.Decision != DecisionPublish || result.ReasonCode != "acceptable" {
		t.Fatalf("ordinary negative feedback was not accepted: %+v", result)
	}
}

func getenv(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}
