package ai

import "github.com/arnavsurve/promise/pkg/models"

// Messages type for RequestPayload
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestPayload for Groq API
type RequestPayload struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	ResponseFormat map[string]string `json:"response_format"`
}

// MessageResponse represents a response choice from the AI
type MessageResponse struct {
	Role    string `json:"role"`
	Content string `json:"content"` // This contains the actual JSON subtasks
}

// Choice represents a single completion choice
type Choice struct {
	Message MessageResponse `json:"message"`
}

// GroqResponse represents the full AI API response
type GroqResponse struct {
	Choices []Choice `json:"choices"`
}

// TaskResponse represents the AI-generated task breakdown
type TaskResponse struct {
	Subtasks []models.Task `json:"subtasks"`
}

type CommandResponse struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Context string   `json:"context"`
}

type CodeResponse struct {
	Code     string `json:"code"`
	Filename string `json:"filename"`
	Context  string `json:"context"`
}
