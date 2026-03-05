package cmd

import (
	"bufio"
	"context"
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
  piper -i "you are a python expert"

Flags:
  -m, --model     string   Model (default: claude-sonnet-4-20250514)
  -s, --system    string   System prompt (default: "You are a helpful assistant.")
  -t, --tokens    int      Max output tokens (default: 4096)
  -p, --provider  string   Provider: anthropic, openai (default: anthropic)
      --base-url  string   API base URL (for OpenAI-compat providers)
  -i, --interactive        Multi-turn conversation mode (requires TTY)
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
	interactiveFlag := fs.BoolP("interactive", "i", false, "Multi-turn conversation mode")
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

	// Interactive mode requires a TTY.
	if *interactiveFlag && !isTTY {
		fmt.Fprintln(stderr, "piper: --interactive requires stdin to be a terminal")
		return 1
	}

	if !*interactiveFlag && isTTY && len(positional) == 0 {
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

	// Create provider.
	p, err := provider.New(providerName, apiKey, baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return 2
	}

	if *verbose {
		fmt.Fprintf(stderr, "provider=%s model=%s max_tokens=%d\n", providerName, model, maxTokens)
	}

	if *interactiveFlag {
		return runInteractive(ctx, p, stdin, stdout, stderr, model, system, maxTokens, positional, *verbose)
	}

	// Read stdin.
	var stdinContent string
	if !isTTY {
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "piper: read stdin: %v\n", err)
			return 1
		}
		stdinContent = string(data)
	}

	// Build user message: context args + stdin.
	userMessage := buildUserMessage(positional, stdinContent)
	if strings.TrimSpace(userMessage) == "" {
		fmt.Fprintln(stderr, "piper: no input provided")
		fmt.Fprintln(stderr, usage)
		return 1
	}

	req := &provider.Request{
		Model:     model,
		System:    system,
		MaxTokens: maxTokens,
		Messages: []provider.Message{
			{Role: "user", Content: userMessage},
		},
	}

	start := time.Now()

	if *noStream {
		return doComplete(ctx, p, req, stdout, stderr, *verbose, start)
	}
	return doStream(ctx, p, req, stdout, stderr, *verbose, start)
}

// runInteractive runs a multi-turn conversation REPL on the terminal.
func runInteractive(ctx context.Context, p provider.Provider, stdin *os.File, stdout, stderr io.Writer, model, system string, maxTokens int, initialArgs []string, verbose bool) int {
	var messages []provider.Message

	// If positional args were given, use them as the first user message.
	if len(initialArgs) > 0 {
		firstMsg := strings.Join(initialArgs, " ")
		messages = append(messages, provider.Message{Role: "user", Content: firstMsg})
		reply, code := sendAndCollect(ctx, p, stdout, stderr, model, system, maxTokens, messages, verbose)
		if code != 0 {
			return code
		}
		messages = append(messages, provider.Message{Role: "assistant", Content: reply})
	}

	scanner := bufio.NewScanner(stdin)
	for {
		fmt.Fprint(stderr, "\n>>> ")
		if !scanner.Scan() {
			// EOF or error — exit cleanly.
			fmt.Fprintln(stdout)
			if err := scanner.Err(); err != nil {
				fmt.Fprintf(stderr, "piper: read error: %v\n", err)
				return 1
			}
			return 0
		}
		if ctx.Err() != nil {
			return 4
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		messages = append(messages, provider.Message{Role: "user", Content: line})
		reply, code := sendAndCollect(ctx, p, stdout, stderr, model, system, maxTokens, messages, verbose)
		if code != 0 {
			return code
		}
		messages = append(messages, provider.Message{Role: "assistant", Content: reply})
	}
}

// sendAndCollect streams a response and returns the full assistant text.
func sendAndCollect(ctx context.Context, p provider.Provider, stdout, stderr io.Writer, model, system string, maxTokens int, messages []provider.Message, verbose bool) (string, int) {
	req := &provider.Request{
		Model:     model,
		System:    system,
		MaxTokens: maxTokens,
		Messages:  messages,
	}

	start := time.Now()
	ch, err := p.Stream(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			return "", 4
		}
		fmt.Fprintf(stderr, "piper: %v\n", err)
		return "", 3
	}

	var sb strings.Builder
	var lastDelta string
	for ev := range ch {
		if ev.Err != nil {
			fmt.Fprintf(stderr, "\npiper: %v\n", ev.Err)
			return "", 3
		}
		if ev.Delta != "" {
			fmt.Fprint(stdout, ev.Delta)
			sb.WriteString(ev.Delta)
			lastDelta = ev.Delta
		}
		if ev.Done {
			if !strings.HasSuffix(lastDelta, "\n") {
				fmt.Fprintln(stdout)
			}
			if verbose {
				fmt.Fprintf(stderr, "latency=%s input_tokens=%d output_tokens=%d\n",
					time.Since(start).Round(time.Millisecond), ev.InputTokens, ev.OutputTokens)
			}
			return sb.String(), 0
		}
	}

	if ctx.Err() != nil {
		fmt.Fprintln(stdout)
		return sb.String(), 4
	}
	fmt.Fprintln(stdout)
	return sb.String(), 0
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
