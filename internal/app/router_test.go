package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthAndTaskListHandlers(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	if err := service.store.Add(&Task{
		ID:          "task-1",
		Name:        "demo",
		Category:    "misc",
		Description: "demo task",
		Status:      StatusSolved,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	health, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", health.StatusCode)
	}

	tasks, err := http.Get(server.URL + "/api/tasks")
	if err != nil {
		t.Fatalf("GET /api/tasks: %v", err)
	}
	defer tasks.Body.Close()
	var payload struct {
		Tasks []taskResponse `json:"tasks"`
	}
	if err := json.NewDecoder(tasks.Body).Decode(&payload); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	if len(payload.Tasks) != 1 || payload.Tasks[0].ID != "task-1" {
		t.Fatalf("tasks payload=%+v", payload)
	}
}

func TestContainerListShowsRunningAndRetainedUnsolvedContainers(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	now := time.Now().UTC()
	flag := "flag{done}"
	for _, task := range []*Task{
		{
			ID:            "running",
			Name:          "running-demo",
			Category:      "misc",
			Description:   "running task",
			Status:        StatusRunning,
			ContainerName: "ctf-agent-running",
			CreatedAt:     now,
			StartedAt:     &now,
		},
		{
			ID:              "retained",
			Name:            "retained-demo",
			Category:        "pwn",
			Description:     "retained task",
			Status:          StatusFailed,
			ContainerName:   "ctf-agent-retained",
			ContainerKept:   true,
			OpenCodeWebURL:  "http://127.0.0.1:49152",
			OpenCodeSession: "ses_demo",
			CreatedAt:       now.Add(time.Second),
		},
		{
			ID:            "solved",
			Name:          "solved-demo",
			Category:      "crypto",
			Description:   "solved task",
			Status:        StatusSolved,
			Flag:          &flag,
			ContainerName: "ctf-agent-solved",
			CreatedAt:     now.Add(2 * time.Second),
		},
	} {
		if err := service.store.Add(task); err != nil {
			t.Fatalf("Add(%s): %v", task.ID, err)
		}
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/containers")
	if err != nil {
		t.Fatalf("GET /api/containers: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("containers status=%d", resp.StatusCode)
	}
	var payload containerListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode containers: %v", err)
	}
	expected := buildManagedContainerResponses(service.store.List(), map[string]DockerContainer{
		"ctf-agent-running":  {Name: "ctf-agent-running", Image: "ctf-agent-opencode:latest", Status: "Up 2 seconds", Running: true},
		"ctf-agent-retained": {Name: "ctf-agent-retained", Image: "ctf-agent-pwn:latest", Status: "Up 1 minute", Running: true},
	}, true, service.cfg)
	if len(expected) != 2 {
		t.Fatalf("expected containers=%+v", expected)
	}
	seen := map[string]bool{}
	for _, item := range expected {
		seen[item.TaskID] = true
	}
	if !seen["running"] || !seen["retained"] {
		t.Fatalf("expected states=%v", seen)
	}
	seenState := map[string]string{}
	for _, item := range payload.Containers {
		if item.ContainerState == "missing" {
			continue
		}
		seenState[item.TaskID] = item.ContainerState
	}
	if _, ok := seenState["solved"]; ok {
		t.Fatalf("solved container should not be listed: %v", seenState)
	}
}

func TestTaskIDFromWebSocketPath(t *testing.T) {
	t.Parallel()

	id, err := taskIDFromWebSocketPath("/ws/tasks/abc123/logs")
	if err != nil {
		t.Fatalf("taskIDFromWebSocketPath valid: %v", err)
	}
	if id != "abc123" {
		t.Fatalf("id=%q", id)
	}
	if _, err := taskIDFromWebSocketPath("/ws/tasks/../logs"); err == nil {
		t.Fatal("expected invalid path error")
	}
}

func TestProviderSettingsHandlers(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/settings/provider")
	if err != nil {
		t.Fatalf("GET provider settings: %v", err)
	}
	defer resp.Body.Close()
	var settings providerSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatalf("decode provider settings: %v", err)
	}
	if settings.ActiveFormat != ProviderFormatOpenAICompatible || len(settings.Options) != 2 {
		t.Fatalf("settings=%+v", settings)
	}
	for _, option := range settings.Options {
		if option.APIKeyConfigured && option.ProviderNPM == "" {
			t.Fatalf("option leaked malformed config: %+v", option)
		}
	}

	body := bytes.NewBufferString(`{"format":"anthropic"}`)
	update, err := http.Post(server.URL+"/api/settings/provider", "application/json", body)
	if err != nil {
		t.Fatalf("POST provider settings: %v", err)
	}
	defer update.Body.Close()
	if update.StatusCode != http.StatusOK {
		t.Fatalf("update provider status=%d", update.StatusCode)
	}
	var updated providerSettingsResponse
	if err := json.NewDecoder(update.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated settings: %v", err)
	}
	if updated.ActiveFormat != ProviderFormatAnthropic || service.ActiveProviderFormat() != ProviderFormatAnthropic {
		t.Fatalf("updated=%+v active=%q", updated, service.ActiveProviderFormat())
	}
}

func TestSetProviderFormatKeepsActiveFormatWhenPersistFails(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	blocker := filepath.Join(t.TempDir(), "provider-state-blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	service.cfg.DataDir = blocker

	if _, err := service.SetProviderFormat(ProviderFormatAnthropic); err == nil {
		t.Fatal("SetProviderFormat() succeeded despite persist failure")
	}
	if got := service.ActiveProviderFormat(); got != ProviderFormatOpenAICompatible {
		t.Fatalf("active provider changed after persist failure: %q", got)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dataDir := t.TempDir()
	providers := map[string]OpenCodeProviderConfig{
		ProviderFormatOpenAICompatible: {
			Format:       ProviderFormatOpenAICompatible,
			Label:        "OpenAI兼容",
			ProviderID:   "ctf",
			ProviderName: "CTF Gateway",
			ProviderNPM:  "@ai-sdk/openai-compatible",
			BaseURL:      "https://gateway.example/v1",
			APIKey:       "openai-key",
			Model:        "gpt-demo",
		},
		ProviderFormatAnthropic: {
			Format:       ProviderFormatAnthropic,
			Label:        "Anthropic",
			ProviderID:   "anthropic",
			ProviderName: "Anthropic",
			ProviderNPM:  "@ai-sdk/anthropic",
			BaseURL:      "https://api.anthropic.com/v1",
			APIKey:       "anthropic-key",
			Model:        "claude-demo",
		},
	}
	service, err := NewService(Config{
		DataDir:                dataDir,
		ChallengeDir:           filepath.Join(dataDir, "challenges"),
		DockerImage:            "ctf-agent-opencode:latest",
		MaxContainers:          1,
		MemLimit:               "128m",
		CPUs:                   "0.5",
		PidsLimit:              "64",
		OpenCodeProviderFormat: ProviderFormatOpenAICompatible,
		OpenCodeProviders:      providers,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}
