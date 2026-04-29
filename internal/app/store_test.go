package app

import (
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
	if err := store.MarkOpenCodeSession(task.ID, "ses_demo", "http://127.0.0.1:49152/L3dvcmtzcGFjZQ/session/ses_demo"); err != nil {
		t.Fatalf("MarkOpenCodeSession: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.OpenCodeSession != "ses_demo" || !strings.Contains(got.OpenCodeWebURL, "/session/ses_demo") {
		t.Fatalf("OpenCode session was not persisted: %+v", got)
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

func TestExtractFlagIgnoresAssistantUpdateWithoutFinalReadableOutput(t *testing.T) {
	t.Parallel()

	service := &Service{}
	task := &Task{}
	logs := `Observation: assistant output updated:
关键步骤：
1. 已还原载荷。
- ` + "`iscc{a91b0bbf-e6fd-42dd-b9a6-5ef4f2bc695f}`" + `
[runner] container timed out after 240s; retained for inspection`

	got := service.extractFlagForTask(task, logs)
	if got != nil {
		t.Fatalf("extractFlagForTask()=%v want nil", *got)
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
		OpenCodeWebURL:  "http://127.0.0.1:49152/L3dvcmtzcGFjZQ/session/ses_demo",
		ContainerKept:   true,
	}
	logs := `[dispatcher] queued workers=4
[runner] starting container image=ctf-agent-misc:latest
Thought: challenge='timeout-demo' category='misc'
[runner] container timed out after 240s; retained for inspection`

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

func TestWriteupNeedsRepairForSparseFailedWriteup(t *testing.T) {
	t.Parallel()

	task := &Task{Status: StatusFailed}
	content := `# timeout-demo

## 解题过程

Thought: challenge='timeout-demo' category='misc'
`

	if !writeupNeedsRepair(task, content) {
		t.Fatal("writeupNeedsRepair()=false want true")
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
