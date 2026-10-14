package triage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

type fakeGeminiEmbeddingClient struct {
	response *genai.EmbedContentResponse
	err      error
	model    string
	contents []*genai.Content
	config   *genai.EmbedContentConfig
}

func (c *fakeGeminiEmbeddingClient) EmbedContent(_ context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	c.model = model
	c.contents = contents
	c.config = config
	return c.response, c.err
}

func TestGeminiSemanticEmbedderUsesSemanticSimilarityTask(t *testing.T) {
	client := &fakeGeminiEmbeddingClient{
		response: &genai.EmbedContentResponse{
			Embeddings: []*genai.ContentEmbedding{
				{Values: []float32{1, 0}},
				{Values: []float32{0, 1}},
			},
		},
	}
	embedder := &GeminiSemanticEmbedder{
		client: client, model: DefaultGeminiEmbeddingModel, timeout: time.Second,
	}

	embeddings, err := embedder.Embed(context.Background(), []string{"review one", "review two"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(embeddings) != 2 || client.model != DefaultGeminiEmbeddingModel ||
		len(client.contents) != 2 || client.config.TaskType != "SEMANTIC_SIMILARITY" {
		t.Fatalf("embedding request = model %q, contents %d, task %q, vectors %d",
			client.model, len(client.contents), client.config.TaskType, len(embeddings))
	}
	if embedder.Model() != DefaultGeminiEmbeddingModel {
		t.Fatalf("Model() = %q, want %q", embedder.Model(), DefaultGeminiEmbeddingModel)
	}
}

func TestGeminiSemanticEmbedderRejectsInvalidResponsesAndRedactsKey(t *testing.T) {
	tests := []struct {
		name     string
		response *genai.EmbedContentResponse
		err      error
	}{
		{
			name:     "wrong vector count",
			response: &genai.EmbedContentResponse{},
		},
		{
			name: "empty vector",
			response: &genai.EmbedContentResponse{
				Embeddings: []*genai.ContentEmbedding{{Values: []float32{0, 0}}},
			},
		},
		{name: "upstream error", err: errors.New("request failed with secret-key")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			embedder := &GeminiSemanticEmbedder{
				client: &fakeGeminiEmbeddingClient{response: test.response, err: test.err},
				model:  DefaultGeminiEmbeddingModel, apiKey: "secret-key", timeout: time.Second,
			}
			_, err := embedder.Embed(context.Background(), []string{"review"})
			if err == nil {
				t.Fatal("Embed() succeeded, want an error")
			}
			if strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("embedding error exposed API key: %v", err)
			}
		})
	}
}

func TestGeminiSemanticEmbedderClassifiesAPIStatusRetries(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: 404, retryable: false},
		{status: 503, retryable: true},
	} {
		t.Run(fmt.Sprintf("status_%d", test.status), func(t *testing.T) {
			embedder := &GeminiSemanticEmbedder{
				client: &fakeGeminiEmbeddingClient{err: &genai.APIError{Code: test.status, Message: "provider error"}},
				model:  DefaultGeminiEmbeddingModel, apiKey: "secret-key", timeout: time.Second,
			}
			_, err := embedder.Embed(context.Background(), []string{"review"})
			var failure *ProviderFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Embed() error = %T %v, want ProviderFailure", err, err)
			}
			if failure.Status != test.status || failure.Retryable != test.retryable {
				t.Fatalf("provider failure = %+v, want status %d retryable %t", failure, test.status, test.retryable)
			}
		})
	}
}

func TestGeminiSemanticEmbedderSmoke(t *testing.T) {
	if testing.Short() || getenv("GEMINI_SMOKE_TEST") != "1" {
		t.Skip("set GEMINI_SMOKE_TEST=1 to make a real Gemini embedding request")
	}
	apiKey := getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY must be set in the process environment for the smoke test")
	}
	model := getenv("GEMINI_EMBEDDING_MODEL")
	if model == "" {
		model = DefaultGeminiEmbeddingModel
	}
	embedder, err := NewGeminiSemanticEmbedder(apiKey, model, 90*time.Second)
	if err != nil {
		t.Fatalf("initialize Gemini semantic embedder: %v", err)
	}
	embeddings, err := embedder.Embed(context.Background(), []string{
		"The professor explained the concepts clearly.",
		"The instructor made the material easy to understand.",
	})
	if err != nil {
		t.Fatalf("Gemini embedding smoke request failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("Gemini returned %d embeddings, want 2", len(embeddings))
	}
	similarity, err := vectorCosineSimilarity(embeddings[0], embeddings[1])
	if err != nil || similarity <= 0 {
		t.Fatalf("semantic similarity = %f, %v; want a positive similarity", similarity, err)
	}
}
