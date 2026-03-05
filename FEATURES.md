# Piper Feature Improvements

Proposed new features for piper, organized by priority.

---

## Priority Ranking

| # | Feature | Complexity | Value |
|---|---------|-----------|-------|
| 1 | Request Timeout | Low | High |
| 2 | Retry Logic | Low-Med | High |
| 3 | File Input (`-f`) | Low | High |
| 4 | Named Profiles | Med | High |
| 5 | Interactive Mode | High | High |
| 6 | Output to File (`-o`) | Low | Med |
| 7 | Token Budget Warning | Low | Med |
| 8 | Template System | Med | Med |
| 9 | Shell Completions | Med | Med |
| 10 | Progress Indicator | Med | Med |
| 11 | Conversation History | High | Med |
| 12 | Structured Output | High | Med-High |

---

## Feature Details

### 1. Request Timeout (`--timeout DURATION`)

Set a maximum wall-clock time for the entire request. No timeout exists today, so a slow API can hang forever.

- Add `--timeout DURATION` flag (default: `5m`)
- Use `context.WithTimeout()` wrapping the existing signal context
- Report exit code 3 on timeout with a clear message

**Files:** `cmd/root.go`

---

### 2. Retry Logic for Transient Errors

Automatically retry on rate limits (429) and server errors (5xx) with exponential backoff. Currently any API error is fatal.

- Add `--retries INT` flag (default: 3)
- Detect retryable HTTP status codes in both providers
- Sleep with jitter: `2^n * 100ms + rand(0..100ms)`
- Log retry attempts to stderr in verbose mode

**Files:** `provider/anthropic.go`, `provider/openai.go`, `cmd/root.go`

---

### 3. File Input (`-f FILE`)

Read context from a file instead of (or in addition to) stdin. Useful in scripts where piping is awkward.

- Add `-f FILE` flag; read and append its content to the user message
- Allow combining: `piper -f mycode.go "explain this"` (no stdin needed)
- Support multiple `-f` flags; each file gets its filename as a header

**Files:** `cmd/root.go`

---

### 4. Named Profiles in Config

Support multiple named configuration profiles for different use cases.

```toml
[profiles.coding]
model = "claude-opus-4-20250514"
system = "You are a senior software engineer."

[profiles.quick]
model = "claude-haiku-4-5-20251001"
max_tokens = 512
```

- Add `--profile NAME` flag
- Profile sections override defaults
- Example: `cat code.py | piper --profile coding "review"`

**Files:** `config/config.go`, `cmd/root.go`

---

### 5. Interactive / Multi-turn Mode (`-i`)

Allow back-and-forth conversation without re-piping. Maintains history in memory for the session.

- Add `-i` flag to enter a REPL loop
- Keep a `[]Message` slice, append each turn
- Print a prompt (`>>> `) to stderr, read from stdin interactively
- Send full history on each turn

**Files:** `cmd/root.go`, `provider/provider.go` (multi-message Request)

---

### 6. Output to File (`-o FILE`)

Write response to a file instead of stdout.

- Add `-o FILE` flag
- Stream response to the file instead of `os.Stdout`
- Example: `cat doc.md | piper -o summary.md "summarize"`

**Files:** `cmd/root.go`

---

### 7. Token Budget Warning

Warn when input is large relative to the token limit, before sending.

- Heuristic: estimate ~4 chars/token, warn to stderr if input > 80% of `--tokens`
- Add `--max-input-tokens INT` to hard-limit and truncate (with warning)
- Verbose mode shows estimated input token count before sending

**Files:** `cmd/root.go`

---

### 8. Template System (`--template NAME`)

Pre-defined prompt templates to avoid retyping common prompts.

- Templates stored in `~/.config/piper/templates/` as `.txt` files
- `--template review` loads `review.txt` as the context arg
- List available templates with `piper --list-templates`

**Files:** New `templates/` package, `cmd/root.go`

---

### 9. Shell Completion Generation

Generate shell completion scripts for bash, zsh, and fish.

- Add `piper --completion bash|zsh|fish` subcommand
- Include known flag completions
- Include common model names as completions for `-m`

**Files:** `cmd/completion.go` (new)

---

### 10. Progress Indicator

Show a spinner on stderr when stdout is redirected (non-TTY), so the user knows the tool isn't hung.

- Detect when stdout is redirected
- Show a spinner on stderr: `⠋ streaming...` updated every 100ms
- Clear on completion or error

**Files:** `cmd/root.go`

---

### 11. Conversation History (`--history FILE`)

Persist conversation turns to a file so context survives across invocations.

- `--history FILE` reads prior turns from a JSONL file at startup
- Appends current turn on completion
- `--history-clear` truncates the file

**Files:** `cmd/root.go`, new `history/history.go`

---

### 12. Structured Output (`--json-schema FILE`)

Request validated JSON output from the model. Useful for LLM-as-data-pipeline workflows.

- Pass JSON schema to Anthropic's structured output API
- Validate response against schema
- Print validated JSON to stdout for `jq` processing

**Files:** `provider/anthropic.go`, `cmd/root.go`
