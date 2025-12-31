package handlers

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
	"awning-backend/services"
	"awning-backend/storage"
	"awning-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tiktoken-go/tokenizer"
)

const (
	MAX_INPUT_TOKENS  = 200000
	MAX_OUTPUT_TOKENS = 400000
	TOKEN_MODEL       = tokenizer.Cl100kBase
)

// StreamEvent represents a streaming event from the AI
type StreamEvent struct {
	Type    string
	Content string
}

// ChatHandler handles chat-related requests
type ChatHandler struct {
	logger        *slog.Logger
	cfg           *common.Config
	storage       *storage.RedisClient
	promptBuilder *utils.PromptBuilder
	vertexClient  VertexClient
	processorsSvc *services.Processors
}

// VertexClient interface for AI content generation
type VertexClient interface {
	GenerateContentStream(ctx context.Context, prompt string, callback func(StreamEvent) error) error
}

// NewChatHandler creates a new chat handler
func NewChatHandler(cfg *common.Config, storage *storage.RedisClient, promptBuilder *utils.PromptBuilder, vertexClient VertexClient, processorsSvc *services.Processors) *ChatHandler {
	logger := slog.With("handler", "ChatHandler")

	return &ChatHandler{
		logger:        logger,
		cfg:           cfg,
		storage:       storage,
		promptBuilder: promptBuilder,
		vertexClient:  vertexClient,
		processorsSvc: processorsSvc,
	}
}

type SendSSEEvent func(c *gin.Context, eventType, data string)

