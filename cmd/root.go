package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/niru/piper/config"
	"github.com/niru/piper/provider"
	"github.com/spf13/pflag"
)

type interruptedKey struct{}

// WithInterrupted returns a context that can track whether the process was interrupted.
func WithInterrupted(stdin *os.File) context.Context {
	return context.WithValue(context.Background(), interruptedKey{}, new(bool))
}

// WasInterrupted returns true if the context was cancelled due to a signal.
func WasInterrupted(ctx context.Context) bool {
	if v, ok := ctx.Value(interruptedKey{}).(*bool); ok {
		return *v
	}
	return false
}

const usage = `Usage: piper [flags] [CONTEXT...]

Reads from stdin, sends to an LLM, streams the response to stdout.

Examples:
  cat README.md | piper "summarize in 3 bullets"
  git diff main | piper "review for bugs"
  echo "hello" | piper -m gpt-4o -p openai "respond"

  # Multi-turn conversation (--chat mode):
  echo '[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"}]' \
    | piper --chat "what did I say first?"

Flags:
  -m, --model     string   Model (default: claude-sonnet-4-20250514)
  -s, --system    string   System prompt (default: "You are a helpful assistant.")
  -t, --tokens    int      Max output tokens (default: 4096)
  -p, --provider  string   Provider: anthropic, openai (default: anthropic)
      --base-url  string   API base URL (for OpenAI-compat providers)
      --chat               Read conversation history as JSON from stdin
  -r, --raw                Disable markdown rendering, raw text (default)
      --no-stream          Disable streaming
  -v, --verbose            Metadata to stderr
      --version            Print version`

// Run is the main entrypoint. It returns an exit code.
func Run(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer, version string) int {
	// Parse flags.
	fs := pflag.NewFlagSet("piper", pflag.ContinueOnError)
	fs.SetOutput(stderr)

	modelFlag := fs.StringP("model", "m", "", "Model")
	systemFlag := fs.StringP("system", "s", "", "System prompt")
	tokensFlag := fs.IntP("tokens", "t", 0, "Max output tokens")
	providerFlag := fs.StringP("provider", "p", "", "Provider: anthropic, openai")
	baseURLFlag := fs.String("base-url", "", "API base URL")
	chatMode := fs.Bool("chat", false, "Read conversation history as JSON from stdin")
	_ = fs.BoolP("raw", "r", true, "Disable markdown rendering")
	noStream := fs.Bool("no-stream", false, "Disable streaming")
	verbose := fs.BoolP("verbose", "v", false, "Metadata to stderr")
	showVersion := fs.Bool("version", false, "Print version")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "piper %s\n", version)
		return 0
	}

	// Check stdin is not a TTY with no positional args.
	isTTY := isTerminal(stdin)
	positional := fs.Args()

	if isTTY && len(positional) == 0 {
		fmt.Fprintln(stderr, usage)
		return 1
	}

	// Load config.
	cfg := config.Load(stderr)

	// Apply flag overrides.
	providerName := cfg.Provider
	if *providerFlag != "" {
		providerName = *providerFlag
	}

	model := cfg.Model
	if *modelFlag != "" {
		model = *modelFlag
	}

	system := cfg.Defaults.System
	if *systemFlag != "" {
		system = *systemFlag
	}

	maxTokens := cfg.Defaults.MaxTokens
	if *tokensFlag != 0 {
		maxTokens = *tokensFlag
	}

	baseURL := cfg.BaseURL
	if *baseURLFlag != "" {
		baseURL = *baseURLFlag
	}

	// Resolve API key.
	apiKey := cfg.ResolveAPIKey(providerName)
	if apiKey == "" {
		fmt.Fprintf(stderr, "piper: no API key found for provider %q\n", providerName)
		fmt.Fprintf(stderr, "Set %s or add api_key to config\n", envKeyName(providerName))
		return 2
	}

	// Read stdin.
	var stdinData []byte
	if !isTTY {
		var err error
		stdinData, err = io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "piper: read stdin: %v\n", err)
			return 1
		}
	}

	// Build the message list.
	var messages []provider.Message
	if *chatMode {
		// Chat mode: stdin is a JSON array of prior messages.
		if len(stdinData) == 0 {
			fmt.Fprintln(stderr, "piper: --chat requires conversation JSON on stdin")
			return 1
		}
		if err := json.Unmarshal(stdinData, &messages); err != nil {
			fmt.Fprintf(stderr, "piper: --chat: parse conversation JSON: %v\n", err)
			return 1
		}
		// Append positional args as a new user turn if provided.
		if len(positional) > 0 {
			messages = append(messages, provider.Message{
				Role:    "user",
				Content: strings.Join(positional, " "),
			})
		}
		if len(messages) == 0 {
			fmt.Fprintln(stderr, "piper: --chat: conversation is empty")
			return 1
		}
	} else {
		userMessage := buildUserMessage(positional, string(stdinData))
		if strings.TrimSpace(userMessage) == "" {
			fmt.Fprintln(stderr, "piper: no input provided")
			fmt.Fprintln(stderr, usage)
			return 1
		}
		messages = []provider.Message{{Role: "user", Content: userMessage}}
	}

	// Create provider.
	p, err := provider.New(providerName, apiKey, baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return 2
	}

	req := &provider.Request{
		Model:     model,
		System:    system,
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	if *verbose {
		fmt.Fprintf(stderr, "provider=%s model=%s max_tokens=%d\n", providerName, model, maxTokens)
	}

	start := time.Now()

	if *noStream {
		return doComplete(ctx, p, req, stdout, stderr, *verbose, start)
	}
	return doStream(ctx, p, req, stdout, stderr, *verbose, start)
}

