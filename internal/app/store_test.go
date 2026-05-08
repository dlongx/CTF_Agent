package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStorePersistsTaskAndLogs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:              "task-1",
		Name:            "persist-demo",
		Category:        "misc",
		Description:     "persist this task",
		AttachmentsDir:  filepath.Join(root, "task-1", "attachments"),
		AttachmentCount: 2,
		Status:          StatusQueued,
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.AppendLog(task.ID, "Final: flag{persist_ok}\n"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}

	loaded, err := NewStore(root)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	recovered, ok := loaded.Get(task.ID)
	if !ok {
		t.Fatal("task was not recovered")
	}
	if recovered.AttachmentCount != 2 {
		t.Fatalf("AttachmentCount=%d", recovered.AttachmentCount)
	}
	logs, ok := loaded.Logs(task.ID)
	if !ok || logs != "Final: flag{persist_ok}\n" {
		t.Fatalf("Logs()=(%q,%v)", logs, ok)
	}
}

func TestStoreRejectsUnsafeTaskID(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	err = store.Add(&Task{
		ID:          "../escape",
		Name:        "escape",
		Category:    "misc",
		Description: "escape",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("Add() accepted unsafe task id")
	}
	if _, ok := store.Get("../escape"); ok {
		t.Fatal("Get() returned unsafe task id")
	}
}

func TestMarkFlagCompletesRunningTaskMetadata(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "task-mark-flag",
		Name:        "mark flag",
		Category:    "misc",
		Description: "solve it",
		Status:      StatusRunning,
		LastStep:    "任务正在执行",
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.MarkFlag(task.ID, " flag{ok} "); err != nil {
		t.Fatalf("MarkFlag: %v", err)
	}
	got, ok := store.Get(task.ID)
	if !ok {
		t.Fatal("task missing after MarkFlag")
	}
	if got.Status != StatusSolved || got.Flag == nil || *got.Flag != "flag{ok}" {
		t.Fatalf("task not solved after MarkFlag: %+v", got)
	}
	if got.FinishedAt == nil || got.LastStep != "Flag已捕获" {
		t.Fatalf("completion metadata not set: finished=%v last=%q", got.FinishedAt, got.LastStep)
	}
}

func TestStoreSkipsMismatchedTaskIDOnLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	taskDir := filepath.Join(root, "task-dir")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	meta := []byte(`{"id":"other-id","name":"bad","category":"misc","description":"bad","status":"queued"}`)
	if err := os.WriteFile(filepath.Join(taskDir, "meta.json"), meta, 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if got := store.List(); len(got) != 0 {
		t.Fatalf("loaded mismatched task id: %+v", got)
	}
}

func TestStoreRecoverableIDs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	now := time.Now().UTC()
	for _, task := range []*Task{
		{ID: "solved", Name: "done", Category: "misc", Description: "done", Status: StatusSolved, CreatedAt: now},
		{ID: "queued", Name: "queued", Category: "misc", Description: "queued", Status: StatusQueued, CreatedAt: now.Add(time.Second)},
		{ID: "running", Name: "running", Category: "misc", Description: "running", Status: StatusRunning, CreatedAt: now.Add(2 * time.Second)},
		{ID: "retained-running", Name: "retained", Category: "misc", Description: "retained", Status: StatusRunning, ContainerName: "ctf-agent-retained-running", CreatedAt: now.Add(3 * time.Second)},
	} {
		if err := store.Add(task); err != nil {
			t.Fatalf("Add(%s): %v", task.ID, err)
		}
	}

	ids := store.RecoverableIDs()
	if len(ids) != 2 || ids[0] != "queued" || ids[1] != "running" {
		t.Fatalf("RecoverableIDs()=%v", ids)
	}
	if err := store.MarkRecovered("running"); err != nil {
		t.Fatalf("MarkRecovered: %v", err)
	}
	task, _ := store.Get("running")
	if task.Status != StatusQueued || task.StartedAt != nil {
		t.Fatalf("recovered task=%+v", task)
	}
}

