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

func TestLoadConfigTaskTimeoutFollowsOpenCodeTimeout(t *testing.T) {
	t.Setenv("OPENCODE_TIMEOUT_SECONDS", "180")
	t.Setenv("CTF_AGENT_TASK_TIMEOUT", "")

	cfg := LoadConfig()

	if cfg.TaskTimeoutSeconds != 240 {
		t.Fatalf("TaskTimeoutSeconds=%d want 240", cfg.TaskTimeoutSeconds)
	}
}

func TestLoadConfigTaskTimeoutClampsShortValue(t *testing.T) {
	t.Setenv("OPENCODE_TIMEOUT_SECONDS", "180")
	t.Setenv("CTF_AGENT_TASK_TIMEOUT", "120")

	cfg := LoadConfig()

	if cfg.TaskTimeoutSeconds != 240 {
		t.Fatalf("TaskTimeoutSeconds=%d want 240", cfg.TaskTimeoutSeconds)
	}
}

func TestLoadConfigTaskTimeoutKeepsLongValue(t *testing.T) {
	t.Setenv("OPENCODE_TIMEOUT_SECONDS", "180")
	t.Setenv("CTF_AGENT_TASK_TIMEOUT", "420")

	cfg := LoadConfig()

	if cfg.TaskTimeoutSeconds != 420 {
		t.Fatalf("TaskTimeoutSeconds=%d want 420", cfg.TaskTimeoutSeconds)
	}
}
