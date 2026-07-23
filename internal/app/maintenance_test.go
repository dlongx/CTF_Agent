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
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestHubDeliversAndUnsubscribes(t *testing.T) {
	t.Parallel()
	hub := NewHub()
	logs := hub.Subscribe("task")
	events := hub.SubscribeEvents()
	hub.Publish("task", "line")
	hub.PublishEvent(`{"type":"task_changed"}`)
	if got := <-logs; got != "line" {
		t.Fatalf("log=%q", got)
	}
	if got := <-events; !strings.Contains(got, "task_changed") {
		t.Fatalf("event=%q", got)
	}
	hub.Unsubscribe("task", logs)
	hub.UnsubscribeEvents(events)
	if _, ok := <-logs; ok {
		t.Fatal("log subscription was not closed")
	}
	if _, ok := <-events; ok {
		t.Fatal("event subscription was not closed")
	}
	// Unknown subscriptions are harmless.
	hub.Unsubscribe("missing", make(chan string))
	hub.UnsubscribeEvents(make(chan string))
}

func TestStorageSanitizesAndDeduplicatesUploads(t *testing.T) {
	t.Parallel()
	root := tempDirWithRemoveRetry(t)
	attachments, err := prepareTaskDirs(root, "upload-task")
	if err != nil {
		t.Fatalf("prepareTaskDirs: %v", err)
	}
	if _, err := prepareTaskDirs(root, "../escape"); err == nil {
		t.Fatal("unsafe task ID was accepted")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, name := range []string{"../bad name.txt", "bad name.txt"} {
		part, err := writer.CreateFormFile("attachments", name)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		_, _ = part.Write([]byte(name))
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	defer req.MultipartForm.RemoveAll()
	count, err := saveUploadedFiles(req.MultipartForm.File["attachments"], attachments)
	if err != nil || count != 2 {
		t.Fatalf("saveUploadedFiles=(%d,%v)", count, err)
	}
	entries, _ := os.ReadDir(attachments)
	if len(entries) != 2 || entries[0].Name() == entries[1].Name() {
		t.Fatalf("uploaded entries=%v", entries)
	}
	if safeFilename("") != "attachment" || !strings.HasSuffix(safeFilename(strings.Repeat("a", 220)+".txt"), "a") {
		t.Fatal("safeFilename fallback or truncation failed")
	}
	if !fileExists(filepath.Join(attachments, entries[0].Name())) {
		t.Fatal("saved file was not found")
	}
}

func TestFilenameAndSummaryHelpers(t *testing.T) {
	t.Parallel()
	if got := writeupFilenameForTask(&Task{Name: " Demo / Task "}); got != "Demo-Task-wp.md" {
		t.Fatalf("writeup filename=%q", got)
	}
	if got := safeFilenameStem("***"); got != "" {
		t.Fatalf("safe stem=%q", got)
	}
	if got := summarizeLastStep("[opencode] INFO ignored\n\nuseful step\n"); got != "useful step" {
		t.Fatalf("last step=%q", got)
	}
	if got := summarizeLastStep(""); got != "暂无可用步骤" {
		t.Fatalf("empty last step=%q", got)
	}
	if got := summarizeLastStep(strings.Repeat("x", 300)); len(got) != 220 {
		t.Fatalf("summary length=%d", len(got))
	}
}

func TestDockerOOMHelpers(t *testing.T) {
	t.Parallel()

	count, ok := parseDockerOOMKillCount("low 0\noom 3\noom_kill 2\n")
	if !ok || count != 2 {
		t.Fatalf("parseDockerOOMKillCount=(%d,%v)", count, ok)
	}
	if _, ok := parseDockerOOMKillCount("oom 3\n"); ok {
		t.Fatal("missing oom_kill entry was accepted")
	}
	if !dockerOOMOccurred(
		dockerOOMSnapshot{killCount: 1, hasCount: true},
		dockerOOMSnapshot{killCount: 2, hasCount: true},
	) {
		t.Fatal("increased cgroup oom_kill count was not detected")
	}
	if dockerOOMOccurred(
		dockerOOMSnapshot{killCount: 2, hasCount: true},
		dockerOOMSnapshot{killCount: 2, hasCount: true},
	) {
		t.Fatal("unchanged cgroup oom_kill count was treated as a new OOM")
	}
	if !dockerOOMOccurred(
		dockerOOMSnapshot{hasState: true},
		dockerOOMSnapshot{killed: true, hasState: true},
	) {
		t.Fatal("Docker OOM state transition was not detected")
	}
}

func TestAutoDockerResourceLimits(t *testing.T) {
	t.Parallel()

	limits := autoDockerResourceLimits(16, 8<<30)
	if limits.memory != "7168m" || limits.cpus != "15" {
		t.Fatalf("auto limits=%+v", limits)
	}
	minimum := autoDockerResourceLimits(1, 1<<30)
	if minimum.memory != "512m" || minimum.cpus != "1" {
		t.Fatalf("minimum auto limits=%+v", minimum)
	}
	explicit := resolveDockerResourceLimits("2g", "3.5")
	if explicit.memory != "2g" || explicit.cpus != "3.5" {
		t.Fatalf("explicit limits=%+v", explicit)
	}
}

func TestReadinessHelpers(t *testing.T) {
	t.Parallel()
	root := tempDirWithRemoveRetry(t)
	if got := checkWritableDirectory(filepath.Join(root, "data")); !got.OK || got.Reason != "writable" {
		t.Fatalf("writable check=%+v", got)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := checkWritableDirectory(file); got.OK {
		t.Fatalf("file accepted as directory:%+v", got)
	}
	images := configuredImages(Config{DockerImage: "base", CategoryImages: map[string]string{"misc": "base", "pwn": "pwn"}})
	if strings.Join(images, ",") != "base,pwn" {
		t.Fatalf("images=%v", images)
	}
	if providerHTTPErrorCode(http.StatusUnauthorized) != "unauthorized" ||
		providerHTTPErrorCode(http.StatusForbidden) != "unauthorized" ||
		providerHTTPErrorCode(http.StatusTooManyRequests) != "rate_limited" ||
		providerHTTPErrorCode(522) != "upstream_http_522" {
		t.Fatal("provider status mapping failed")
	}
}

func TestRouterPagesValidationAndLifecycle(t *testing.T) {
	t.Parallel()
	service := newTestService(t)
	service.cfg.AutoContinueRounds = 0
	service.runDockerTask = func(_ context.Context, _ Config, task *Task, sink LogSink, containerSink func(string)) (DockerResult, error) {
		name := "ctf-agent-" + task.ID
		containerSink(name)
		sink("Observation: OpenCode session=ses_router\n")
		sink("Observation: final readable OpenCode output:\n这道题目已经解出\nflag{router_ok}\n")
		return DockerResult{ExitCode: 0, ContainerName: name, Retained: true}, nil
	}
	defer service.Close()
	server := httptest.NewServer(NewRouter(service))
	defer server.Close()

	for _, path := range []string{"/", "/containers", "/tasks/missing"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		_ = resp.Body.Close()
		if path == "/tasks/missing" && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("missing page status=%d", resp.StatusCode)
		}
	}

	bad, err := http.Post(server.URL+"/api/tasks", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad task status=%d", bad.StatusCode)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("name", "router task")
	_ = w.WriteField("type", "misc")
	_ = w.WriteField("description", "solve")
	_ = w.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/tasks", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	created, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer created.Body.Close()
	if created.StatusCode != http.StatusAccepted {
		t.Fatalf("create status=%d", created.StatusCode)
	}
	var task taskResponse
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		stored, _ := service.store.Get(task.ID)
		if stored.Status == StatusSolved {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("submitted task did not complete")
}

func TestDockerCommandIntegration(t *testing.T) {
	fakeDir := buildFakeDocker(t)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_PS", "ctf-agent-live\timage:test\tUp 1 minute\t\t12kB (virtual 1GB)\nctf-agent-old\timage:test\tExited (0)\t\t4kB")
	t.Setenv("FAKE_DOCKER_CREATED", "2026-01-01T00:00:00.000000000Z")
	t.Setenv("FAKE_DOCKER_CAT", "# WP\nflag{fake}\n")

	containers, err := ListDockerContainers()
	if err != nil || len(containers) != 2 || !containers["ctf-agent-live"].Running || containers["ctf-agent-old"].Running {
		t.Fatalf("ListDockerContainers=%+v err=%v", containers, err)
	}
	created, err := DockerContainerCreatedAt("ctf-agent-old")
	if err != nil || created.Year() != 2026 {
		t.Fatalf("DockerContainerCreatedAt=%v err=%v", created, err)
	}
	if _, err := DockerContainerCreatedAt("database"); err == nil {
		t.Fatal("unmanaged inspect was accepted")
	}
	content, err := ReadContainerWorkspaceFile(context.Background(), "ctf-agent-live", "demo-wp.md")
	if err != nil || !strings.Contains(content, "flag{fake}") {
		t.Fatalf("ReadContainerWorkspaceFile=%q err=%v", content, err)
	}
	if _, err := ReadContainerWorkspaceFile(context.Background(), "ctf-agent-live", "../secret"); err == nil {
		t.Fatal("unsafe workspace path was accepted")
	}

	root := tempDirWithRemoveRetry(t)
	config := Config{
		DockerImage: "image:test", CategoryImages: map[string]string{"misc": "image:test"},
		MemLimit: "256m", CPUs: "0.5", PidsLimit: "64", DisableNetwork: true,
		ChallengeDir: filepath.Join(root, "challenges"),
		AgentScript:  filepath.Join(root, "bridge.py"), SkillsDir: filepath.Join(root, "skills"),
		OpenCodeProviderID: "ctf", OpenCodeProviderName: "fake", OpenCodeProviderNPM: "npm",
		OpenCodeBaseURL: "http://127.0.0.1:8317/v1", OpenCodeAPIKey: "key", OpenCodeModel: "model",
		OpenCodeRunTimeout: time.Minute, OpenCodeIdleTimeout: time.Second,
	}
	_ = os.WriteFile(config.AgentScript, []byte("pass"), 0o644)
	_ = os.MkdirAll(config.SkillsDir, 0o755)
	attachments := filepath.Join(root, "attachments")
	_ = os.MkdirAll(attachments, 0o755)
	task := &Task{ID: "docker-task", Name: "demo", Category: "misc", Description: "solve", AttachmentsDir: attachments}
	t.Setenv("FAKE_DOCKER_EXEC", "bridge output\n")
	var logs strings.Builder
	var captured string
	result, err := RunDockerTask(context.Background(), config, task, func(text string) { logs.WriteString(text) }, func(name string) { captured = name })
	if err != nil || result.ExitCode != 0 || !result.Retained || captured != "ctf-agent-docker-task" {
		t.Fatalf("RunDockerTask=%+v captured=%q err=%v logs=%s", result, captured, err, logs.String())
	}
	task.ContainerName = captured
	task.ContainerKept = true
	task.OpenCodeSession = "ses_fake"
	hintResult, err := RunDockerHint(context.Background(), config, task, "continue", func(text string) { logs.WriteString(text) })
	if err != nil || hintResult.ExitCode != 0 || !hintResult.Retained {
		t.Fatalf("RunDockerHint=%+v err=%v", hintResult, err)
	}
	if err := CloseTaskContainer(captured); err != nil {
		t.Fatalf("CloseTaskContainer: %v", err)
	}

	service := &Service{cfg: config, store: mustTestStore(t), hub: NewHub(), activeProviderFormat: ProviderFormatOpenAICompatible}
	config.OpenCodeProviders = map[string]OpenCodeProviderConfig{ProviderFormatOpenAICompatible: {
		Format: ProviderFormatOpenAICompatible, ProviderID: "ctf", ProviderNPM: "npm", BaseURL: "http://127.0.0.1:8317/v1", APIKey: "key", Model: "model",
	}}
	service.cfg = config
	ready := service.Readiness(context.Background())
	if !ready.OK {
		t.Fatalf("Readiness=%+v", ready)
	}
}

func mustTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(tempDirWithRemoveRetry(t))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func buildFakeDocker(t *testing.T) string {
	t.Helper()
	dir := tempDirWithRemoveRetry(t)
	if runtime.GOOS == "windows" {
		script := `@echo off
if "%1"=="ps" (powershell -NoProfile -Command "[Console]::Out.Write($env:FAKE_DOCKER_PS)" & exit /b 0)
if "%1"=="inspect" (powershell -NoProfile -Command "[Console]::Out.Write($env:FAKE_DOCKER_CREATED)" & exit /b 0)
if "%1"=="exec" (
  echo %* | findstr /c:" cat /workspace/" >nul
  if not errorlevel 1 (powershell -NoProfile -Command "[Console]::Out.Write($env:FAKE_DOCKER_CAT)" & exit /b 0)
  powershell -NoProfile -Command "[Console]::Out.Write($env:FAKE_DOCKER_EXEC)"
  exit /b 0
)
if "%1"=="run" exit /b 0
if "%1"=="update" exit /b 0
if "%1"=="rm" exit /b 0
if "%1"=="version" exit /b 0
if "%1"=="image" exit /b 0
exit /b 2
`
		if err := os.WriteFile(filepath.Join(dir, "docker.cmd"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	script := `#!/bin/sh
case "$1" in
  ps) printf '%s' "$FAKE_DOCKER_PS" ;;
  inspect) printf '%s' "$FAKE_DOCKER_CREATED" ;;
  exec)
    case " $* " in
      *" cat /workspace/"*) printf '%s' "$FAKE_DOCKER_CAT" ;;
      *) printf '%s' "$FAKE_DOCKER_EXEC" ;;
    esac ;;
  run|rm|update|version|image) exit 0 ;;
  *) exit 2 ;;
esac
`
	path := filepath.Join(dir, "docker")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
