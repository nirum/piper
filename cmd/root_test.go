package cmd

import (
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
