package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBuildUserMessage(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    string
		expected string
	}{
		{
			name:     "context only",
			args:     []string{"summarize", "in 3 bullets"},
			stdin:    "",
			expected: "summarize in 3 bullets",
		},
		{
			name:     "stdin only",
			args:     nil,
			stdin:    "hello world",
			expected: "hello world",
		},
		{
			name:     "context and stdin",
			args:     []string{"summarize"},
			stdin:    "some content here",
			expected: "summarize\n\nsome content here",
		},
		{
			name:     "empty",
			args:     nil,
			stdin:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildUserMessage(tt.args, tt.stdin)
			if got != tt.expected {
				t.Errorf("buildUserMessage(%v, %q) = %q, want %q",
					tt.args, tt.stdin, got, tt.expected)
			}
		})
	}
}

func TestEnvKeyName(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"custom", "CUSTOM_API_KEY"},
	}

	for _, tt := range tests {
		got := envKeyName(tt.provider)
		if got != tt.expected {
			t.Errorf("envKeyName(%q) = %q, want %q", tt.provider, got, tt.expected)
		}
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run(nil, []string{"--version"}, nil, &stdout, &stderr, "1.2.3")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := stdout.String(); got != "piper 1.2.3\n" {
		t.Errorf("output = %q, want %q", got, "piper 1.2.3\n")
	}
}

func TestRun_BadFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{"--unknown-flag"}, nil, &stdout, &stderr, "test")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRun_MissingAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{"-p", "openai"}, nil, &stdout, &stderr, "test")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no API key") {
		t.Errorf("stderr %q should contain 'no API key'", stderr.String())
	}
}

func TestRun_StreamMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":", world!"}}]}`,
			`data: [DONE]`,
		}
		for _, ev := range events {
			fmt.Fprintln(w, ev)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	wr.WriteString("hello")
	wr.Close()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{
		"-p", "openai", "--base-url", server.URL, "summarize",
	}, rd, &stdout, &stderr, "test")
	rd.Close()

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Hello, world!") {
		t.Errorf("stdout %q should contain 'Hello, world!'", stdout.String())
	}
}

func TestRun_NoStreamMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"Done!"}}],"usage":{"prompt_tokens":5,"completion_tokens":2},"model":"gpt-4o"}`)
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	wr.WriteString("hello")
	wr.Close()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{
		"-p", "openai", "--base-url", server.URL, "--no-stream", "summarize",
	}, rd, &stdout, &stderr, "test")
	rd.Close()

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Done!") {
		t.Errorf("stdout %q should contain 'Done!'", stdout.String())
	}
}

func TestRun_StreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"type":"server_error","message":"Internal error"}}`)
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	wr.WriteString("hello")
	wr.Close()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{
		"-p", "openai", "--base-url", server.URL, "summarize",
	}, rd, &stdout, &stderr, "test")
	rd.Close()

	if code != 3 {
		t.Errorf("exit code = %d, want 3; stderr: %s", code, stderr.String())
	}
}

func TestRun_VerboseMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		events := []string{
			`data: {"choices":[{"delta":{"content":"Hi"}}]}`,
			`data: [DONE]`,
		}
		for _, ev := range events {
			fmt.Fprintln(w, ev)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")

	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	wr.WriteString("hello")
	wr.Close()

	var stdout, stderr strings.Builder
	code := Run(context.Background(), []string{
		"-p", "openai", "--base-url", server.URL, "-v", "summarize",
	}, rd, &stdout, &stderr, "test")
	rd.Close()

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "provider=openai") {
		t.Errorf("stderr %q should contain 'provider=openai'", stderr.String())
	}
	if !strings.Contains(stderr.String(), "latency=") {
		t.Errorf("stderr %q should contain 'latency='", stderr.String())
	}
}
