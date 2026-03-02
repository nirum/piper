# Piper

A Unix-native CLI tool that reads text from stdin, sends it to an LLM, and streams the response to stdout. Composable, pipe-friendly, single-binary.

## Usage

```
piper [flags] [CONTEXT...]
```

Positional args are joined as context prepended to stdin.

```bash
cat README.md | piper "summarize in 3 bullets"
git diff main | piper "review for bugs"
curl -s api.example.com/data.json | piper "extract emails" | sort -u
```

Response goes to stdout, errors and metadata go to stderr.

## Flags

```
-m, --model     string   Model (default: claude-sonnet-4-20250514)
-s, --system    string   System prompt (default: "You are a helpful assistant.")
-t, --tokens    int      Max output tokens (default: 4096)
-p, --provider  string   Provider: anthropic, openai (default: anthropic)
    --base-url  string   API base URL (for OpenAI-compat providers)
-r, --raw                Disable markdown rendering, raw text (default)
    --no-stream          Disable streaming
-v, --verbose            Metadata to stderr
    --version            Print version
```

## Providers

Piper supports **Anthropic** and **OpenAI-compatible** APIs out of the box. The OpenAI-compatible provider works with GPT, Ollama, and local models via `--base-url`.

## Configuration

Optional config file at `~/.config/piper/config.toml`:

```toml
provider = "anthropic"
model = "claude-sonnet-4-20250514"
api_key = "sk-ant-..."

[defaults]
max_tokens = 4096
system = "You are a helpful assistant."
```

API keys are resolved in order: environment variable (`ANTHROPIC_API_KEY` / `OPENAI_API_KEY`) then config file. Flags override everything.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Usage error (bad flags, no stdin) |
| 2 | Config/auth error (missing API key) |
| 3 | API error (rate limit, network, server) |
| 4 | Interrupted (SIGINT/SIGTERM) |

## Building

```bash
go build -o piper .
```

## Testing

```bash
go test ./...
```

Integration tests require a real API key and are gated behind `PIPER_INTEGRATION=1`.
