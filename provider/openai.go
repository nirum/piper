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
	"time"
)

const defaultOpenAIBaseURL = "https://api.openai.com"

type OpenAI struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, baseURL string) *OpenAI {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	// Trim trailing slash for consistent URL construction.
	baseURL = strings.TrimRight(baseURL, "/")
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// SetTimeout configures the HTTP client timeout. Zero means no timeout.
func (o *OpenAI) SetTimeout(d time.Duration) { o.client.Timeout = d }

// OpenAI API types.
type openaiRequest struct {
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	MaxTokens int             `json:"max_completion_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

type openaiError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (o *OpenAI) Complete(ctx context.Context, req *Request) (*Response, error) {
	body := o.buildRequest(req, false)

	resp, err := o.do(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, o.parseError(resp)
	}

	var result openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var text string
	if len(result.Choices) > 0 {
		text = result.Choices[0].Message.Content
	}

	return &Response{
		Content:      text,
		InputTokens:  result.Usage.PromptTokens,
		OutputTokens: result.Usage.CompletionTokens,
		Model:        result.Model,
	}, nil
}

func (o *OpenAI) Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	body := o.buildRequest(req, true)

	resp, err := o.do(ctx, body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, o.parseError(resp)
	}

	ch := make(chan StreamEvent, 16)
	go o.readSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (o *OpenAI) buildRequest(req *Request, stream bool) openaiRequest {
	msgs := make([]openaiMessage, 0, len(req.Messages)+1)

	if req.System != "" {
		msgs = append(msgs, openaiMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, openaiMessage{Role: m.Role, Content: m.Content})
	}

	return openaiRequest{
		Model:     req.Model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
		Stream:    stream,
	}
}

func (o *OpenAI) do(ctx context.Context, body interface{}) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		o.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	return o.client.Do(httpReq)
}

func (o *OpenAI) readSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			select {
			case ch <- StreamEvent{Done: true}:
			case <-ctx.Done():
			}
			return
		}

		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
			select {
			case ch <- StreamEvent{Delta: event.Choices[0].Delta.Content}:
			case <-ctx.Done():
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case ch <- StreamEvent{Err: fmt.Errorf("read stream: %w", err)}:
		case <-ctx.Done():
		}
	}
}

func (o *OpenAI) parseError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(resp.Body)

	var apiErr openaiError
	if json.Unmarshal(bodyBytes, &apiErr) == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("openai API error (%d): %s: %s",
			resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
	}

	return fmt.Errorf("openai API error (%d): %s", resp.StatusCode, string(bodyBytes))
}
