package app

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestClearResultsHandlerClearsFlagsAndWriteups(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	flag := "flag{old}"
	writeupName := "old_wp.md"
	task := &Task{
		ID:              "clear-results",
		Name:            "clear results",
		Category:        "misc",
		Description:     "clear",
		Status:          StatusSolved,
		Flag:            &flag,
		WriteupFileName: writeupName,
		CreatedAt:       time.Now().UTC(),
	}
	if err := service.store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := service.store.SaveWriteup(task.ID, task.WriteupFileName, "# old\n"); err != nil {
		t.Fatalf("SaveWriteup: %v", err)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/maintenance/clear-results", "application/json", nil)
	if err != nil {
		t.Fatalf("POST clear-results: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear-results status=%d", resp.StatusCode)
	}
	got, _ := service.store.Get(task.ID)
	if got.Status != StatusCompleted || got.Flag != nil || got.WriteupFileName != "" {
		t.Fatalf("result data was not cleared: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(service.cfg.ChallengeDir, task.ID, writeupName)); !os.IsNotExist(err) {
		t.Fatalf("writeup file was not removed: %v", err)
	}
}

func TestTaskWriteupHandlerDownloadsSolvedWriteup(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	flag := "FLAG-12345"
	task := &Task{
		ID:              "download-writeup",
		Name:            "download writeup",
		Category:        "misc",
		Description:     "download",
		Status:          StatusSolved,
		Flag:            &flag,
		WriteupFileName: "download-writeup-wp.md",
		CreatedAt:       time.Now().UTC(),
	}
	if err := service.store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := service.store.SaveWriteup(task.ID, task.WriteupFileName, "# WP\n\nFLAG-12345\n"); err != nil {
		t.Fatalf("SaveWriteup: %v", err)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	detail, err := http.Get(server.URL + "/api/tasks/" + task.ID)
	if err != nil {
		t.Fatalf("GET task detail: %v", err)
	}
	defer detail.Body.Close()
	var payload taskResponse
	if err := json.NewDecoder(detail.Body).Decode(&payload); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	if !payload.HasWriteup || payload.WriteupFileName != task.WriteupFileName {
		t.Fatalf("writeup metadata missing: %+v", payload)
	}

	resp, err := http.Get(server.URL + "/api/tasks/" + task.ID + "/writeup")
	if err != nil {
		t.Fatalf("GET writeup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("writeup status=%d", resp.StatusCode)
	}
	body := new(bytes.Buffer)
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read writeup body: %v", err)
	}
	if !strings.Contains(body.String(), "FLAG-12345") {
		t.Fatalf("writeup body=%q", body.String())
	}
}

func TestAuthMiddlewareProtectsApplicationRoutes(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	service.cfg.AccessToken = "test-access-token"
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

	unauthorized, err := http.Get(server.URL + "/api/tasks")
	if err != nil {
		t.Fatalf("GET unauthorized tasks: %v", err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d want %d", unauthorized.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/tasks", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-access-token")
	authorized, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET authorized tasks: %v", err)
	}
	defer authorized.Body.Close()
	if authorized.StatusCode != http.StatusOK {
		t.Fatalf("authorized status=%d want %d", authorized.StatusCode, http.StatusOK)
	}
}

func TestCorsMiddlewareUsesExplicitAllowedOrigins(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	service.cfg.AllowedOrigins = []string{"https://ctf.example"}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	req, err := http.NewRequest(http.MethodOptions, server.URL+"/api/tasks", nil)
	if err != nil {
		t.Fatalf("NewRequest allowed: %v", err)
	}
	req.Header.Set("Origin", "https://ctf.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS allowed: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://ctf.example" {
		t.Fatalf("allowed origin header=%q", got)
	}

	blockedReq, err := http.NewRequest(http.MethodOptions, server.URL+"/api/tasks", nil)
	if err != nil {
		t.Fatalf("NewRequest blocked: %v", err)
	}
	blockedReq.Header.Set("Origin", "https://evil.example")
	blocked, err := http.DefaultClient.Do(blockedReq)
	if err != nil {
		t.Fatalf("OPTIONS blocked: %v", err)
	}
	defer blocked.Body.Close()
	if got := blocked.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("blocked origin should not receive CORS header, got %q", got)
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

func TestTaskLogsTailQuery(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	task := &Task{
		ID:          "tail-task",
		Name:        "tail task",
		Category:    "misc",
		Description: "demo task",
		Status:      StatusRunning,
		CreatedAt:   time.Now().UTC(),
	}
	if err := service.store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := service.store.AppendLog(task.ID, "alpha\nbravo\ncharlie\n"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/tasks/" + task.ID + "/logs?tail=8")
	if err != nil {
		t.Fatalf("GET task logs tail: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logs status=%d", resp.StatusCode)
	}
	var payload struct {
		Logs string `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode logs: %v", err)
	}
	if payload.Logs != "charlie\n" {
		t.Fatalf("logs=%q", payload.Logs)
	}
}

func TestCreateTaskReturnsTooManyRequestsWhenQueueFull(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	service.Close()
	for i := 0; i < cap(service.queue); i++ {
		service.queue <- "blocked-" + strconvItoa(i)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"name":        "queue-full",
		"type":        "misc",
		"description": "queue full",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField(%s): %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close multipart writer: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/tasks", writer.FormDataContentType(), body)
	if err != nil {
		t.Fatalf("POST /api/tasks: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status=%d want %d", resp.StatusCode, http.StatusTooManyRequests)
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

func TestServicePrefersExplicitProviderFormatEnvOverSavedState(t *testing.T) {
	t.Setenv("OPENCODE_PROVIDER_FORMAT", ProviderFormatOpenAICompatible)
	dataDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dataDir, "provider.json"),
		[]byte(`{"format":"anthropic"}`),
		0o600,
	); err != nil {
		t.Fatalf("write provider state: %v", err)
	}

	service, err := NewService(Config{
		DataDir:                dataDir,
		ChallengeDir:           filepath.Join(dataDir, "challenges"),
		DockerImage:            "ctf-agent-opencode:latest",
		MaxContainers:          1,
		OpenCodeProviderFormat: ProviderFormatOpenAICompatible,
		OpenCodeProviders: map[string]OpenCodeProviderConfig{
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
		},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer service.Close()

	if got := service.ActiveProviderFormat(); got != ProviderFormatOpenAICompatible {
		t.Fatalf("ActiveProviderFormat=%q want %q", got, ProviderFormatOpenAICompatible)
	}
}

func TestTaskMessagesHandlerStateRules(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	runnerMessages := make(chan string, 1)
	service.runDockerHint = func(_ context.Context, _ Config, task *Task, message string, logSink LogSink) (DockerResult, error) {
		runnerMessages <- message
		logSink("Observation: final readable OpenCode output:\n还需要继续分析。\n")
		return DockerResult{ExitCode: 1, ContainerName: task.ContainerName, Retained: true}, nil
	}
	now := time.Now().UTC()
	flag := "flag{done}"
	for _, task := range []*Task{
		{
			ID:            "running-message",
			Name:          "running-message",
			Category:      "misc",
			Description:   "running",
			Status:        StatusRunning,
			ContainerName: "ctf-agent-running-message",
			ContainerKept: true,
			CreatedAt:     now,
		},
		{
			ID:              "failed-message",
			Name:            "failed-message",
			Category:        "misc",
			Description:     "failed retained",
			Status:          StatusFailed,
			ContainerName:   "ctf-agent-failed-message",
			ContainerKept:   true,
			OpenCodeSession: "ses_failed",
			CreatedAt:       now.Add(time.Second),
		},
		{
			ID:              "solved-message",
			Name:            "solved-message",
			Category:        "misc",
			Description:     "solved",
			Status:          StatusSolved,
			Flag:            &flag,
			ContainerName:   "ctf-agent-solved-message",
			ContainerKept:   true,
			OpenCodeSession: "ses_solved",
			CreatedAt:       now.Add(2 * time.Second),
		},
	} {
		if err := service.store.Add(task); err != nil {
			t.Fatalf("Add(%s): %v", task.ID, err)
		}
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()
	postMessage := func(taskID string) *http.Response {
		t.Helper()
		resp, err := http.Post(
			server.URL+"/api/tasks/"+taskID+"/messages",
			"application/json",
			bytes.NewBufferString(`{"message":"try another path"}`),
		)
		if err != nil {
			t.Fatalf("POST message %s: %v", taskID, err)
		}
		return resp
	}

	running := postMessage("running-message")
	defer running.Body.Close()
	if running.StatusCode != http.StatusConflict {
		t.Fatalf("running message status=%d want %d", running.StatusCode, http.StatusConflict)
	}

	solved := postMessage("solved-message")
	defer solved.Body.Close()
	if solved.StatusCode != http.StatusAccepted {
		t.Fatalf("solved retained message status=%d want %d", solved.StatusCode, http.StatusAccepted)
	}

	failed := postMessage("failed-message")
	defer failed.Body.Close()
	if failed.StatusCode != http.StatusAccepted {
		t.Fatalf("failed retained message status=%d want %d", failed.StatusCode, http.StatusAccepted)
	}
	select {
	case got := <-runnerMessages:
		if got != "try another path" {
			t.Fatalf("runner message=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected retained failed task to continue through Docker hint runner")
	}
}

func TestTerminalTaskResponseMessageStatusServerDriven(t *testing.T) {
	t.Parallel()

	response := newTaskResponse(&Task{
		ID:              "terminal-response",
		Name:            "terminal-response",
		Category:        "misc",
		Description:     "failed retained",
		Status:          StatusFailed,
		ContainerName:   "ctf-agent-terminal-response",
		ContainerKept:   true,
		OpenCodeSession: "ses_terminal",
		CreatedAt:       time.Now().UTC(),
	})
	if response.OpenCodeSession != "ses_terminal" || !response.OpenCodeAvailable || response.OpenCodeStatus != "ready" {
		t.Fatalf("unexpected opencode state: %+v", response)
	}
	if !response.CanSendMessage || response.MessageStatus == "" {
		t.Fatalf("message state not server driven: %+v", response)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal response: %v", err)
	}
	if bytes.Contains(encoded, []byte("opencode_web_url")) || bytes.Contains(encoded, []byte("opencode_host_port")) {
		t.Fatalf("terminal response leaked legacy web fields: %s", encoded)
	}
}

func TestRemovedOpenCodeRoute(t *testing.T) {
	t.Parallel()

	service := newTestService(t)
	defer service.Close()
	task := &Task{
		ID:              "terminal-only",
		Name:            "terminal-only",
		Category:        "misc",
		Description:     "terminal",
		Status:          StatusFailed,
		ContainerName:   "ctf-agent-terminal-only",
		ContainerKept:   true,
		OpenCodeSession: "ses_terminal",
		CreatedAt:       time.Now().UTC(),
	}
	if err := service.store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/tasks/" + task.ID + "/opencode")
	if err != nil {
		t.Fatalf("GET removed opencode route: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("removed opencode route status=%d want %d", resp.StatusCode, http.StatusNotFound)
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
