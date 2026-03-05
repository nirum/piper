package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropic_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want test-key", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %s", got, anthropicVersion)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"content": [{"type": "text", "text": "Hello, world!"}],
			"usage": {"input_tokens": 10, "output_tokens": 5},
			"model": "claude-sonnet-4-20250514"
		}`)
	}))
	defer server.Close()

	a2 := newTestAnthropic("test-key", server.URL)

	resp, err := a2.Complete(context.Background(), &Request{
		Model:     "claude-sonnet-4-20250514",
		System:    "test",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "Hello, world!" {
		t.Errorf("Content = %q, want Hello, world!", resp.Content)
	}
	if resp.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", resp.InputTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.OutputTokens)
	}
}

func TestAnthropic_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"usage":{"input_tokens":15}}}`,
			`data: {"type":"content_block_start","index":0}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world!"}}`,
			`data: {"type":"message_delta","usage":{"output_tokens":8}}`,
			`data: {"type":"message_stop"}`,
		}

		for _, ev := range events {
			fmt.Fprintln(w, ev)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	a := newTestAnthropic("test-key", server.URL)
	ch, err := a.Stream(context.Background(), &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var content string
	var done bool
	var inputTokens, outputTokens int

	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		content += ev.Delta
		if ev.Done {
			done = true
			inputTokens = ev.InputTokens
			outputTokens = ev.OutputTokens
		}
	}

	if !done {
		t.Error("stream did not receive done event")
	}
	if content != "Hello, world!" {
		t.Errorf("content = %q, want Hello, world!", content)
	}
	if inputTokens != 15 {
		t.Errorf("inputTokens = %d, want 15", inputTokens)
	}
	if outputTokens != 8 {
		t.Errorf("outputTokens = %d, want 8", outputTokens)
	}
}

func TestAnthropic_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"type":"rate_limit_error","message":"Rate limited"}}`)
	}))
	defer server.Close()

	a := newTestAnthropic("test-key", server.URL)
	_, err := a.Complete(context.Background(), &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

// newTestAnthropic creates an Anthropic client that hits a test server.
func newTestAnthropic(apiKey, baseURL string) *Anthropic {
	return &Anthropic{
		apiKey:  apiKey,
		client:  &http.Client{},
		baseURL: baseURL,
	}
}

func TestAnthropic_StreamErrorEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
		fmt.Fprintln(w)
		flusher.Flush()
	}))
	defer server.Close()

	a := newTestAnthropic("test-key", server.URL)
	ch, err := a.Stream(context.Background(), &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var gotErr bool
	for ev := range ch {
		if ev.Err != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected stream error event, got none")
	}
}

func TestAnthropic_StreamContextCancel(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}`)
		fmt.Fprintln(w)
		fmt.Fprintln(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`)
		fmt.Fprintln(w)
		flusher.Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := newTestAnthropic("test-key", server.URL)
	ch, err := a.Stream(ctx, &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	<-ready
	cancel()
	// Drain channel — must close without deadlock.
	for range ch {
	}
}

func TestAnthropic_StreamNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"Invalid API key"}}`)
	}))
	defer server.Close()

	a := newTestAnthropic("bad-key", server.URL)
	_, err := a.Stream(context.Background(), &Request{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error from Stream(), got nil")
	}
}
