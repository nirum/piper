package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion = "2023-06-01"
)

type Anthropic struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey:  apiKey,
		baseURL: anthropicBaseURL,
		client:  &http.Client{},
	}
}

// SetDebug activates request/response debug logging to w. Pass nil to disable.
func (a *Anthropic) SetDebug(w io.Writer) { wrapDebug(a.client, w) }

// Anthropic API request/response types.
type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

type anthropicError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (a *Anthropic) Complete(ctx context.Context, req *Request) (*Response, error) {
	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  req.Messages,
	}

	resp, err := a.do(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, a.parseError(resp)
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var text string
	for _, c := range result.Content {
		text += c.Text
	}

	return &Response{
		Content:      text,
		InputTokens:  result.Usage.InputTokens,
		OutputTokens: result.Usage.OutputTokens,
		Model:        result.Model,
	}, nil
}

func (a *Anthropic) Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  req.Messages,
		Stream:    true,
	}

	resp, err := a.do(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, a.parseError(resp)
	}

	ch := make(chan StreamEvent, 16)
	go a.readSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (a *Anthropic) do(ctx context.Context, body interface{}) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		a.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	return a.client.Do(httpReq)
}

func (a *Anthropic) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	var inputTokens, outputTokens int
	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "message_start":
			inputTokens = event.Message.Usage.InputTokens
		case "content_block_delta":
			if event.Delta.Text != "" {
				select {
				case ch <- StreamEvent{Delta: event.Delta.Text}:
				case <-ctx.Done():
					return
				}
			}
		case "message_delta":
			outputTokens = event.Usage.OutputTokens
		case "message_stop":
			select {
			case ch <- StreamEvent{Done: true, InputTokens: inputTokens, OutputTokens: outputTokens}:
			case <-ctx.Done():
			}
			return
		case "error":
			select {
			case ch <- StreamEvent{Err: fmt.Errorf("stream error: %s", data)}:
			case <-ctx.Done():
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- StreamEvent{Err: fmt.Errorf("read stream: %w", err)}:
		case <-ctx.Done():
		}
	}
}

func (a *Anthropic) parseError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(resp.Body)

	var apiErr anthropicError
	if json.Unmarshal(bodyBytes, &apiErr) == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("anthropic API error (%d): %s: %s",
			resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
	}

	return fmt.Errorf("anthropic API error (%d): %s", resp.StatusCode, string(bodyBytes))
}