func TestStoreMarksFlaggedTaskSolvedRegardlessOfExitCode(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	flag := "flag{done}"
	task := &Task{
		ID:          "flagged",
		Name:        "flagged",
		Category:    "misc",
		Description: "done",
		Status:      StatusRunning,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.MarkFinished(task.ID, 124, &flag, "timed out", "ctf-agent-flagged", true); err != nil {
		t.Fatalf("MarkFinished: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Status != StatusSolved || got.Error != nil {
		t.Fatalf("flagged task was not normalized as solved: %+v", got)
	}
}

func TestStoreHidesWriteupForUnsolvedTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:              "unsolved-writeup",
		Name:            "unsolved-writeup",
		Category:        "misc",
		Description:     "not solved",
		Status:          StatusFailed,
		WriteupFileName: "wp.md",
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, task.ID, "wp.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale wp: %v", err)
	}

	if _, _, ok := store.WriteupPath(task.ID); ok {
		t.Fatal("WriteupPath() returned stale writeup for unsolved task")
	}
}

func TestStoreRejectsUnsafeWriteupFilename(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:              "unsafe-writeup",
		Name:            "unsafe-writeup",
		Category:        "misc",
		Description:     "unsafe writeup",
		Status:          StatusSolved,
		WriteupFileName: "../outside.md",
		CreatedAt:       time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, _, ok := store.WriteupPath(task.ID); ok {
		t.Fatal("WriteupPath() returned unsafe writeup filename")
	}
	if err := store.SaveWriteup(task.ID, "..\\outside.md", "bad"); err == nil {
		t.Fatal("SaveWriteup() accepted unsafe filename")
	}
}

func TestAppendLogDoesNotSolveRunningTaskFromAssistantUpdate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "running-log-flag",
		Name:        "running-log-flag",
		Category:    "misc",
		Description: "running task with final assistant update",
		Status:      StatusRunning,
		LastStep:    "任务正在执行",
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	logs := `Observation: assistant output updated:
full decoded: flag{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}

## 解题过程总结

flag{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}`

	service := &Service{store: store, hub: NewHub()}
	service.AppendLog(task.ID, logs)

	got, _ := store.Get(task.ID)
	if got.Status != StatusRunning || got.Flag != nil || got.FinishedAt != nil {
		t.Fatalf("running task was solved from live log: %+v", got)
	}
	fullLogs, ok := store.Logs(task.ID)
	if !ok || !strings.Contains(fullLogs, "flag{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}") {
		t.Fatalf("live log was not appended:\n%s", fullLogs)
	}
}

func TestRunTaskExtractsFlagOnlyFromFinalReadableOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "midrun-false-flag",
		Name:        "midrun false flag",
		Category:    "misc",
		Description: "prints a fake flag before solving",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{store: store, hub: NewHub()}
	service.runDockerTask = func(_ context.Context, _ Config, task *Task, logSink LogSink, _ func(string)) (DockerResult, error) {
		logSink("Observation: assistant output updated:\nflag{wrong_midrun}\n")
		midRun, ok := store.Get(task.ID)
		if !ok {
			t.Fatalf("task missing during fake runner")
		}
		if midRun.Status != StatusRunning || midRun.Flag != nil || midRun.FinishedAt != nil {
			t.Fatalf("task solved before fake runner returned: %+v", midRun)
		}
		logSink("Observation: final readable OpenCode output:\n已验证最终结果。\n这道题目已经解出\nDASCTF{right_final}\n")
		return DockerResult{ExitCode: 0, Retained: true}, nil
	}

	service.runTask(0, task.ID)

	got, _ := store.Get(task.ID)
	if got.Status != StatusSolved || got.Flag == nil || *got.Flag != "DASCTF{right_final}" || got.ContainerKept {
		t.Fatalf("task should solve from final readable output: %+v", got)
	}
}

func TestManagedContainerNameValidation(t *testing.T) {
	t.Parallel()

	if !isManagedContainerName("ctf-agent-safe_task-1") {
		t.Fatal("expected managed container name")
	}
	for _, name := range []string{"database", "ctf-agent-../escape", "ctf-agent-", "ctf-agent-bad/name"} {
		if isManagedContainerName(name) {
			t.Fatalf("accepted unmanaged container name %q", name)
		}
	}
}

func TestStoreMarksRunningTaskFailedWhenContainerClosed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:            "running-close",
		Name:          "running-close",
		Category:      "misc",
		Description:   "running",
		Status:        StatusRunning,
		ContainerName: "ctf-agent-running-close",
		CreatedAt:     time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.MarkContainerClosed(task.ID); err != nil {
		t.Fatalf("MarkContainerClosed: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.ContainerName != "" || got.ContainerKept || got.Error == nil {
		t.Fatalf("closed running task was not marked failed: %+v", got)
	}
}

