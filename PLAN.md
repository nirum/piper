# Piper — CLI Tool Plan

## Context
Build a Unix-native CLI tool that reads text from stdin, optionally adds context, sends it to an LLM, and streams the response to stdout. Goal: composable, pipe-friendly, single-binary.

## Key Decisions

- **Language: Go** — single static binary, fast startup, no runtime deps. (`brew install go` to bootstrap)
- **Providers: Anthropic + OpenAI-compatible from day 1** — provider interface with both impls. Covers Claude, GPT, Ollama, local models.
- **Config: `~/.config/piper/config.toml`** — XDG-compliant, human-editable, optional (env + flags suffice)
- **No subcommands** — the binary name *is* the verb

## 1. CLI UX

```
Usage: piper [flags] [CONTEXT...]

Flags:
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

Positional args are joined as context prepended to stdin.

```bash
cat README.md | piper "summarize in 3 bullets"
git diff main | piper "review for bugs"
curl -s api.example.com/data.json | piper "extract emails" | sort -u
```

Rules: response → stdout, errors/metadata → stderr. If stdin is TTY with no input, print usage and exit 1. Streaming on by default.

## 2. Config & Auth

**Resolution order per provider** (first wins):
- Anthropic: `ANTHROPIC_API_KEY` env → `config.toml` `api_key`
- OpenAI: `OPENAI_API_KEY` env → `config.toml` `api_key`

```toml
provider = "anthropic"
model = "claude-sonnet-4-20250514"
api_key = "sk-ant-..."

[defaults]
max_tokens = 4096
system = "You are a helpful assistant."
```

Warn to stderr if config file perms wider than `0600`.

## 3. Provider Abstraction

```go
// provider/provider.go
type Provider interface {
    Complete(ctx context.Context, req *Request) (*Response, error)
    Stream(ctx context.Context, req *Request) (<-chan StreamEvent, error)
}
```

Both impls use `net/http` directly with SSE parsing — no SDK deps.

- **Anthropic:** `POST /v1/messages`, `x-api-key` header, `anthropic-version` header
- **OpenAI-compatible:** `POST /v1/chat/completions`, `Authorization: Bearer` header, configurable base URL (for Ollama, local models)

Select via `--provider` flag or `provider` config field. Default: `anthropic`.

## 4. Safety

- API key only in `x-api-key` header; never logged, never in query params
- `--verbose` prints model, token counts, latency to stderr — never secrets
- Custom `http.Transport` that never logs request/response bodies
- Config file permission check on read
- No built-in PII redaction (better handled upstream; document in README)

## 5. Error Handling & Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Usage error (bad flags, no stdin) |
| 2 | Config/auth error (missing API key) |
| 3 | API error (rate limit, network, server) |
| 4 | Interrupted (SIGINT/SIGTERM) |

Errors to stderr as `piper: <message>`. On SIGINT during streaming, flush received text then exit 4.

## 6. Project Structure

```
piper/
  main.go                  # entry point, signal handling
  cmd/root.go              # flags, stdin, orchestration
  config/config.go         # TOML + env loading, validation
  provider/provider.go     # interface + types + factory
  provider/anthropic.go    # Anthropic HTTP client, SSE parser
  provider/openai.go       # OpenAI-compatible HTTP client, SSE parser
  go.mod
```

## 7. Test Plan

| File | Tests |
|------|-------|
| `config/config_test.go` | TOML parsing, env override, permission check |
| `provider/anthropic_test.go` | Request building, SSE parsing (httptest mock), error mapping |
| `provider/openai_test.go` | Request building, SSE parsing (httptest mock), error mapping |
| `cmd/root_test.go` | Flag parsing, stdin detection, context joining |
| `integration_test.go` | E2E with real API key (gated: `PIPER_INTEGRATION=1`) |

stdlib `testing` only. `httptest.Server` for mocks. Target >80% coverage on `config/` and `provider/`.

## 8. MVP Milestones

1. **Skeleton** — `go mod init`, `main.go`, `config/config.go`, flag parsing with `pflag`
2. **Provider interface + Anthropic** — `Provider` interface, Anthropic HTTP client, SSE streaming
3. **OpenAI-compatible provider** — OpenAI HTTP client, SSE streaming, configurable base URL
4. **CLI wiring** — stdin reading, context joining, streaming output loop, exit codes, `--verbose`
4. **Tests + polish** — unit tests, integration test, README
5. **Distribution** — `.goreleaser.yaml` for cross-compilation

## Verification

```bash
# Basic functionality
echo "hello" | go run . "respond with one word"

# Streaming
cat README.md | go run . "summarize"

# Verbose mode
echo "test" | go run . -v "echo back" 2>meta.txt

# Error cases
go run .                    # exit 1: no stdin
ANTHROPIC_API_KEY= go run . # exit 2: no key

# Tests
go test ./...
```
