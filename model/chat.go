package model

import (
	"awning-backend/common"
	"encoding/json"
	"fmt"
	"time"
)

type ChatMessageContext struct {
	OnboardingData *OnboardingData `json:"onboarding_data,omitempty"`
}

// ChatMessage represents a single message in a chat
type ChatMessage struct {
	ID        string              `json:"id"`
	Role      ChatMessageRole     `json:"role"` // "user" or "assistant"
	Content   string              `json:"content"`
	Timestamp int64               `json:"timestamp"`
	Context   *ChatMessageContext `json:"context,omitempty"`
}

func NewChatMessage(role ChatMessageRole, content string) *ChatMessage {
	return &ChatMessage{
		ID:        common.RandomID(),
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}
}

type ChatMessageRole string

const (
	ChatMessageRoleUser      ChatMessageRole = "user"
	ChatMessageRoleAssistant ChatMessageRole = "assistant"
)

type ChatStage string

const (
	ChatStageInitialCreation ChatStage = "initial_creation"
	ChatStageUserInput       ChatStage = "update"
)

// Chat represents a conversation with multiple messages
type Chat struct {
	ID        string          `json:"id"`
	Messages  []ChatMessage   `json:"messages"`
	ChatStage ChatStage       `json:"chat_stage"`
	CreatedAt int64           `json:"created_at"`
	UpdatedAt int64           `json:"updated_at"`
	LastRole  ChatMessageRole `json:"last_role"` // "user" or "assistant"
}

// ChatRequest represents the incoming chat request
type ChatRequest struct {
	ChatID    string    `json:"chat_id,omitempty"`
	ChatStage ChatStage `json:"chat_stage,omitempty"`
	// Message       string            `json:"message" binding:"required"`
	Message       *ChatMessage      `json:"message,omitempty"`        // Optional, for updating chat state
	Variables     map[string]string `json:"variables,omitempty"`      // For template variables
	TemplateInput string            `json:"template_input,omitempty"` // For template-based responses
}

// ChatResponse represents the response to a chat request
type ChatResponse struct {
	ChatID         string      `json:"chat_id"`
	ChatStage      ChatStage   `json:"chat_stage"`
	Message        ChatMessage `json:"message"`
	Timestamp      int64       `json:"timestamp"`
	TemplateOutput string      `json:"template_output,omitempty"` // For template-based responses
}

// NewChat creates a new chat instance
func NewChat(id string) *Chat {
	now := time.Now().Unix()
	return &Chat{
		ID:        id,
		Messages:  []ChatMessage{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (c *Chat) AddMessageWithRoleAndContent(role ChatMessageRole, content string) {
	message := NewChatMessage(role, content)
	c.AddMessage(message)
}

// AddMessage adds a message to the chat
func (c *Chat) AddMessage(message *ChatMessage) {
	c.Messages = append(c.Messages, *message)
	c.UpdatedAt = time.Now().Unix()
	c.LastRole = ChatMessageRole(message.Role)
	c.ChatStage = ChatStageUserInput
}

// ToJSON converts the chat to JSON
func (c *Chat) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// FromJSON creates a chat from JSON
func FromJSON(data []byte) (*Chat, error) {
	var chat Chat
	err := json.Unmarshal(data, &chat)
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetMessageHistory returns formatted message history for prompts
func (c *Chat) GetMessageHistory() string {
	var result string
	for _, msg := range c.Messages {
		result += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
	}
	return result
}
