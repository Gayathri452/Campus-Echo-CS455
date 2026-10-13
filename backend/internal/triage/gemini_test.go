package triage

import (
	"context"
	"errors"
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
}

func (f *fakeGeminiClient) GenerateContent(_ context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.model = model
	f.contents = contents
	f.config = config
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

	secret := "must-not-appear-in-logs"
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

	provider = newGeminiProviderWithClient(&fakeGeminiClient{
		err: genai.APIError{Code: 429, Message: secret},
	}, "", time.Second)
	_, err = provider.Classify(context.Background(), feedback.Feedback{Content: "Review"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 429") || strings.Contains(err.Error(), secret) {
		t.Fatalf("Gemini API error = %v, want sanitized HTTP 429", err)
	}
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
	provider, err := NewGeminiProvider(apiKey, model, 30*time.Second)
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