func doComplete(ctx context.Context, p provider.Provider, req *provider.Request, stdout, stderr io.Writer, verbose bool, start time.Time) int {
	resp, err := p.Complete(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return 4
		}
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return 3
	}

	fmt.Fprint(stdout, resp.Content)
	if !strings.HasSuffix(resp.Content, "\n") {
		fmt.Fprintln(stdout)
	}

	if verbose {
		fmt.Fprintf(stderr, "latency=%s input_tokens=%d output_tokens=%d\n",
			time.Since(start).Round(time.Millisecond), resp.InputTokens, resp.OutputTokens)
	}
	return 0
}

func doStream(ctx context.Context, p provider.Provider, req *provider.Request, stdout, stderr io.Writer, verbose bool, start time.Time) int {
	ch, err := p.Stream(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return 4
		}
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return 3
	}

	var lastContent string
	for ev := range ch {
		if ev.Err != nil {
			fmt.Fprintf(stderr, "\npiper: %v\n", ev.Err)
			return 3
		}
		if ev.Delta != "" {
			fmt.Fprint(stdout, ev.Delta)
			lastContent = ev.Delta
		}
		if ev.Done {
			if !strings.HasSuffix(lastContent, "\n") {
				fmt.Fprintln(stdout)
			}
			if verbose {
				fmt.Fprintf(stderr, "latency=%s input_tokens=%d output_tokens=%d\n",
					time.Since(start).Round(time.Millisecond), ev.InputTokens, ev.OutputTokens)
			}
			return 0
		}
	}

	// Channel closed without Done event — context cancelled.
	if ctx.Err() != nil {
		if v, ok := ctx.Value(interruptedKey{}).(*bool); ok {
			*v = true
		}
		fmt.Fprintln(stdout) // flush partial line
		return 4
	}

	// Stream ended without explicit done.
	fmt.Fprintln(stdout)
	return 0
}

func buildUserMessage(contextArgs []string, stdinContent string) string {
	parts := make([]string, 0, 2)
	if len(contextArgs) > 0 {
		parts = append(parts, strings.Join(contextArgs, " "))
	}
	if stdinContent != "" {
		parts = append(parts, stdinContent)
	}
	return strings.Join(parts, "\n\n")
}

func envKeyName(providerName string) string {
	switch providerName {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	default:
		return strings.ToUpper(providerName) + "_API_KEY"
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
