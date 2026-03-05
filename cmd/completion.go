package cmd

import (
	"fmt"
	"io"
)

// printCompletion writes a shell completion script to stdout and returns an exit code.
func printCompletion(stdout, stderr io.Writer, shell string) int {
	switch shell {
	case "zsh":
		fmt.Fprint(stdout, zshCompletion)
		return 0
	default:
		fmt.Fprintf(stderr, "piper: unsupported shell %q for completion (supported: zsh)\n", shell)
		return 1
	}
}

// zshCompletion is a zsh completion script for piper.
// Install with: piper --completion zsh > "${fpath[1]}/_piper"
const zshCompletion = `#compdef piper

_piper() {
  local context state line
  typeset -A opt_args

  local -a anthropic_models openai_models providers

  anthropic_models=(
    'claude-opus-4-20250514:Most capable Anthropic model'
    'claude-sonnet-4-20250514:Balanced performance and speed (default)'
    'claude-haiku-4-5-20251001:Fastest and most compact Anthropic model'
  )

  openai_models=(
    'gpt-4o:OpenAI flagship multimodal model'
    'gpt-4o-mini:Lightweight and fast OpenAI model'
    'gpt-4-turbo:GPT-4 Turbo'
    'gpt-3.5-turbo:Fast and cost-effective OpenAI model'
    'o1:OpenAI o1 reasoning model'
    'o3-mini:OpenAI o3 mini reasoning model'
  )

  providers=(
    'anthropic:Anthropic (Claude models)'
    'openai:OpenAI or compatible API'
  )

  _arguments -s \
    '(-m --model)'{-m,--model}'[Model to use]:model:->model' \
    '(-s --system)'{-s,--system}'[System prompt]:system prompt: ' \
    '(-t --tokens)'{-t,--tokens}'[Max output tokens]:tokens: ' \
    '(-p --provider)'{-p,--provider}'[Provider]:provider:(($providers))' \
    '--base-url[API base URL for OpenAI-compatible providers]:url: ' \
    '(-i --interactive)'{-i,--interactive}'[Multi-turn conversation mode (requires TTY)]' \
    '(-r --raw)'{-r,--raw}'[Disable markdown rendering (default)]' \
    '--no-stream[Disable streaming, wait for full response]' \
    '(-v --verbose)'{-v,--verbose}'[Print metadata to stderr]' \
    '--completion[Generate shell completion script]:shell:(zsh bash fish)' \
    '--version[Print version and exit]' \
    '*:context: '

  case $state in
    model)
      local provider="${opt_args[-p]:-${opt_args[--provider]:-anthropic}}"
      case $provider in
        openai)
          _describe 'OpenAI models' openai_models
          ;;
        *)
          _describe 'Anthropic models' anthropic_models
          ;;
      esac
      ;;
  esac
}

_piper "$@"
`
