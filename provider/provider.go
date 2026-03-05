package provider

import (
	"context"
	"fmt"
)

// Request represents a completion request to any provider.
type Request struct {
	Model     string
	System    string
	Messages  []Message
	MaxTokens int
	Stream    bool
}

// Message is a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response is a non-streaming completion response.
type Response struct {
	Content      string
	InputTokens  int
	OutputTokens int
	Model        string
}

// StreamEvent represents one chunk of a streaming response.
type StreamEvent struct {
	// Delta is the text content of this chunk. Empty for non-content events.
	Delta string
	// Err is set if an error occurred during streaming.
	Err error
	// Done indicates this is the final event.
	Done bool
	// Usage is populated on the final event.
	InputTokens  int
	OutputTokens int
}

// Provider is the interface implemented by all LLM backends.
type Provider interface {
	// Complete sends a non-streaming completion request.
	Complete(ctx context.Context, req *Request) (*Response, error)
	// Stream sends a streaming completion request.
	Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
}

// Retryable is an optional interface providers may implement to configure retry behavior.
type Retryable interface {
	SetRetries(n int)
}

// New creates a provider instance based on name.
func New(name, apiKey, baseURL string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropic(apiKey), nil
	case "openai":
		return NewOpenAI(apiKey, baseURL), nil
	default:
		return nil, fmt.Errorf("unknown provider: %q", name)
	}
}
