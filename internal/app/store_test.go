package app

import (
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

func TestServiceRepairsInvalidSolvedFlag(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	flag := "+"
	task := &Task{
		ID:          "invalid-flag",
		Name:        "invalid-flag",
		Category:    "misc",
		Description: "invalid flag",
		Status:      StatusSolved,
		Flag:        &flag,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}

	service := &Service{store: store}
	service.repairInvalidSolvedFlags()

	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.Flag != nil || got.Error == nil {
		t.Fatalf("invalid solved flag was not repaired: %+v", got)
	}
}

func TestServiceRepairsPromptEchoSolvedFlag(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	flag := "time.sleep(required_seconds)"
	task := &Task{
		ID:          "prompt-echo-flag",
		Name:        "prompt-echo-flag",
		Category:    "misc",
		Description: "prompt echo flag",
		Status:      StatusSolved,
		Flag:        &flag,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	logs := `Observation: final readable OpenCode output:
license: MIT
compatibility: Requires filesystem-based agent
# CTF Miscellaneous
Quick reference for miscellaneous CTF challenges.
- **Time-only validation:** Start session, ` + "`time.sleep(required_seconds)`" + `, submit win.`
	if err := store.AppendLog(task.ID, logs); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}

	service := &Service{store: store}
	service.repairInvalidSolvedFlags()

	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.Flag != nil || got.Error == nil {
		t.Fatalf("prompt echo solved flag was not repaired: %+v", got)
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

func TestRunTaskOnlySolvesAfterRunnerReturnsFinalMarker(t *testing.T) {
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
	service.runDockerTask = func(_ Config, task *Task, logSink LogSink, _ func(string)) (DockerResult, error) {
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
	if got.Status != StatusSolved || got.Flag == nil || *got.Flag != "DASCTF{right_final}" {
		t.Fatalf("task did not solve from final marker: %+v", got)
	}
}

func TestServiceRepairsFalseLiveCapturedSolvedTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	flag := "flag{wrong_live}"
	now := time.Now().UTC()
	task := &Task{
		ID:              "false-live",
		Name:            "false live",
		Category:        "misc",
		Description:     "mis-captured while running",
		Status:          StatusSolved,
		Flag:            &flag,
		ContainerName:   "ctf-agent-false-live",
		ContainerKept:   false,
		OpenCodeSession: "ses_false_live",
		CreatedAt:       now,
		FinishedAt:      &now,
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := store.SaveWriteup(task.ID, "false_live_wp.md", "# wrong\n"); err != nil {
		t.Fatalf("SaveWriteup: %v", err)
	}
	if err := store.AppendLog(task.ID, liveFlagCaptureMarker+"\nflag{wrong_live}\n"); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}

	service := &Service{store: store, hub: NewHub()}
	service.repairFalseLiveCapturedFlagsFromContainers(map[string]DockerContainer{
		"ctf-agent-false-live": {Name: "ctf-agent-false-live", Running: true},
	})

	got, _ := store.Get(task.ID)
	if got.Status != StatusFailed || got.Flag != nil || !got.ContainerKept {
		t.Fatalf("false live capture was not repaired: %+v", got)
	}
	if got.ContainerName != "ctf-agent-false-live" || got.OpenCodeSession != "ses_false_live" {
		t.Fatalf("runtime continuation metadata was not preserved: %+v", got)
	}
	if got.WriteupFileName != "" {
		t.Fatalf("stale writeup filename kept: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(root, task.ID, "false_live_wp.md")); !os.IsNotExist(err) {
		t.Fatalf("stale writeup file still exists or stat failed unexpectedly: %v", err)
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

func TestExtractFlagPrefersFinalReadableLastLine(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
这里是中文解题过程。
custom-final-token`

	got := service.extractFlagForTask(task, logs)
	if got == nil || *got != "custom-final-token" {
		t.Fatalf("extractFlagForTask()=%v want custom-final-token", got)
	}
}

func TestExtractFlagRejectsOpenCodeStatusToken(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
Performing one time database migration, may take a few minutes...

sqlite-migration:done

Database migration complete.

Not supported model MiMo-V2.5-Pro`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractFlagFromChineseSolvedMarker(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
已完成验算。
这道题目已经解出
DASCTF{abc}`

	got := service.extractFlagForTask(task, logs)
	if got == nil || *got != "DASCTF{abc}" {
		t.Fatalf("extractFlagForTask()=%v want DASCTF{abc}", got)
	}
}

func TestExtractFlagSkipsBlankLineAfterChineseMarker(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
这道题目已经解出


ISCC{skip_blank_ok}`

	got := service.extractFlagForTask(task, logs)
	if got == nil || *got != "ISCC{skip_blank_ok}" {
		t.Fatalf("extractFlagForTask()=%v want ISCC{skip_blank_ok}", got)
	}
}

func TestExtractFlagIgnoresPromptEchoChineseMarker(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
Prompt要求OpenCode最终严格输出：
这道题目已经解出
DASCTF{prompt_echo}`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractFlagIgnoresSkillDocChineseMarker(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
Active CTF skills:
--- skill: ctf-misc ---
这道题目已经解出
DASCTF{skill_doc}
Attachments are mounted read-only at /attachments`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractFlagIgnoresAssistantUpdateWithoutFinalReadableOutput(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: assistant output updated:
关键步骤：
1. 已还原载荷。
- ` + "`iscc{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}`" + `
[runner] failed container retained for hints container_name=ctf-agent-demo`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractFlagIgnoresAssistantUpdateStandaloneFinalLine(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: assistant output updated:
成功解出flag。

flag{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}
[runner] agent exited with status=3221225786
[runner] failed container retained for hints container_name=ctf-agent-demo`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractOpenCodeExportTextIgnoresUserPromptAndReadsToolOutput(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
  "messages": [
    {
      "info": {"role": "user"},
      "parts": [{"type": "text", "text": "example flag{wrong} in prompt"}]
    },
    {
      "info": {"role": "assistant"},
      "parts": [
        {"type": "text", "text": "正在解析载荷"},
        {"type": "tool", "state": {"output": "payload: b'flag{right-answer}'"}}
      ]
    }
  ]
}`)

	text := extractOpenCodeExportText(raw)
	if strings.Contains(text, "flag{wrong}") {
		t.Fatalf("extractOpenCodeExportText() included user prompt: %q", text)
	}
	if flag := extractFlagToken(text); flag != "flag{right-answer}" {
		t.Fatalf("extractFlagToken()=%q want flag{right-answer}; text=%q", flag, text)
	}
}

func TestExtractFlagRejectsSingleCharacterPromptResidue(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
Active CTF skills:
Repunit decomposition uses only 1 and +.
+`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestExtractFlagRejectsPromptEchoInlineCode(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: final readable OpenCode output:
license: MIT
compatibility: Requires filesystem-based agent
# CTF Miscellaneous
Quick reference for miscellaneous CTF challenges.
- **Time-only validation:** Start session, ` + "`time.sleep(required_seconds)`" + `, submit win.
Final: opencode bridge completed`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
	}
}

func TestNormalizeFinalFlagLineDoesNotExtractArbitraryInlineCode(t *testing.T) {
	t.Parallel()

	if got := normalizeFinalFlagLine("- **Time-only validation:** Start session, `time.sleep(required_seconds)`, submit win."); got != "" {
		t.Fatalf("normalizeFinalFlagLine()=%q want empty", got)
	}
	if got := normalizeFinalFlagLine("Final Flag: `flag{demo}`"); got != "flag{demo}" {
		t.Fatalf("normalizeFinalFlagLine()=%q want flag{demo}", got)
	}
	if got := normalizeFinalFlagLine("`custom-final-token`"); got != "custom-final-token" {
		t.Fatalf("normalizeFinalFlagLine()=%q want custom-final-token", got)
	}
	if got := normalizeFinalFlagLine("- `flag{demo}`"); got != "" {
		t.Fatalf("normalizeFinalFlagLine()=%q want empty", got)
	}
	if got := normalizeFinalFlagLine("- Final Flag: `flag{demo}`"); got != "flag{demo}" {
		t.Fatalf("normalizeFinalFlagLine()=%q want flag{demo}", got)
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

func TestBuildWriteupFiltersPlatformLogs(t *testing.T) {
	t.Parallel()

	flag := "flag{demo}"
	task := &Task{
		Name:        "demo",
		Category:    "crypto",
		Description: "solve it",
		Status:      StatusSolved,
		Flag:        &flag,
	}
	logs := `[dispatcher] queued workers=4
[runner] starting container image=ctf-agent-opencode:latest
[opencode] INFO service=bus type=message.part.delta publishing
Observation: final readable OpenCode output:
Active CTF skills:
--- skill: crypto ---
lots of skill docs
[skill truncated]

Attachments are mounted read-only at /attachments.
<path>/attachments/a.py</path>
check_calc 123456
pt flag{demo}

Flag: flag{demo}
Final: opencode bridge completed
[runner] removing solved container_name=ctf-agent-demo`

	writeup := buildWriteup(task, logs)
	for _, unwanted := range []string{
		"[dispatcher]",
		"[runner]",
		"[opencode]",
		"Active CTF skills",
		"lots of skill docs",
		"[skill truncated]",
	} {
		if strings.Contains(writeup, unwanted) {
			t.Fatalf("writeup contains unwanted %q:\n%s", unwanted, writeup)
		}
	}
	if !strings.Contains(writeup, "Flag: flag{demo}") {
		t.Fatalf("writeup missing flag:\n%s", writeup)
	}
	if !strings.Contains(writeup, "pt flag{demo}") {
		t.Fatalf("writeup missing solving output:\n%s", writeup)
	}
}

func TestBuildWriteupFormatsTaggedCodeBlocks(t *testing.T) {
	t.Parallel()

	task := &Task{
		Name:        "code-demo",
		Category:    "crypto",
		Description: "solve it",
		Status:      StatusSolved,
	}
	logs := `Observation: final readable OpenCode output:
<path>/attachments/generator.py</path>
<type>file</type>
<content>
1: import random
2:
3: def main():
4:     print("flag{demo}")

(End of file - total 4 lines)
</content>

运行脚本后得到Flag。`

	writeup := buildWriteup(task, logs)
	if !strings.Contains(writeup, "### 文件`/attachments/generator.py`") {
		t.Fatalf("writeup missing file heading:\n%s", writeup)
	}
	if !strings.Contains(writeup, "```python\nimport random") {
		t.Fatalf("writeup missing python code fence:\n%s", writeup)
	}
	if strings.Contains(writeup, "1: import random") || strings.Contains(writeup, "4:     print") {
		t.Fatalf("writeup kept numbered prefixes:\n%s", writeup)
	}
	if strings.Contains(writeup, "<content>") || strings.Contains(writeup, "</content>") {
		t.Fatalf("writeup kept content tags:\n%s", writeup)
	}
}

func TestBuildWriteupFallsBackWhenFinalOutputMissing(t *testing.T) {
	t.Parallel()

	lastStep := "Thought: challenge='timeout-demo' category='misc'"
	task := &Task{
		Name:            "timeout-demo",
		Category:        "misc",
		Description:     "solve it",
		Status:          StatusFailed,
		LastStep:        lastStep,
		OpenCodeSession: "ses_demo",
		ContainerKept:   true,
	}
	logs := `[dispatcher] queued workers=4
[runner] starting container image=ctf-agent-misc:latest
Thought: challenge='timeout-demo' category='misc'
[runner] failed container retained for hints container_name=ctf-agent-timeout-demo`

	writeup := buildWriteup(task, logs)
	if !strings.Contains(writeup, "平台尚未捕获到OpenCode最终可读输出") {
		t.Fatalf("writeup missing fallback text:\n%s", writeup)
	}
	if !strings.Contains(writeup, "容器仍保留") {
		t.Fatalf("writeup missing retained-container hint:\n%s", writeup)
	}
	if !strings.Contains(writeup, lastStep) {
		t.Fatalf("writeup missing last step:\n%s", writeup)
	}
}

func TestExtractGeneratedWriteupUsesLastValidBlock(t *testing.T) {
	t.Parallel()

	logs := generatedWriteupBeginMarker + `
too short
` + generatedWriteupEndMarker + `
Observation: final readable OpenCode output:
running logs
` + generatedWriteupBeginMarker + `
# demo

## 解题过程

通过脚本还原编码链路，验证输出后得到Flag。

## Flag

flag{latest}
` + generatedWriteupEndMarker

	got, ok := extractGeneratedWriteup(logs)
	if !ok {
		t.Fatal("extractGeneratedWriteup() ok=false want true")
	}
	if !strings.Contains(got, "flag{latest}") || strings.Contains(got, "too short") {
		t.Fatalf("extractGeneratedWriteup() picked wrong block:\n%s", got)
	}
}

func TestBuildStoredWriteupFallsBackWhenGeneratedBlockInvalid(t *testing.T) {
	t.Parallel()

	flag := "flag{fallback}"
	task := &Task{
		Name:        "fallback-demo",
		Category:    "misc",
		Description: "solve it",
		Status:      StatusSolved,
		Flag:        &flag,
	}
	logs := generatedWriteupBeginMarker + `
short
` + generatedWriteupEndMarker + `
Observation: final readable OpenCode output:
已确认最终结果。
Flag: flag{fallback}`

	writeup := buildStoredWriteup(task, logs)
	if !strings.Contains(writeup, "# fallback-demo") || !strings.Contains(writeup, "Flag:flag{fallback}") {
		t.Fatalf("buildStoredWriteup() did not fall back to platform writeup:\n%s", writeup)
	}
	if strings.Contains(writeup, generatedWriteupBeginMarker) || strings.Contains(writeup, "short") {
		t.Fatalf("buildStoredWriteup() leaked invalid generated block:\n%s", writeup)
	}
}

func TestSafeWriteupFilenameAddsSuffixAndSanitizes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "ez misc", want: "ez misc_wp.md"},
		{name: "unsafe", in: `bad/name:demo`, want: "bad_name_demo_wp.md"},
		{name: "already suffixed", in: "demo_wp", want: "demo_wp.md"},
		{name: "empty", in: " \t", want: "writeup_wp.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeWriteupFilename(tt.in); got != tt.want {
				t.Fatalf("safeWriteupFilename(%q)=%q want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestServiceSaveWriteupUsesGeneratedWriteupAndSuffix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	flag := "DASCTF{wp_ok}"
	task := &Task{
		ID:          "generated-writeup",
		Name:        `题目/名称`,
		Category:    "web",
		Description: "solve it",
		Status:      StatusSolved,
		Flag:        &flag,
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.Add(task); err != nil {
		t.Fatalf("Add: %v", err)
	}
	logs := `Observation: OpenCode writeup file: 题目_名称_wp.md
` + generatedWriteupBeginMarker + `
# 题目/名称

## 解题思路

利用路径遍历读取配置，再伪造签名拿到Flag。

## Flag

DASCTF{wp_ok}
` + generatedWriteupEndMarker + `
Observation: final readable OpenCode output:
这道题目已经解出
DASCTF{wp_ok}`
	if err := store.AppendLog(task.ID, logs); err != nil {
		t.Fatalf("AppendLog: %v", err)
	}

	service := &Service{store: store, hub: NewHub()}
	service.saveWriteup(task.ID)

	path, filename, ok := store.WriteupPath(task.ID)
	if !ok {
		t.Fatal("WriteupPath() ok=false want true")
	}
	if filename != "题目_名称_wp.md" {
		t.Fatalf("writeup filename=%q want 题目_名称_wp.md", filename)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "利用路径遍历读取配置") || strings.Contains(got, "Observation: final readable") {
		t.Fatalf("saved writeup did not use generated markdown:\n%s", got)
	}
}

func TestWriteupNeedsRepairIgnoresUnsolvedWriteup(t *testing.T) {
	t.Parallel()

	task := &Task{Status: StatusFailed}
	content := `# timeout-demo

## 解题过程

Thought: challenge='timeout-demo' category='misc'
`

	if writeupNeedsRepair(task, content) {
		t.Fatal("writeupNeedsRepair()=true want false for unsolved task")
	}
}

func TestWriteupNeedsRepairKeepsSolvedWriteup(t *testing.T) {
	t.Parallel()

	task := &Task{Status: StatusSolved}
	content := `# solved-demo

## 解题过程

Flag: flag{done}
`

	if writeupNeedsRepair(task, content) {
		t.Fatal("writeupNeedsRepair()=true want false")
	}
}
