package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/oauth2/google"
)

const (
	// VERTEX_API_ENDPOINT = "https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/%s/models/%s:generateContent"
	VERTEX_API_OPENAI_ENDPOINT = "https://${ENDPOINT}/v1/projects/${PROJECT_ID}/locations/${REGION}/endpoints/openapi/chat/completions"
	VERTEX_SCOPES              = "https://www.googleapis.com/auth/cloud-platform"
)

// VertexClient handles direct API calls to Vertex AI using service account credentials
type VertexClient struct {
	projectID   string
	location    string
	publisher   string
	modelName   string
	credentials *google.Credentials
	httpClient  *http.Client
	mu          sync.RWMutex
}

// GlobalVertexClient is the global instance of VertexClient
var GlobalVertexClient *VertexClient

// VertexRequest represents the request body for Vertex AI generateContent
type VertexRequest struct {
	Contents         []VertexContent         `json:"contents"`
	GenerationConfig *VertexGenerationConfig `json:"generationConfig,omitempty"`
}

type VertexContent struct {
	Role  string       `json:"role"`
	Parts []VertexPart `json:"parts"`
}

type VertexPart struct {
	Text string `json:"text"`
}

type VertexGenerationConfig struct {
	Temperature     float32 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	TopP            float32 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
}

// VertexResponse represents the response from Vertex AI generateContent
type VertexResponse struct {
	Candidates    []VertexCandidate `json:"candidates"`
	UsageMetadata *VertexUsage      `json:"usageMetadata,omitempty"`
}

type VertexCandidate struct {
	Content       VertexContent `json:"content"`
	FinishReason  string        `json:"finishReason,omitempty"`
	SafetyRatings []interface{} `json:"safetyRatings,omitempty"`
}

type VertexUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// NewVertexClient creates a new VertexClient using service account credentials
func NewVertexClient(ctx context.Context, credentialsFile, location, publisher, modelName string) (*VertexClient, error) {
	// Read credentials file
	credData, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Parse credentials to get project ID
	var credJSON struct {
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(credData, &credJSON); err != nil {
		return nil, fmt.Errorf("failed to parse credentials JSON: %w", err)
	}

	if credJSON.ProjectID == "" {
		return nil, fmt.Errorf("project_id not found in credentials file")
	}

	// Create credentials with the required scope
	creds, err := google.CredentialsFromJSON(ctx, credData, VERTEX_SCOPES)
	if err != nil {
		return nil, fmt.Errorf("failed to create credentials: %w", err)
	}

	client := &VertexClient{
		projectID:   credJSON.ProjectID,
		location:    location,
		publisher:   publisher,
		modelName:   modelName,
		credentials: creds,
		httpClient:  &http.Client{},
	}

	slog.Info("VertexClient initialized",
		"project_id", client.projectID,
		"location", location,
		"publisher", publisher,
		"model", modelName,
	)

	return client, nil
}

// getAccessToken retrieves a valid access token from the credentials
func (c *VertexClient) getAccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	token, err := c.credentials.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	return token.AccessToken, nil
}

// buildEndpoint constructs the full API endpoint URL
func (c *VertexClient) buildEndpoint() string {
	return strings.NewReplacer(
		"${ENDPOINT}", "aiplatform.googleapis.com",
		"${PROJECT_ID}", c.projectID,
		"${REGION}", c.location,
	).Replace(VERTEX_API_OPENAI_ENDPOINT)
}

// GenerateContent sends a request to Vertex AI and returns the generated text
func (c *VertexClient) GenerateContent(ctx context.Context, prompt string, temperature float32, maxOutputTokens int) (string, error) {
	// Get access token
	accessToken, err := c.getAccessToken(ctx)
	if err != nil {
		return "", err
	}

	// Build request body
	reqBody := VertexRequest{
		Contents: []VertexContent{
			{
				Role: "user",
				Parts: []VertexPart{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &VertexGenerationConfig{
			Temperature:     temperature,
			MaxOutputTokens: maxOutputTokens,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := c.buildEndpoint()
	slog.Debug("Calling Vertex AI endpoint", "endpoint", endpoint)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error status
	if resp.StatusCode != http.StatusOK {
		slog.Error("Vertex AI API error",
			"status_code", resp.StatusCode,
			"response", string(body),
		)
		return "", fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var vertexResp VertexResponse
	if err := json.Unmarshal(body, &vertexResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Log token usage if available
	if vertexResp.UsageMetadata != nil {
		slog.Info("Token usage",
			"prompt_tokens", vertexResp.UsageMetadata.PromptTokenCount,
			"output_tokens", vertexResp.UsageMetadata.CandidatesTokenCount,
			"total_tokens", vertexResp.UsageMetadata.TotalTokenCount,
		)
	}

	// Extract text from response
	if len(vertexResp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in response")
	}

	var textParts []string
	for _, part := range vertexResp.Candidates[0].Content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
	}

	if len(textParts) == 0 {
		return "", fmt.Errorf("no text in response")
	}

	return strings.Join(textParts, ""), nil
}

// InitGlobalVertexClient initializes the global VertexClient
func InitGlobalVertexClient(ctx context.Context, credentialsFile, location, publisher, modelName string) error {
	client, err := NewVertexClient(ctx, credentialsFile, location, publisher, modelName)
	if err != nil {
		return err
	}
	GlobalVertexClient = client
	return nil
}
