package app

import (
	"os"
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

func TestValidateServerSecurityRequiresTokenForPublicBind(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"127.0.0.1:8000", "localhost:8000", "[::1]:8000"} {
		if err := (Config{Addr: addr}).ValidateServerSecurity(); err != nil {
			t.Fatalf("loopback addr %q should not require token: %v", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:8000", ":8000", "192.0.2.10:8000"} {
		if err := (Config{Addr: addr}).ValidateServerSecurity(); err == nil {
			t.Fatalf("public addr %q should require access token", addr)
		}
	}
	if err := (Config{Addr: "0.0.0.0:8000", AccessToken: "token"}).ValidateServerSecurity(); err != nil {
		t.Fatalf("public addr with token should be valid: %v", err)
	}
}

func TestSplitCSVTrimsEmptyItems(t *testing.T) {
	t.Parallel()

	got := splitCSV(" https://a.example, ,https://b.example ")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("splitCSV=%v", got)
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

func TestLoadConfigReadsLocalOpenCodeEnvFile(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})
	for _, name := range []string{
		"OPENCODE_PROVIDER_FORMAT",
		"OPENCODE_OPENAI_PROVIDER_ID",
		"OPENCODE_OPENAI_PROVIDER_NPM",
		"OPENCODE_OPENAI_BASE_URL",
		"OPENCODE_OPENAI_API_KEY",
		"OPENCODE_OPENAI_MODEL",
	} {
		t.Setenv(name, "")
	}
	content := strings.Join([]string{
		"OPENCODE_PROVIDER_FORMAT=openai-compatible",
		"OPENCODE_OPENAI_PROVIDER_ID=ctf",
		"OPENCODE_OPENAI_PROVIDER_NPM=@ai-sdk/openai-compatible",
		"OPENCODE_OPENAI_BASE_URL=https://gateway.example/v1",
		"OPENCODE_OPENAI_API_KEY=local-key",
		"OPENCODE_OPENAI_MODEL=local-model",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, "opencode.env"), []byte(content), 0o600); err != nil {
		t.Fatalf("write opencode.env: %v", err)
	}

	cfg := LoadConfig()

	if cfg.OpenCodeProviderFormat != ProviderFormatOpenAICompatible {
		t.Fatalf("OpenCodeProviderFormat=%q", cfg.OpenCodeProviderFormat)
	}
	if cfg.OpenCodeBaseURL != "https://gateway.example/v1" ||
		cfg.OpenCodeAPIKey != "local-key" ||
		cfg.OpenCodeModel != "local-model" {
		t.Fatalf("local opencode.env was not loaded: base=%q key_len=%d model=%q", cfg.OpenCodeBaseURL, len(cfg.OpenCodeAPIKey), cfg.OpenCodeModel)
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

func TestOpenCodeProviderEnvRewritesLoopbackBaseURLForDocker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "localhost",
			raw:  "http://localhost:8317/v1",
			want: "http://host.docker.internal:8317/v1",
		},
		{
			name: "ipv4 loopback",
			raw:  "http://127.0.0.1:8317/v1",
			want: "http://host.docker.internal:8317/v1",
		},
		{
			name: "ipv6 loopback",
			raw:  "http://[::1]:8317/v1",
			want: "http://host.docker.internal:8317/v1",
		},
		{
			name: "remote gateway",
			raw:  "https://gateway.example/v1",
			want: "https://gateway.example/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dockerReachableBaseURL(tt.raw); got != tt.want {
				t.Fatalf("dockerReachableBaseURL(%q)=%q want %q", tt.raw, got, tt.want)
			}
		})
	}

	env := strings.Join(openCodeProviderEnv(Config{OpenCodeBaseURL: "http://localhost:8317/v1"}), "\n")
	if !strings.Contains(env, "OPENCODE_BASE_URL=http://host.docker.internal:8317/v1") {
		t.Fatalf("openCodeProviderEnv did not rewrite loopback base url:\n%s", env)
	}
}
