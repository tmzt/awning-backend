package main

import (
	"awning-backend/common"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/oauth2/google"
)

const (
	// Vertex AI OpenAI-compatible endpoint
	// https://aiplatform.googleapis.com/v1/projects/{PROJECT_ID}/locations/{LOCATION}/endpoints/openapi/chat/completions
	VERTEX_ENDPOINT = "aiplatform.googleapis.com"
	VERTEX_REGION   = "global"
	VERTEX_SCOPE    = "https://www.googleapis.com/auth/cloud-platform"

	DEFAULT_VERTEX_MODEL = "qwen/qwen3-next-80b-a3b-thinking-maas"
)

// OpenAI-compatible request/response types
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason,omitempty"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

// Streaming response types (SSE format from OpenAI-compatible API)
type OpenAIStreamChoice struct {
	Index        int                `json:"index"`
	Delta        OpenAIMessageDelta `json:"delta"`
	FinishReason string             `json:"finish_reason,omitempty"`
}

type OpenAIMessageDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type OpenAIStreamResponse struct {
	ID      string               `json:"id"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   *OpenAIUsage         `json:"usage,omitempty"`
}

// StreamEvent represents an SSE event sent to the client
type StreamEvent struct {
	Type    string `json:"type"`    // "thinking", "content", "done", "error"
	Content string `json:"content"` // Text content for the event
}

// VertexOpenAIClient handles API calls to Vertex AI using OpenAI-compatible endpoint
type VertexOpenAIClient struct {
	projectID           string
	locationEndpoint    string
	completionsEndpoint string
	httpClient          *http.Client
	tokenSrc            func() (string, error)
	cfg                 *common.Config
}

// GlobalVertexOpenAIClient is the global instance
var GlobalVertexOpenAIClient *VertexOpenAIClient

// NewVertexOpenAIClient creates a client using service account credentials
func NewVertexOpenAIClient(ctx context.Context, cfg *common.Config, credData []byte) (*VertexOpenAIClient, error) {
	// credData, err := os.ReadFile(credentialsFile)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to read credentials: %w", err)
	// }

	var cred struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(credData, &cred); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	creds, err := google.CredentialsFromJSON(ctx, credData, VERTEX_SCOPE)
	if err != nil {
		return nil, fmt.Errorf("failed to create credentials: %w", err)
	}

	locationEndpoint := fmt.Sprintf("https://%s/v1/projects/%s/locations/%s", VERTEX_ENDPOINT, cred.ProjectID, VERTEX_REGION)

	// Build endpoint URL
	endpoint := fmt.Sprintf("%s/endpoints/openapi/chat/completions", locationEndpoint)

	client := &VertexOpenAIClient{
		projectID:           cred.ProjectID,
		completionsEndpoint: endpoint,
		httpClient:          &http.Client{},
		tokenSrc: func() (string, error) {
			token, err := creds.TokenSource.Token()
			if err != nil {
				return "", err
			}
			return token.AccessToken, nil
		},
		cfg: cfg,
	}

	slog.Info("VertexOpenAIClient initialized", "project_id", cred.ProjectID, "endpoint", endpoint)
	return client, nil
}

// GenerateContent sends a chat completion request
func (c *VertexOpenAIClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	token, err := c.tokenSrc()
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	model, ok := c.cfg.GetDefaultModel()
	if !ok {
		slog.Warn("Default model not in enabled models, using fallback", "default_model", DEFAULT_VERTEX_MODEL)
		model = DEFAULT_VERTEX_MODEL
	}

	slog.Debug("Using model for content generation", "model", model)

	reqBody := OpenAIChatRequest{
		Model: model,
		Messages: []OpenAIMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	slog.Debug("Calling Vertex AI", "endpoint", c.completionsEndpoint, "model", DEFAULT_VERTEX_MODEL)

	req, err := http.NewRequestWithContext(ctx, "POST", c.completionsEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("API error", "status", resp.StatusCode, "body", string(body))
		return "", fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var chatResp OpenAIChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if chatResp.Usage != nil {
		slog.Info("Token usage",
			"prompt", chatResp.Usage.PromptTokens,
			"completion", chatResp.Usage.CompletionTokens,
			"total", chatResp.Usage.TotalTokens)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// StreamCallback is called for each streaming event
type StreamCallback func(event StreamEvent) error

// GenerateContentStream sends a streaming chat completion request
func (c *VertexOpenAIClient) GenerateContentStream(ctx context.Context, prompt string, callback StreamCallback) error {
	token, err := c.tokenSrc()
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	model, ok := c.cfg.GetDefaultModel()
	if !ok {
		slog.Warn("Default model not in enabled models, using fallback", "default_model", DEFAULT_VERTEX_MODEL)
		model = DEFAULT_VERTEX_MODEL
	}

	slog.Debug("Using model for content generation", "model", model)

	reqBody := OpenAIChatRequest{
		Model: DEFAULT_VERTEX_MODEL,
		Messages: []OpenAIMessage{
			{Role: "user", Content: prompt},
		},
		Stream: true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	slog.Debug("Starting streaming request to Vertex AI", "endpoint", c.completionsEndpoint, "model", model)

	req, err := http.NewRequestWithContext(ctx, "POST", c.completionsEndpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("API error", "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Process SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// SSE format: "data: {...}" or "data: [DONE]"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			slog.Debug("Stream completed")
			break
		}

		// Parse the JSON chunk
		var streamResp OpenAIStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			slog.Warn("Failed to parse stream chunk", "error", err, "data", data)
			continue
		}

		if len(streamResp.Choices) == 0 {
			continue
		}

		delta := streamResp.Choices[0].Delta

		// Handle reasoning/thinking content
		if delta.ReasoningContent != "" {
			if err := callback(StreamEvent{Type: "thinking", Content: delta.ReasoningContent}); err != nil {
				return fmt.Errorf("callback error: %w", err)
			}
		}

		// Handle regular content
		if delta.Content != "" {
			if err := callback(StreamEvent{Type: "content", Content: delta.Content}); err != nil {
				return fmt.Errorf("callback error: %w", err)
			}
		}

		// Check for finish reason
		if streamResp.Choices[0].FinishReason != "" {
			slog.Debug("Stream finished", "reason", streamResp.Choices[0].FinishReason)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	return nil
}

// InitGlobalVertexOpenAIClient initializes the global client
func InitGlobalVertexOpenAIClient(ctx context.Context, cfg *common.Config, credData []byte) error {
	client, err := NewVertexOpenAIClient(ctx, cfg, credData)
	if err != nil {
		return err
	}
	GlobalVertexOpenAIClient = client
	return nil
}
