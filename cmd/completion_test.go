package cmd

import (
	"strings"
	"testing"
)

func TestPrintCompletion_Zsh(t *testing.T) {
	var stdout, stderr strings.Builder
	code := printCompletion(&stdout, &stderr, "zsh")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "#compdef piper") {
		t.Errorf("zsh completion missing '#compdef piper' header")
	}
	if !strings.Contains(out, "_piper") {
		t.Errorf("zsh completion missing '_piper' function")
	}
	// Should include all flags.
	for _, flag := range []string{"--model", "--system", "--tokens", "--provider", "--base-url", "--verbose", "--no-stream", "--version", "--completion"} {
		if !strings.Contains(out, flag) {
			t.Errorf("zsh completion missing flag %q", flag)
		}
	}
	// Should include known models.
	for _, model := range []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "gpt-4o"} {
		if !strings.Contains(out, model) {
			t.Errorf("zsh completion missing model %q", model)
		}
	}
}

func TestPrintCompletion_UnknownShell(t *testing.T) {
	var stdout, stderr strings.Builder
	code := printCompletion(&stdout, &stderr, "powershell")
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for unknown shell", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout output for unknown shell, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "powershell") {
		t.Errorf("stderr = %q, expected mention of the unknown shell name", stderr.String())
	}
}

func TestRunCompletion_ViaFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run(nil, []string{"--completion", "zsh"}, nil, &stdout, &stderr, "test")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "#compdef piper") {
		t.Errorf("expected zsh completion script in stdout")
	}
}
