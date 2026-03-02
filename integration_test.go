package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/niru/piper/cmd"
)

func TestIntegration_Anthropic(t *testing.T) {
	if os.Getenv("PIPER_INTEGRATION") != "1" {
		t.Skip("set PIPER_INTEGRATION=1 to run integration tests")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	// Create a pipe to simulate stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
	w.Close()

	var stdout, stderr bytes.Buffer
	ctx := cmd.WithInterrupted(r)
	code := cmd.Run(ctx, []string{"-v", "respond with exactly one word"}, r, &stdout, &stderr, "test")

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		t.Error("expected non-empty output")
	}
	t.Logf("response: %q", output)
	t.Logf("stderr: %s", stderr.String())
}

func TestIntegration_NoStream(t *testing.T) {
	if os.Getenv("PIPER_INTEGRATION") != "1" {
		t.Skip("set PIPER_INTEGRATION=1 to run integration tests")
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello"))
	w.Close()

	var stdout, stderr bytes.Buffer
	ctx := context.Background()
	code := cmd.Run(ctx, []string{"--no-stream", "respond with exactly one word"}, r, &stdout, &stderr, "test")

	if code != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		t.Error("expected non-empty output")
	}
	t.Logf("response: %q", output)
}
