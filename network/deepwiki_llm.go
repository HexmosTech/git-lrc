package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/golbi-ai/agentrouter/llm"
)

// DeepwikiLLM is the network-boundary LLM backend for wiki generation. It
// satisfies deepwiki.Completer. Agent-CLI providers (opencode, claude) shell
// out to the machine's own authenticated CLI; HTTP providers (gemini,
// anthropic, openai) route through agentrouter. Provider selection is
// explicit — there is no silent fallback.
type DeepwikiLLM struct {
	provider string
	model    string
	baseURL  string
	apiKey   string
}

// NewDeepwikiLLM constructs the LLM backend. provider selects the backend
// (opencode, claude, gemini, anthropic, openai). model/baseURL/apiKey are
// backend-specific; empty values defer to the provider's own defaults where
// supported.
func NewDeepwikiLLM(provider, model, baseURL, apiKey string) *DeepwikiLLM {
	return &DeepwikiLLM{provider: provider, model: model, baseURL: baseURL, apiKey: apiKey}
}

// Complete runs a single blocking completion. cwd anchors agent-CLI
// subprocesses to the target repo; system is the optional system prompt;
// prompt is the user content.
func (d *DeepwikiLLM) Complete(ctx context.Context, cwd, system, prompt string) (string, error) {
	full := prompt
	if system != "" {
		full = system + "\n\n" + prompt
	}

	switch d.provider {
	case "", "opencode":
		return runOpencode(ctx, cwd, d.model, full)
	case "claude":
		return runClaude(ctx, cwd, d.model, full)
	case "gemini":
		if d.apiKey == "" {
			return "", errors.New("gemini provider requires an API key (--dw-api-key)")
		}
		return completeHTTP(ctx, cwd, system, prompt, func() (llm.Provider, error) {
			return llm.NewGemini(d.apiKey, d.model)
		})
	case "anthropic":
		if d.apiKey == "" {
			return "", errors.New("anthropic provider requires an API key (--dw-api-key)")
		}
		return completeHTTP(ctx, cwd, system, prompt, func() (llm.Provider, error) {
			return llm.NewAnthropic(d.apiKey, d.model)
		})
	case "openai":
		if d.baseURL == "" {
			return "", errors.New("openai provider requires a base URL (--dw-base-url)")
		}
		return completeHTTP(ctx, cwd, system, prompt, func() (llm.Provider, error) {
			return llm.NewOpenAI(d.baseURL, d.apiKey, d.model)
		})
	default:
		return "", fmt.Errorf("unknown deepwiki provider %q", d.provider)
	}
}

// completeHTTP routes a completion through an agentrouter HTTP provider.
func completeHTTP(ctx context.Context, cwd, system, prompt string, build func() (llm.Provider, error)) (string, error) {
	p, err := build()
	if err != nil {
		return "", err
	}
	resp, err := p.Complete(ctx, llm.Request{
		System:   system,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}},
		Cwd:      cwd,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// runOpencode drives `opencode run` (v2.x: `--auto` permission posture, prompt
// on stdin). opencode v2 dropped the `--dangerously-skip-permissions`/`--dir`
// flags agentrouter targets, so this backend owns its own invocation.
func runOpencode(ctx context.Context, cwd, model, prompt string) (string, error) {
	args := []string{"run", "--auto"}
	if model != "" {
		args = append(args, "-m", model)
	}
	cmd := exec.CommandContext(ctx, "opencode", args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("opencode: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", errors.New("opencode: empty completion")
	}
	return out, nil
}

// runClaude drives `claude -p` (print mode) with the prompt on stdin, using
// --append-system-prompt semantics via a combined prompt.
func runClaude(ctx context.Context, cwd, model, prompt string) (string, error) {
	args := []string{"-p", "--output-format", "text"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", errors.New("claude: empty completion")
	}
	return out, nil
}
