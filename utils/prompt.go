package utils

import (
	"awning-backend/model"
	"fmt"
	"os"
	"strings"
)

// PromptBuilder handles template-based prompt construction
type PromptBuilder struct {
	template string
}

// NewPromptBuilder creates a new prompt builder from a template file
func NewPromptBuilder(templatePath string) (*PromptBuilder, error) {
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	return &PromptBuilder{
		template: string(data),
	}, nil
}

// Build constructs a prompt by replacing variables in the template
func (pb *PromptBuilder) Build(onboardingData *model.OnboardingData, extraVariables map[string]string, chatHistory string, userMessage string) string {
	prompt := pb.template

	// Convert onboarding data to map
	onboardingMap := onboardingData.ToMap()

	// Merge onboarding data, extra variables into a single map
	variables := make(map[string]string)
	for k, v := range onboardingMap {
		variables[k] = v
	}
	for k, v := range extraVariables {
		variables[k] = v
	}

	// Replace all variables in the template
	for key, value := range variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		prompt = strings.ReplaceAll(prompt, placeholder, value)
	}

	// Append chat context and user message
	if chatHistory != "" {
		prompt += fmt.Sprintf("\n\n## Chat History\n\n%s", chatHistory)
	}

	prompt += fmt.Sprintf("\n\n## Current User Request\n\n%s", userMessage)

	return prompt
}

// BuildSimple constructs a simple prompt with just chat history and user message
func (pb *PromptBuilder) BuildSimple(chatHistory string, userMessage string) string {
	prompt := pb.template

	// Append chat context and user message
	if chatHistory != "" {
		prompt += fmt.Sprintf("\n\n## Chat History\n\n%s", chatHistory)
	}

	prompt += fmt.Sprintf("\n\n## Current User Request\n\n%s", userMessage)

	return prompt
}
