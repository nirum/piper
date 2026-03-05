package cmd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/niru/piper/provider"
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

// fakeProvider is a test double that returns a scripted sequence of responses.
type fakeProvider struct {
	responses []string
	idx       int
}

func (f *fakeProvider) Complete(_ context.Context, _ *provider.Request) (*provider.Response, error) {
	resp := f.nextResponse()
	return &provider.Response{Content: resp}, nil
}

func (f *fakeProvider) Stream(_ context.Context, _ *provider.Request) (<-chan provider.StreamEvent, error) {
	text := f.nextResponse()
	ch := make(chan provider.StreamEvent, 2)
	ch <- provider.StreamEvent{Delta: text}
	ch <- provider.StreamEvent{Done: true}
	close(ch)
	return ch, nil
}

func (f *fakeProvider) nextResponse() string {
	if f.idx >= len(f.responses) {
		return "default response"
	}
	r := f.responses[f.idx]
	f.idx++
	return r
}

func TestRunInteractive_RequiresTTY(t *testing.T) {
	// A pipe is not a TTY, so --interactive should fail with exit code 1.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	var stdout, stderr strings.Builder
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	code := Run(context.Background(), []string{"--interactive"}, r, &stdout, &stderr, "test")
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (non-TTY should be rejected)", code)
	}
	if !strings.Contains(stderr.String(), "terminal") {
		t.Errorf("stderr = %q, expected mention of 'terminal'", stderr.String())
	}
}

func TestSendAndCollect(t *testing.T) {
	fp := &fakeProvider{responses: []string{"hello world"}}
	messages := []provider.Message{{Role: "user", Content: "say hello"}}

	var stdout, stderr strings.Builder
	reply, code := sendAndCollect(context.Background(), fp, &stdout, &stderr, "model", "system", 100, messages, false)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if reply != "hello world" {
		t.Errorf("reply = %q, want 'hello world'", reply)
	}
	if !strings.Contains(stdout.String(), "hello world") {
		t.Errorf("stdout = %q, expected 'hello world'", stdout.String())
	}
}

func TestRunInteractive_MultiTurn(t *testing.T) {
	fp := &fakeProvider{responses: []string{"response1", "response2"}}

	// Simulate two user turns via a pipe.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Write two lines then close (EOF).
	go func() {
		w.WriteString("first question\nsecond question\n")
		w.Close()
	}()

	var stdout, stderr strings.Builder
	code := runInteractive(context.Background(), fp, r, &stdout, &stderr, "model", "system", 100, nil, false)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	r.Close()

	out := stdout.String()
	if !strings.Contains(out, "response1") {
		t.Errorf("expected 'response1' in output, got: %q", out)
	}
	if !strings.Contains(out, "response2") {
		t.Errorf("expected 'response2' in output, got: %q", out)
	}
}

func TestRunInteractive_InitialArgs(t *testing.T) {
	fp := &fakeProvider{responses: []string{"initial reply", "follow-up reply"}}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		w.WriteString("follow-up question\n")
		w.Close()
	}()

	var stdout, stderr strings.Builder
	code := runInteractive(context.Background(), fp, r, &stdout, &stderr, "model", "system", 100, []string{"initial", "question"}, false)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	r.Close()

	out := stdout.String()
	if !strings.Contains(out, "initial reply") {
		t.Errorf("expected 'initial reply' in output, got: %q", out)
	}
	if !strings.Contains(out, "follow-up reply") {
		t.Errorf("expected 'follow-up reply' in output, got: %q", out)
	}
}
