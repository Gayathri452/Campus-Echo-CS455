package triage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/genai"
)

type geminiEmbeddingClient interface {
	EmbedContent(context.Context, string, []*genai.Content, *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error)
}

type geminiSDKEmbeddingClient struct {
	client *genai.Client
}

func (c geminiSDKEmbeddingClient) EmbedContent(ctx context.Context, model string, contents []*genai.Content, config *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
	return c.client.Models.EmbedContent(ctx, model, contents, config)
}

type GeminiSemanticEmbedder struct {
	client  geminiEmbeddingClient
	model   string
	apiKey  string
	timeout time.Duration
}

func (e *GeminiSemanticEmbedder) Model() string {
	return e.model
}

func NewGeminiSemanticEmbedder(apiKey, model string, timeout time.Duration) (*GeminiSemanticEmbedder, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrGeminiProviderNotConfigured
	}
	if model == "" {
		model = DefaultGeminiEmbeddingModel
	}
	if timeout <= 0 {
		timeout = DefaultProcessTimeout
	}
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			Timeout:      &timeout,
			RetryOptions: &genai.HTTPRetryOptions{Attempts: genai.Ptr(int32(1))},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create Gemini embedding client: %w", err)
	}
	return &GeminiSemanticEmbedder{
		client:  geminiSDKEmbeddingClient{client: client},
		model:   model,
		apiKey:  apiKey,
		timeout: timeout,
	}, nil
}

func (e *GeminiSemanticEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.client == nil {
		return nil, ErrGeminiProviderNotConfigured
	}
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if len(texts) > maxEmbeddingBatchSize {
		return nil, fmt.Errorf("embedding batch size %d exceeds limit %d", len(texts), maxEmbeddingBatchSize)
	}
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = genai.NewContentFromText(text, genai.RoleUser)
	}
	requestCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	response, err := e.client.EmbedContent(
		requestCtx,
		e.model,
		contents,
		&genai.EmbedContentConfig{TaskType: "SEMANTIC_SIMILARITY"},
	)
	if err != nil {
		return nil, classifyGeminiEmbeddingError(err, requestCtx, e.apiKey)
	}
	if response == nil || len(response.Embeddings) != len(texts) {
		return nil, fmt.Errorf("Gemini semantic embedding returned %d vectors for %d reviews", embeddingCount(response), len(texts))
	}
	embeddings := make([][]float32, len(response.Embeddings))
	for i, embedding := range response.Embeddings {
		if embedding == nil || !validEmbedding(embedding.Values) {
			return nil, fmt.Errorf("Gemini semantic embedding %d was empty or invalid", i)
		}
		embeddings[i] = embedding.Values
	}
	return embeddings, nil
}

func embeddingCount(response *genai.EmbedContentResponse) int {
	if response == nil {
		return 0
	}
	return len(response.Embeddings)
}

func classifyGeminiEmbeddingError(err error, requestCtx context.Context, apiKey string) error {
	var apiError genai.APIError
	var apiErrorPointer *genai.APIError
	if errors.As(err, &apiError) && apiError.Code != 0 {
		return &ProviderFailure{
			Kind:      "http",
			Status:    apiError.Code,
			Retryable: retryableGeminiStatus(apiError.Code),
			Message:   redactGeminiError(apiError.Message, apiKey, ""),
		}
	}
	if errors.As(err, &apiErrorPointer) && apiErrorPointer != nil && apiErrorPointer.Code != 0 {
		return &ProviderFailure{
			Kind:      "http",
			Status:    apiErrorPointer.Code,
			Retryable: retryableGeminiStatus(apiErrorPointer.Code),
			Message:   redactGeminiError(apiErrorPointer.Message, apiKey, ""),
		}
	}
	if errors.Is(requestCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &ProviderFailure{Kind: "deadline exceeded", Retryable: true}
	}
	if errors.Is(requestCtx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return &ProviderFailure{Kind: "canceled"}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return &ProviderFailure{
			Kind:      "network error",
			Retryable: networkError.Timeout() || networkError.Temporary(),
			Message:   redactGeminiError(networkError.Error(), apiKey, ""),
		}
	}
	return &ProviderFailure{
		Kind:    "failed",
		Message: redactGeminiError(err.Error(), apiKey, ""),
	}
}