func TestStoreLoadsFlaggedFailedTaskAsSolved(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	flag := "flag{loaded}"
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	msg := "runner exited with status 124"
	task := &Task{
		ID:          "loaded",
		Name:        "loaded",
		Category:    "misc",
		Description: "done",
		Status:      StatusFailed,
		Flag:        &flag,
		Error:       &msg,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	loaded, err := NewStore(root)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, _ := loaded.Get(task.ID)
	if got.Status != StatusSolved || got.Error != nil {
		t.Fatalf("flagged failed task was not loaded as solved: %+v", got)
	}
}

func TestStorePersistsOpenCodeSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "session",
		Name:        "session",
		Category:    "misc",
		Description: "session",
		Status:      StatusRunning,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.MarkOpenCodeSession(task.ID, "ses_demo"); err != nil {
		t.Fatalf("MarkOpenCodeSession: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.OpenCodeSession != "ses_demo" {
		t.Fatalf("OpenCode session was not persisted: %+v", got)
	}
}

func TestStoreLoadsLegacyWebMetadata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	taskDir := filepath.Join(root, "legacy-web")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	meta := []byte(`{
  "id": "legacy-web",
  "name": "legacy-web",
  "category": "misc",
  "description": "legacy",
  "status": "failed",
  "container_name": "ctf-agent-legacy-web",
  "container_kept": true,
  "opencode_session": "ses_legacy",
  "opencode_web_url": "http://127.0.0.1:49152/L3dvcmtzcGFjZQ/session/ses_legacy",
  "opencode_host_port": "49152",
  "created_at": "2026-01-01T00:00:00Z"
}`)
	if err := os.WriteFile(filepath.Join(taskDir, "meta.json"), meta, 0o644); err != nil {
		t.Fatalf("write legacy meta: %v", err)
	}

	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	got, ok := store.Get("legacy-web")
	if !ok {
		t.Fatal("legacy task was not loaded")
	}
	if got.OpenCodeSession != "ses_legacy" || !got.ContainerKept {
		t.Fatalf("legacy task metadata not preserved: %+v", got)
	}
}

func TestOpenCodeStateReportsBridgeFailure(t *testing.T) {
	t.Parallel()

	message := "Final: opencode bridge failed: opencode server did not become ready"
	task := &Task{
		Status:   StatusFailed,
		Error:    stringPtr("runner exited with status 1"),
		LastStep: message,
	}

	status, gotMessage, available := openCodeState(task)
	if status != "error" || gotMessage != message || available {
		t.Fatalf("openCodeState()=(%q,%q,%v)", status, gotMessage, available)
	}
}

func TestCloseQueuedTaskStopsBeforeWorkerRuns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "queued-stop",
		Name:        "queued-stop",
		Category:    "misc",
		Description: "queued",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{store: store, hub: NewHub(), done: make(chan struct{}), queue: make(chan string, 1)}
	service.runDockerTask = func(context.Context, Config, *Task, LogSink, func(string)) (DockerResult, error) {
		t.Fatal("stopped queued task should not run")
		return DockerResult{}, nil
	}
	if err := service.CloseTaskContainer(task.ID); err != nil {
		t.Fatalf("CloseTaskContainer: %v", err)
	}
	service.runTask(0, task.ID)
	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.LastStep != taskStoppedMessage {
		t.Fatalf("queued task was not stopped: %+v", got)
	}
}

func TestRunTaskMarksTimeout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "timeout-task",
		Name:        "timeout-task",
		Category:    "misc",
		Description: "timeout",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{
		cfg:   Config{TaskTimeout: time.Nanosecond},
		store: store,
		hub:   NewHub(),
		done:  make(chan struct{}),
		runs:  map[string]context.CancelFunc{},
	}
	service.runDockerTask = func(ctx context.Context, _ Config, _ *Task, _ LogSink, _ func(string)) (DockerResult, error) {
		<-ctx.Done()
		return DockerResult{ExitCode: 124, Retained: true}, ctx.Err()
	}
	service.runTask(0, task.ID)
	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.Error == nil || *got.Error != taskTimeoutMessage {
		t.Fatalf("timeout task not marked failed: %+v", got)
	}
}

