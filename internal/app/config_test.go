package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeCategory(t *testing.T) {
	t.Parallel()

	if got := normalizeCategory(" reverse-engineering "); got != "reverse_engineering" {
		t.Fatalf("normalizeCategory()=%q", got)
	}
}

func TestImageForCategoryUsesOverrides(t *testing.T) {
	t.Parallel()

	cfg := Config{
		DockerImage:    "ctf-agent-opencode:latest",
		CategoryImages: map[string]string{"pwn": "ctf-agent-pwn:latest"},
	}

	if got := cfg.ImageForCategory("pwn"); got != "ctf-agent-pwn:latest" {
		t.Fatalf("pwn image=%q", got)
	}
	if got := cfg.ImageForCategory("misc"); got != "ctf-agent-opencode:latest" {
		t.Fatalf("fallback image=%q", got)
	}
}

func TestOpenCodeSessionURLUsesWorkspaceSlug(t *testing.T) {
	t.Parallel()

	cfg := Config{OpenCodeWebPublicBase: "http://127.0.0.1"}
	got := cfg.OpenCodeSessionURL("49152", "ses_demo")
	want := "http://127.0.0.1:49152/L3dvcmtzcGFjZQ/session/ses_demo"
	if got != want {
		t.Fatalf("OpenCodeSessionURL()=%q want %q", got, want)
	}
}

func TestLoadConfigDefaultsRuntimePaths(t *testing.T) {
	t.Setenv("CTF_AGENT_AGENT_SCRIPT", "")
	t.Setenv("CTF_AGENT_SKILLS_DIR", "")

	cfg := LoadConfig()

	if got := filepath.ToSlash(cfg.AgentScript); !strings.HasSuffix(got, "runtime/opencode/bridge.py") {
		t.Fatalf("AgentScript=%q", got)
	}
	if got := filepath.ToSlash(cfg.SkillsDir); !strings.HasSuffix(got, "runtime/opencode/skills") {
		t.Fatalf("SkillsDir=%q", got)
	}
}

func TestLoadConfigBuildsOpenCodeProviderProfiles(t *testing.T) {
	t.Setenv("OPENCODE_PROVIDER_ID", "ctf")
	t.Setenv("OPENCODE_PROVIDER_NAME", "CTF Gateway")
	t.Setenv("OPENCODE_PROVIDER_NPM", "@ai-sdk/openai-compatible")
	t.Setenv("OPENCODE_BASE_URL", "https://gateway.example/v1")
	t.Setenv("OPENCODE_API_KEY", "openai-key")
	t.Setenv("OPENCODE_MODEL", "gpt-demo")
	t.Setenv("OPENCODE_ANTHROPIC_API_KEY", "anthropic-key")
	t.Setenv("OPENCODE_ANTHROPIC_MODEL", "claude-demo")
	t.Setenv("OPENCODE_PROVIDER_FORMAT", "anthropic")

	cfg := LoadConfig()

	if cfg.OpenCodeProviderFormat != ProviderFormatAnthropic {
		t.Fatalf("OpenCodeProviderFormat=%q", cfg.OpenCodeProviderFormat)
	}
	if cfg.OpenCodeProviderNPM != "@ai-sdk/anthropic" || cfg.OpenCodeModel != "claude-demo" {
		t.Fatalf("active provider not anthropic: npm=%q model=%q", cfg.OpenCodeProviderNPM, cfg.OpenCodeModel)
	}
	openAI, ok := cfg.ProviderForFormat(ProviderFormatOpenAICompatible)
	if !ok || openAI.APIKey != "openai-key" || openAI.Model != "gpt-demo" {
		t.Fatalf("openai profile=%+v ok=%v", openAI, ok)
	}
	anthropic, ok := cfg.ProviderForFormat(ProviderFormatAnthropic)
	if !ok || anthropic.APIKey != "anthropic-key" || anthropic.Model != "claude-demo" {
		t.Fatalf("anthropic profile=%+v ok=%v", anthropic, ok)
	}
}
