package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"awning-backend/common"
	"awning-backend/model"
	"awning-backend/sections"
	"awning-backend/sections/common/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tiktoken-go/tokenizer"
)

const (
	MAX_INPUT_TOKENS  = 200000
	MAX_OUTPUT_TOKENS = 400000
	TOKEN_MODEL       = tokenizer.Cl100kBase
)

// Handler handles chat-related requests
type Handler struct {
	logger *slog.Logger
	deps   *sections.Dependencies
}

// NewHandler creates a new chat handler
func NewHandler(deps *sections.Dependencies) *Handler {
	return &Handler{
		logger: slog.With("handler", "ChatHandler"),
		deps:   deps,
	}
}

type SendSSEEvent func(c *gin.Context, eventType, data string)

func (h *Handler) streamVertexResponse(c *gin.Context, requestCtx context.Context, prompt string, chatID string, chatStage model.ChatStage, fullContent *strings.Builder, sendSSEEvent SendSSEEvent) error {
	// Send start message
	sendSSEEvent(c, "start", `{"message":"Starting response generation..."}`)

	// Send periodic placeholder event
	if !h.deps.Config.SendThinking {
		tmr := time.NewTicker(10 * time.Second)
		defer tmr.Stop()

		go func() {
			for {
				select {
				case <-tmr.C:
					h.logger.Info("Sending periodic thinking event")
					sendSSEEvent(c, "thinking", `{"message":"still thinking..."}`)
				case <-requestCtx.Done():
					return
				}
			}
		}()
	}

	// Stream response using Vertex AI
	err := h.deps.VertexClient.GenerateContentStream(requestCtx, prompt, func(event sections.StreamEvent) error {
		if !h.deps.Config.SendThinking && event.Type == "thinking" {
			return nil
		}

		if event.Type == "content" {
			fullContent.WriteString(event.Content)
			return nil
		}

		// Forward the event to the client
		eventJSON, _ := json.Marshal(map[string]string{
			"type":    event.Type,
			"content": event.Content,
		})
		sendSSEEvent(c, event.Type, string(eventJSON))

		return nil
	})

	return err
}

func (h *Handler) postProcessAssistantMessage(requestCtx context.Context, assistantMessage string) (string, error) {
	// Apply processors to the assistant message
	processors := h.deps.ProcessorsSvc.GetEnabledProcessors()
	processedMessage := assistantMessage

	for _, processor := range processors {
		h.logger.Info("Applying processor to assistant message", "processor", processor.Name())
		processedContent, err := processor.Process(requestCtx, []byte(processedMessage))
		if err != nil {
			h.logger.Error("Failed to process content with processor", "processor", processor.Name(), "error", err)
		} else {
			processedMessage = string(processedContent)
		}
	}

	return processedMessage, nil
}