func TestRunTaskAutoContinuesUntilSolved(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "auto-continue",
		Name:        "auto-continue",
		Category:    "web",
		Description: "continue",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{
		cfg:   Config{AutoContinueRounds: 1},
		store: store,
		hub:   NewHub(),
		done:  make(chan struct{}),
		runs:  map[string]context.CancelFunc{},
	}
	var hints int
	service.runDockerTask = func(_ context.Context, _ Config, task *Task, logSink LogSink, containerSink func(string)) (DockerResult, error) {
		containerSink("ctf-agent-" + task.ID)
		logSink("Observation: OpenCode session=ses_auto\n")
		logSink("Observation: final readable OpenCode output:\n下一步继续测试。\n")
		return DockerResult{ExitCode: 0, ContainerName: "ctf-agent-" + task.ID, Retained: true}, nil
	}
	service.runDockerHint = func(_ context.Context, _ Config, task *Task, message string, logSink LogSink) (DockerResult, error) {
		hints++
		logSink("Observation: final readable OpenCode output:\n这道题目已经解出\nflag{auto_continue_ok}\n")
		return DockerResult{ExitCode: 0, ContainerName: task.ContainerName, Retained: true}, nil
	}
	service.runTask(0, task.ID)
	got, _ := store.Get(task.ID)
	if hints != 1 {
		t.Fatalf("auto-continue hints=%d want 1", hints)
	}
	if got.Status != StatusSolved || got.Flag == nil || *got.Flag != "flag{auto_continue_ok}" {
		t.Fatalf("task should solve after auto-continue: %+v", got)
	}
}

func TestRunTaskStopsAutoContinueOnHardFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "auto-exhaust",
		Name:        "auto-exhaust",
		Category:    "web",
		Description: "continue",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{
		cfg:   Config{AutoContinueRounds: 1},
		store: store,
		hub:   NewHub(),
		done:  make(chan struct{}),
		runs:  map[string]context.CancelFunc{},
	}
	service.runDockerTask = func(_ context.Context, _ Config, task *Task, logSink LogSink, containerSink func(string)) (DockerResult, error) {
		containerSink("ctf-agent-" + task.ID)
		logSink("Observation: OpenCode session=ses_exhaust\n")
		logSink("Observation: final readable OpenCode output:\n下一步继续测试。\n")
		return DockerResult{ExitCode: 0, ContainerName: "ctf-agent-" + task.ID, Retained: true}, nil
	}
	var hints int
	service.runDockerHint = func(_ context.Context, _ Config, task *Task, _ string, logSink LogSink) (DockerResult, error) {
		hints++
		logSink("Final: opencode bridge failed: OpenCode terminal finished without readable output\n")
		return DockerResult{ExitCode: 1, ContainerName: task.ContainerName, Retained: true}, nil
	}
	service.runTask(0, task.ID)
	got, _ := store.Get(task.ID)
	if hints != 1 {
		t.Fatalf("auto-continue hints=%d want 1", hints)
	}
	if got.Status != StatusFailed || got.Flag != nil || !got.ContainerKept {
		t.Fatalf("task should fail and keep container after hard failure: %+v", got)
	}
}

func TestRunTaskAutoContinueCanBeStoppedByUser(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	task := &Task{
		ID:          "auto-empty-output",
		Name:        "auto empty output",
		Category:    "web",
		Description: "continue",
		Status:      StatusQueued,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	service := &Service{
		cfg:   Config{AutoContinueRounds: 3},
		store: store,
		hub:   NewHub(),
		done:  make(chan struct{}),
		runs:  map[string]context.CancelFunc{},
	}
	var hints int
	service.runDockerTask = func(_ context.Context, _ Config, task *Task, logSink LogSink, containerSink func(string)) (DockerResult, error) {
		containerSink("ctf-agent-" + task.ID)
		logSink("Observation: OpenCode session=ses_empty\n")
		logSink("Observation: final readable OpenCode output:\n继续测试preview.php路径穿越。\n")
		return DockerResult{ExitCode: 0, ContainerName: "ctf-agent-" + task.ID, Retained: true}, nil
	}
	service.runDockerHint = func(_ context.Context, _ Config, task *Task, _ string, logSink LogSink) (DockerResult, error) {
		hints++
		if err := store.MarkStopped(task.ID, taskStoppedMessage); err != nil {
			t.Fatalf("MarkStopped: %v", err)
		}
		logSink("Observation: final readable OpenCode output:\n仍在继续。\n")
		return DockerResult{ExitCode: 0, ContainerName: task.ContainerName, Retained: true}, nil
	}

	service.runTask(0, task.ID)

	if hints != 1 {
		t.Fatalf("auto-continue hints=%d want 1", hints)
	}
	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.LastStep != taskStoppedMessage {
		t.Fatalf("task should remain stopped: %+v", got)
	}
}
