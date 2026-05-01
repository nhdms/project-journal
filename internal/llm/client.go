// Package llm wraps OpenAI calls used by project-journal Phase 2 features:
// trajectory induction, autoeval, and embeddings.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

const (
	envAPIKey     = "OPENAI_API_KEY"
	envChatModel  = "OPENAI_CHAT_MODEL"
	envEmbedModel = "OPENAI_EMBED_MODEL"
	envTimeout    = "OPENAI_TIMEOUT_SECONDS"
	envBaseURL    = "OPENAI_BASE_URL"

	defaultChatModel  = "gpt-4o-mini"
	defaultEmbedModel = "text-embedding-3-small"
	defaultTimeoutSec = 60
)

// Client wraps a configured OpenAI client with model defaults.
type Client struct {
	api        *openai.Client
	chatModel  string
	embedModel string
	timeout    time.Duration
}

// HasAPIKey reports whether the env var is set (non-empty).
// Use to gate LLM-dependent flows.
func HasAPIKey() bool {
	return os.Getenv(envAPIKey) != ""
}

// NewClient builds a Client from environment variables. Returns an error
// if OPENAI_API_KEY is not set.
func NewClient() (*Client, error) {
	key := os.Getenv(envAPIKey)
	if key == "" {
		return nil, fmt.Errorf("%s not set", envAPIKey)
	}
	chatModel := envOrDefault(envChatModel, defaultChatModel)
	embedModel := envOrDefault(envEmbedModel, defaultEmbedModel)

	timeoutSec := defaultTimeoutSec
	if v := os.Getenv(envTimeout); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeoutSec = n
		}
	}

	cfg := openai.DefaultConfig(key)
	if base := os.Getenv(envBaseURL); base != "" {
		cfg.BaseURL = base
	}
	api := openai.NewClientWithConfig(cfg)

	return &Client{
		api:        api,
		chatModel:  chatModel,
		embedModel: embedModel,
		timeout:    time.Duration(timeoutSec) * time.Second,
	}, nil
}

// ChatModel returns the configured chat completion model.
func (c *Client) ChatModel() string { return c.chatModel }

// EmbedModel returns the configured embedding model.
func (c *Client) EmbedModel() string { return c.embedModel }

// Timeout returns the per-request timeout.
func (c *Client) Timeout() time.Duration { return c.timeout }

// ChatJSON runs a chat completion in JSON mode and unmarshals the response
// into out. Returns an error if the API call fails or JSON cannot be parsed
// (in which case the raw response is included in the error).
func (c *Client) ChatJSON(ctx context.Context, systemPrompt, userPrompt string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.chatModel,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	})
	if err != nil {
		return fmt.Errorf("openai chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return fmt.Errorf("openai chat: empty choices")
	}
	raw := resp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("openai chat: json parse failed: %w; raw=%q", err, raw)
	}
	return nil
}

// Embed returns an embedding vector for text using the configured embed model.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.api.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.embedModel),
		Input: []string{text},
	})
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("openai embed: empty result")
	}
	return resp.Data[0].Embedding, nil
}

func envOrDefault(key, dflt string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return dflt
}