// CreateChatStream handles streaming chat requests
func (h *Handler) CreateChatStream(c *gin.Context) {
	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("HTTP request binding failed", "status", http.StatusBadRequest, "error", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	slog.Debug("Processing streaming chat request", "message_length", len(req.Message.Content), "chat_id", req.ChatID)

	// Determine chat ID
	chatID := req.ChatID
	var chat *model.Chat
	var err error

	ctx := context.Background()

	if chatID == "" {
		chatID = uuid.New().String()
		chat = model.NewChat(chatID)
	} else {
		chat, err = h.deps.Redis.GetChat(ctx, chatID)
		if err != nil {
			slog.Warn("Failed to load existing chat, creating new one", "chat_id", chatID, "error", err)
			chat = model.NewChat(chatID)
		}
	}

	// Add user message to chat
	chat.AddMessage(req.Message)

	// Build prompt
	chatHistory := chat.GetMessageHistory()
	var prompt string

	var onboardingData *model.OnboardingData
	if req.Message.Context != nil && req.Message.Context.OnboardingData != nil {
		onboardingData = req.Message.Context.OnboardingData
	}
	prompt = h.deps.PromptBuilder.Build(onboardingData, req.Variables, chatHistory, req.Message.Content)

	fmt.Fprintf(os.Stderr, "Prompt built:\n======\n%s\n=====\n", prompt)

	// Count tokens using tiktoken
	enc, err := tokenizer.Get(TOKEN_MODEL)
	if err != nil {
		slog.Error("Failed to initialize tokenizer", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize tokenizer"})
		return
	}
	numTokens, err := enc.Count(prompt)
	if err != nil {
		slog.Error("Failed to count tokens", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count tokens"})
		return
	}
	slog.Info("Token count calculated", "count", numTokens)
	if numTokens > MAX_INPUT_TOKENS {
		errorMsg := fmt.Sprintf("Input exceeds maximum token limit of %d", MAX_INPUT_TOKENS)
		slog.Error("Token limit exceeded", "limit", MAX_INPUT_TOKENS, "count", numTokens)
		c.JSON(http.StatusBadRequest, gin.H{"error": errorMsg})
		return
	}

	slog.Info("Full prompt (with context)", "prompt", prompt)

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	requestCtx := c.Request.Context()

	sendSSEEvent(c, "start", fmt.Sprintf(`{"chat_id":"%s"}`, chatID))

	keywords := []string{}

	if req.Message.Context != nil && req.Message.Context.OnboardingData != nil {
		od := req.Message.Context.OnboardingData
		slog.Info("Extracting keywords from onboarding data for mock response selection", "onboarding_data", od)

		if od.BusinessName != "" {
			keyword := common.SafeString(od.BusinessName)
			keywords = append(keywords, keyword)
		}

		if od.BusinessTypeData.Label != "" {
			keyword := common.SafeString(od.BusinessTypeData.Label)
			keywords = append(keywords, keyword)
		}
	}

	slog.Info("Request keywords (used for mock/saved response filenames)", "keywords", keywords)

	isMockResponse := false
	var assistantMessage string

	if h.deps.Config.MockResponse {
		mockFile := ".config/mock_content.txt"
		if len(keywords) > 0 {
			keywordPart := strings.Join(keywords, "_")
			mockFileCandidate := fmt.Sprintf(".config/mocks/mock_content_%s.html", keywordPart)
			slog.Info("Looking for keyword-specific mock file", "file", mockFileCandidate)
			if _, err := os.Stat(mockFileCandidate); err == nil {
				mockFile = mockFileCandidate
			}
		}

		slog.Info("Using mock response", "file", mockFile)
		if _, err := os.Stat(mockFile); err == nil {
			data, err := os.ReadFile(mockFile)
			if err != nil {
				slog.Error("Failed to read mock response file", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read mock response file"})
				return
			}
			assistantMessage = string(data)
			isMockResponse = true
		}
	} else {
		fullContent := strings.Builder{}
		err = h.streamVertexResponse(c, requestCtx, prompt, chatID, req.ChatStage, &fullContent, sendSSEEvent)

		if err == nil {
			assistantMessage = fullContent.String()
		}
	}

	if err != nil {
		slog.Error("Streaming failed", "error", err)
		sendSSEEvent(c, "error", fmt.Sprintf(`{"error":"%s"}`, err.Error()))
		return
	}

	if !isMockResponse || h.deps.Config.PostProcessMockResponses {
		assistantMessage, err = h.postProcessAssistantMessage(requestCtx, assistantMessage)
		if err != nil {
			slog.Error("Post-processing assistant message failed", "error", err)
			sendSSEEvent(c, "error", fmt.Sprintf(`{"error":"%s"}`, err.Error()))
			return
		}
	}

	chat.AddMessageWithRoleAndContent(model.ChatMessageRoleAssistant, assistantMessage)

	// Save chat to Redis
	if err := h.deps.Redis.SaveChat(ctx, chat); err != nil {
		slog.Error("Failed to save chat", "error", err)
	}

	if !h.deps.Config.SaveResponses {
		fmt.Fprintf(os.Stderr, "\n\n%s\n\n", assistantMessage)
	}

	// Send done event
	response := model.ChatResponse{
		ChatID:    chatID,
		ChatStage: req.ChatStage,
		Message: model.ChatMessage{
			ID:        common.RandomID(),
			Role:      "assistant",
			Content:   assistantMessage,
			Timestamp: time.Now().Unix(),
		},
		Timestamp: time.Now().Unix(),
	}

	if h.deps.Config.SaveResponses {
		responseDir := ".var/saved_responses"
		canSave := false
		if _, err := os.Stat(responseDir); os.IsNotExist(err) {
			err := os.MkdirAll(responseDir, 0755)
			if err != nil {
				slog.Error("Failed to create responses directory", "dir", responseDir, "error", err)
				canSave = false
			}
		} else {
			canSave = true
		}

		if canSave {
			responsePrefix := ".var/saved_responses/response"
			if len(keywords) > 0 {
				keywordPart := strings.Join(keywords, "_")
				responsePrefix = fmt.Sprintf(".var/saved_responses/response_%s", keywordPart)
			}
			timestamp := time.Now().Unix() + (time.Now().UnixNano()%1e6)*1000
			responseFile := fmt.Sprintf("%s_%d.html", responsePrefix, timestamp)
			err := os.WriteFile(responseFile, []byte(assistantMessage), 0644)
			if err != nil {
				slog.Error("Failed to save response to file", "file", responseFile, "error", err)
			} else {
				slog.Info("Saved response to file", "file", responseFile)
			}
		}
	}

	doneJSON, _ := json.Marshal(map[string]interface{}{
		"type":     "done",
		"response": response,
	})
	h.logger.Info("Sending done event")
	sendSSEEvent(c, "done", string(doneJSON))
}

// sendSSEEvent sends a Server-Sent Event
func sendSSEEvent(c *gin.Context, eventType, data string) {
	c.Writer.Write([]byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)))
	c.Writer.Flush()
}

// GetChat retrieves a chat by ID
func (h *Handler) GetChat(c *gin.Context) {
	chatID := c.Param("id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id is required"})
		return
	}

	ctx := context.Background()
	chat, err := h.deps.Redis.GetChat(ctx, chatID)
	if err != nil {
		slog.Error("Failed to get chat", "chat_id", chatID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Chat not found"})
		return
	}

	c.JSON(http.StatusOK, chat)
}

// DeleteChat deletes a chat by ID
func (h *Handler) DeleteChat(c *gin.Context) {
	chatID := c.Param("id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id is required"})
		return
	}

	ctx := context.Background()
	if err := h.deps.Redis.DeleteChat(ctx, chatID); err != nil {
		slog.Error("Failed to delete chat", "chat_id", chatID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete chat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Chat deleted successfully"})
}

// RegisterRoutes registers chat-related routes
func RegisterRoutes(r *gin.RouterGroup, deps *sections.Dependencies, jwtManager *auth.JWTManager) {
	handler := NewHandler(deps)

	// Tenant-scoped chat routes
	tenantRoutes := r.Group("/api/v1/chat")
	tenantRoutes.Use(auth.JWTAuthMiddleware(jwtManager))
	{
		tenantRoutes.POST("/stream", handler.CreateChatStream)
		tenantRoutes.GET("/:id", handler.GetChat)
		tenantRoutes.DELETE("/:id", handler.DeleteChat)
	}
}
