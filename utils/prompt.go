package utils

import (
	"awning-backend/model"
	"fmt"
	"maps"
	"os"
	"strings"
)

// PromptBuilder handles template-based prompt construction
type PromptBuilder struct {
	baseTemplate    string
	requestTemplate string
}

// NewPromptBuilder creates a new prompt builder from a template file
func NewPromptBuilder(baseTemplatePath string, requestTemplatePath string) (*PromptBuilder, error) {
	baseData, err := os.ReadFile(baseTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base template file: %w", err)
	}

	requestData, err := os.ReadFile(requestTemplatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read request template file: %w", err)
	}

	return &PromptBuilder{
		baseTemplate:    string(baseData),
		requestTemplate: string(requestData),
	}, nil
}

func (pb *PromptBuilder) replaceValues(template string, onboardingData *model.OnboardingData, extraVariables map[string]string) string {
	// Convert onboarding data to map
	onboardingMap := onboardingData.ToMap()

	// Merge onboarding data, extra variables into a single map
	variables := make(map[string]string)

	maps.Copy(variables, onboardingMap)
	maps.Copy(variables, extraVariables)

	// Replace all variables in the template
	for key, value := range variables {
		placeholder := fmt.Sprintf("{{%s}}", key)
		template = strings.ReplaceAll(template, placeholder, value)
	}

	return template
}

// Build constructs a prompt by replacing variables in the template
func (pb *PromptBuilder) Build(onboardingData *model.OnboardingData, extraVariables map[string]string, chatHistory string, userRequestMessage string) string {
	prompt := ""

	// Append chat context and user message
	if chatHistory != "" {
		prompt += fmt.Sprintf("\n\n## Chat History\n\n%s", chatHistory)
	}

	// fmt.Fprintf(os.Stderr, "Base Template before replacement:\n%s\n", pb.baseTemplate)

	prompt += fmt.Sprintf("\n\n## Base Template\n\n%s", pb.baseTemplate)

	// // Replace variables in the prompt
	// prompt = pb.replaceValues(prompt, onboardingData, extraVariables)

	requestPrompt := pb.replaceValues(userRequestMessage, onboardingData, extraVariables)

	prompt += fmt.Sprintf("\n\n## Current User Request\n\n%s", requestPrompt)

	// fmt.Fprintf(os.Stderr, "Final Prompt before return:\n%s\n", prompt)

	return prompt
}

// // BuildSimple constructs a simple prompt with just chat history and user message
// func (pb *PromptBuilder) BuildSimple(chatHistory string, userMessage string) string {
// 	prompt := pb.requestTemplate

// 	// Append chat context and user message
// 	if chatHistory != "" {
// 		prompt += fmt.Sprintf("\n\n## Chat History\n\n%s", chatHistory)
// 	}

// 	prompt += fmt.Sprintf("\n\n## Current User Request\n\n%s", userMessage)

// 	return prompt
// }
