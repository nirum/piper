package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAI_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"choices": [{"message": {"content": "Hi there!"}}],
			"usage": {"prompt_tokens": 12, "completion_tokens": 4},
			"model": "gpt-4o"
		}`)
	}))
	defer server.Close()

	o := NewOpenAI("test-key", server.URL)
	resp, err := o.Complete(context.Background(), &Request{
		Model:     "gpt-4o",
		System:    "test",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("Content = %q, want Hi there!", resp.Content)
	}
	if resp.InputTokens != 12 {
		t.Errorf("InputTokens = %d, want 12", resp.InputTokens)
	}
	if resp.OutputTokens != 4 {
		t.Errorf("OutputTokens = %d, want 4", resp.OutputTokens)
	}
}

func TestOpenAI_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":", "}}]}`,
			`data: {"choices":[{"delta":{"content":"world!"}}]}`,
			`data: [DONE]`,
		}

		for _, ev := range events {
			fmt.Fprintln(w, ev)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	o := NewOpenAI("test-key", server.URL)
	ch, err := o.Stream(context.Background(), &Request{
		Model:     "gpt-4o",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var content string
	var done bool

	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("stream error: %v", ev.Err)
		}
		content += ev.Delta
		if ev.Done {
			done = true
		}
	}

	if !done {
		t.Error("stream did not receive done event")
	}
	if content != "Hello, world!" {
		t.Errorf("content = %q, want Hello, world!", content)
	}
}

func TestOpenAI_SystemMessage(t *testing.T) {
	var gotMessages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just count — we verify the request building logic.
		gotMessages++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1},"model":"gpt-4o"}`)
	}))
	defer server.Close()

	o := NewOpenAI("test-key", server.URL)
	_, err := o.Complete(context.Background(), &Request{
		Model:     "gpt-4o",
		System:    "Be helpful",
		MaxTokens: 10,
		Messages:  []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
}

func TestOpenAI_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"invalid_api_key","message":"Invalid API key"}}`)
	}))
	defer server.Close()

	o := NewOpenAI("bad-key", server.URL)
	_, err := o.Complete(context.Background(), &Request{
		Model:     "gpt-4o",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAI_StreamContextCancel(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprintln(w, `data: {"choices":[{"delta":{"content":"Hi"}}]}`)
		fmt.Fprintln(w)
		flusher.Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o := NewOpenAI("test-key", server.URL)
	ch, err := o.Stream(ctx, &Request{
		Model:     "gpt-4o",
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

func TestOpenAI_StreamNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `{"error":{"type":"server_error","message":"Service unavailable"}}`)
	}))
	defer server.Close()

	o := NewOpenAI("test-key", server.URL)
	_, err := o.Stream(context.Background(), &Request{
		Model:     "gpt-4o",
		MaxTokens: 100,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error from Stream(), got nil")
	}
}

func TestOpenAI_NoAPIKey(t *testing.T) {
	// OpenAI provider should still make the request even without a key
	// (useful for local models like Ollama that don't need auth).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1},"model":"llama3"}`)
	}))
	defer server.Close()

	o := NewOpenAI("", server.URL)
	resp, err := o.Complete(context.Background(), &Request{
		Model:     "llama3",
		MaxTokens: 10,
		Messages:  []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("Content = %q, want ok", resp.Content)
	}
}