func (h *ChatHandler) streamVertexResponse(c *gin.Context, requestCtx context.Context, prompt string, chatID string, chatStage model.ChatStage, fullContent *strings.Builder, sendSSEEvent SendSSEEvent) error {

	// Send start message
	sendSSEEvent(c, "start", `{"message":"Starting response generation..."}`)

	// Send periodic placeholder event
	if !h.cfg.SendThinking {
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
	err := h.vertexClient.GenerateContentStream(requestCtx, prompt, func(event StreamEvent) error {
		if !h.cfg.SendThinking && event.Type == "thinking" {
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

		// // Collect content for storage
		// if event.Type == "content" {
		// 	fullContent.WriteString(event.Content)
		// }

		return nil
	})

	return err
}

func (h *ChatHandler) postProcessAssistantMessage(requestCtx context.Context, assistantMessage string) (string, error) {
	// Apply processors to the assistant message
	processors := h.processorsSvc.GetEnabledProcessors()
	processedMessage := assistantMessage

	for _, processor := range processors {
		h.logger.Info("Applying processor to assistant message", "processor", processor.Name())
		processedContent, err := processor.Process(requestCtx, []byte(processedMessage))
		if err != nil {
			h.logger.Error("Failed to process content with processor", "processor", processor.Name(), "error", err)
			// Proceed with original content
		} else {
			processedMessage = string(processedContent)
		}
	}

	return processedMessage, nil
}

// CreateChatStream handles streaming chat requests
func (h *ChatHandler) CreateChatStream(c *gin.Context) {
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
		// Create new chat
		chatID = uuid.New().String()
		chat = model.NewChat(chatID)
	} else {
		// Load existing chat
		chat, err = h.storage.GetChat(ctx, chatID)
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
	// if len(req.Variables) > 0 {
	// 	var onboardingData *model.OnboardingData
	// 	if req.Message.Context != nil && req.Message.Context.OnboardingData != nil {
	// 		onboardingData = req.Message.Context.OnboardingData
	// 	}
	// 	prompt = h.promptBuilder.Build(onboardingData, req.Variables, chatHistory, req.Message.Content)
	// } else {
	// 	prompt = h.promptBuilder.BuildSimple(chatHistory, req.Message.Content)
	// }

	var onboardingData *model.OnboardingData
	if req.Message.Context != nil && req.Message.Context.OnboardingData != nil {
		onboardingData = req.Message.Context.OnboardingData
	}
	prompt = h.promptBuilder.Build(onboardingData, req.Variables, chatHistory, req.Message.Content)

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
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	requestCtx := c.Request.Context()

	// Send initial event with chat ID
	sendSSEEvent(c, "start", fmt.Sprintf(`{"chat_id":"%s"}`, chatID))

	// Collect full response
	// var fullContent strings.Builder

	keywords := []string{}

	if od := req.Message.Context.OnboardingData; od != nil {
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

	if h.cfg.MockResponse {
		// We don't actually care about the thinking messages right now
		// when mocking response, just send the done event with mock content

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
			mockResponse := string(data)
			// h.logger.Info("Using mock response from file", "response", mockResponse)
			// fullContent.WriteString(mockResponse)
			assistantMessage = mockResponse
			isMockResponse = true
		}
	} else {
		fullContent := strings.Builder{}

		// Stream response from Vertex AI
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

	if !isMockResponse || h.cfg.PostProcessMockResponses {
		assistantMessage, err = h.postProcessAssistantMessage(requestCtx, assistantMessage)
		if err != nil {
			slog.Error("Post-processing assistant message failed", "error", err)
			sendSSEEvent(c, "error", fmt.Sprintf(`{"error":"%s"}`, err.Error()))
			return
		}
	}

	// _ = isMockResponse

	// Add assistant response to chat

	// assistantMessage := fullContent.String()

	// h.logger.Info("Full assistant message", "message", assistantMessage)

	// if h.cfg.IsProcessorEnabled("image") {
	// 	imageProcessor, ok := h.processorsSvc.GetProcessor("image")
	// 	if !ok {
	// 		slog.Warn("Image processor not found")
	// 		// Proceed without image processing
	// 	} else {
	// 		// Process images in the assistant message
	// 		processedContent, err := imageProcessor.Process(requestCtx, []byte(assistantMessage))
	// 		if err != nil {
	// 			slog.Error("Failed to process images in content", "error", err)
	// 			// Proceed with original content
	// 		} else {
	// 			assistantMessage = string(processedContent)
	// 		}
	// 	}
	// }

	// processors := h.processorsSvc.GetEnabledProcessors()
	// for _, processor := range processors {
	// 	slog.Info("Applying processor to assistant message", "processor", processor.Name())
	// 	processedContent, err := processor.Process(requestCtx, []byte(assistantMessage))
	// 	if err != nil {
	// 		slog.Error("Failed to process content with processor", "processor", processor.Name(), "error", err)
	// 		// Proceed with original content
	// 	} else {
	// 		assistantMessage = string(processedContent)
	// 	}
	// }

	chat.AddMessageWithRoleAndContent(model.ChatMessageRoleAssistant, assistantMessage)

	// Save chat to Redis
	if err := h.storage.SaveChat(ctx, chat); err != nil {
		slog.Error("Failed to save chat", "error", err)
		// Don't fail the request, just log the error
	}

	if !h.cfg.SaveResponses {
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

	if h.cfg.SaveResponses { //&& !isMockResponse {
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
func (h *ChatHandler) GetChat(c *gin.Context) {
	chatID := c.Param("id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id is required"})
		return
	}

	ctx := context.Background()
	chat, err := h.storage.GetChat(ctx, chatID)
	if err != nil {
		slog.Error("Failed to get chat", "chat_id", chatID, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Chat not found"})
		return
	}

	c.JSON(http.StatusOK, chat)
}

// DeleteChat deletes a chat by ID
func (h *ChatHandler) DeleteChat(c *gin.Context) {
	chatID := c.Param("id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chat_id is required"})
		return
	}

	ctx := context.Background()
	if err := h.storage.DeleteChat(ctx, chatID); err != nil {
		slog.Error("Failed to delete chat", "chat_id", chatID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete chat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Chat deleted successfully"})
}
