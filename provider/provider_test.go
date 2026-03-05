package provider

import (
	"strings"
	"testing"
)

func TestNew_Anthropic(t *testing.T) {
	p, err := New("anthropic", "test-key", "")
	if err != nil {
		t.Fatalf("New(anthropic) error: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestNew_OpenAI(t *testing.T) {
	p, err := New("openai", "test-key", "")
	if err != nil {
		t.Fatalf("New(openai) error: %v", err)
	}
	if p == nil {
		t.Error("expected non-nil provider")
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	p, err := New("gemini", "test-key", "")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if p != nil {
		t.Error("expected nil provider for unknown name")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error %q should contain 'unknown provider'", err.Error())
	}
}
