package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	root  string
	mu    sync.RWMutex
	tasks map[string]*Task
}

const (
	containerClosedMessage = "容器已关闭"
	taskStoppedMessage     = "任务已停止"
	taskTimeoutMessage     = "任务执行超时"
)

func NewStore(root string) (*Store, error) {
	store := &Store{root: root, tasks: map[string]*Task{}}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if err := store.Load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Load() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = map[string]*Task{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		task, err := s.readTask(filepath.Join(s.root, entry.Name()), entry.Name())
		if err != nil {
			continue
		}
		s.tasks[task.ID] = task
	}
	return nil
}

func (s *Store) Add(task *Task) error {
	if task == nil {
		return errors.New("task is nil")
	}
	if !isSafeTaskID(task.ID) {
		return errors.New("invalid task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	return s.writeTask(task)
}

func (s *Store) Get(id string) (*Task, bool) {
	if !isSafeTaskID(id) {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	task, ok := s.tasks[id]
	if !ok {
		return nil, false
	}
	copy := *task
	return &copy, true
}

func (s *Store) List() []*Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		copy := *task
		tasks = append(tasks, &copy)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	return tasks
}

func (s *Store) AppendLog(id string, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	logPath := filepath.Join(s.root, id, "logs.txt")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteString(text); err != nil {
		return err
	}
	task.LogSize += len([]byte(text))
	return s.writeTask(task)
}

func (s *Store) Logs(id string) (string, bool) {
	if _, ok := s.Get(id); !ok {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(s.root, id, "logs.txt"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", true
		}
		return "", false
	}
	return string(data), true
}

func (s *Store) MarkRunning(id string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.Status = StatusRunning
		task.StartedAt = &now
		task.Error = nil
		task.LastStep = "任务正在执行"
	})
}

func (s *Store) MarkFinished(id string, exitCode int, flag *string, lastStep string, containerName string, containerKept bool) error {
	return s.MarkFinishedWithFailureMessage(id, exitCode, flag, lastStep, containerName, containerKept, "")
}

func (s *Store) MarkCompleted(id string, exitCode int, lastStep string, containerName string, containerKept bool) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.ExitCode = &exitCode
		task.Flag = nil
		task.Error = nil
		task.FinishedAt = &now
		task.LastStep = strings.TrimSpace(lastStep)
		if task.LastStep == "" {
			task.LastStep = "OpenCode本轮执行结束"
		}
		task.ContainerName = containerName
		task.ContainerKept = containerKept
		task.Status = StatusCompleted
	})
}

func (s *Store) MarkFinishedWithFailureMessage(id string, exitCode int, flag *string, lastStep string, containerName string, containerKept bool, failureMessage string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		failureMessage = strings.TrimSpace(failureMessage)
		task.ExitCode = &exitCode
		task.Flag = flag
		task.FinishedAt = &now
		task.LastStep = lastStep
		task.ContainerName = containerName
		task.ContainerKept = containerKept
		if flag != nil {
			task.Status = StatusSolved
			task.Error = nil
		} else {
			task.Status = StatusFailed
			if failureMessage != "" {
				task.Error = &failureMessage
			} else if task.Error == nil {
				msg := "runner exited with status " + strconvItoa(exitCode)
				task.Error = &msg
			}
		}
	})
}

func (s *Store) MarkFlag(id string, flag string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		flag = strings.TrimSpace(flag)
		task.Flag = &flag
		task.Status = StatusSolved
		task.Error = nil
		if task.FinishedAt == nil {
			task.FinishedAt = &now
		}
		if strings.TrimSpace(task.LastStep) == "" || task.LastStep == "任务正在执行" {
			task.LastStep = "Flag已捕获"
		}
		task.ContainerKept = false
	})
}

func (s *Store) MarkInvalidFlag(id string, message string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.Flag = nil
		task.Status = StatusFailed
		task.Error = &message
		task.LastStep = message
		task.FinishedAt = &now
		task.ContainerKept = false
	})
}

func (s *Store) MarkFalseLiveCapture(id string, message string) error {
	if !isSafeTaskID(id) {
		return errors.New("invalid task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	if task.WriteupFileName != "" && isSafeStoredFilename(task.WriteupFileName) {
		_ = os.Remove(filepath.Join(s.root, id, task.WriteupFileName))
	}
	now := time.Now().UTC()
	task.Flag = nil
	task.Status = StatusFailed
	task.ExitCode = nil
	task.Error = &message
	task.LastStep = message
	task.FinishedAt = &now
	task.ContainerKept = true
	task.WriteupFileName = ""
	return s.writeTask(task)
}

func (s *Store) MarkFailed(id string, message string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.Status = StatusFailed
		task.Error = &message
		task.LastStep = message
		task.FinishedAt = &now
	})
}

func (s *Store) MarkRuntimeContainer(id string, containerName string) error {
	return s.update(id, func(task *Task) {
		task.ContainerName = containerName
	})
}

func (s *Store) MarkOpenCodeSession(id string, sessionID string) error {
	return s.update(id, func(task *Task) {
		task.OpenCodeSession = strings.TrimSpace(sessionID)
	})
}

func (s *Store) SaveWriteup(id string, filename string, content string) error {
	if !isSafeStoredFilename(filename) {
		return errors.New("invalid writeup filename")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	if err := os.WriteFile(filepath.Join(s.root, id, filename), []byte(content), 0o644); err != nil {
		return err
	}
	task.WriteupFileName = filename
	return s.writeTask(task)
}

func (s *Store) ClearResultData() (int, int, error) {
	if s.root == "" {
		return 0, 0, errors.New("store root is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tasksUpdated := 0
	filesRemoved := 0
	for _, task := range s.tasks {
		changed := false
		if task.Flag != nil {
			task.Flag = nil
			changed = true
		}
		if task.WriteupFileName != "" {
			if isSafeStoredFilename(task.WriteupFileName) {
				if err := os.Remove(filepath.Join(s.root, task.ID, task.WriteupFileName)); err == nil {
					filesRemoved++
				} else if err != nil && !os.IsNotExist(err) {
					return tasksUpdated, filesRemoved, err
				}
			}
			task.WriteupFileName = ""
			changed = true
		}
		if task.Status == StatusSolved {
			task.Status = StatusCompleted
			task.Error = nil
			if strings.TrimSpace(task.LastStep) == "" || task.LastStep == "Flag已捕获" {
				task.LastStep = "历史Flag/WP结果已清理"
			}
			changed = true
		}
		if changed {
			tasksUpdated++
			if err := s.writeTask(task); err != nil {
				return tasksUpdated, filesRemoved, err
			}
		}
	}
	return tasksUpdated, filesRemoved, nil
}

func (s *Store) WriteupPath(id string) (string, string, bool) {
	task, ok := s.Get(id)
	if !ok || task.Status != StatusSolved || task.WriteupFileName == "" {
		return "", "", false
	}
	if !isSafeStoredFilename(task.WriteupFileName) {
		return "", "", false
	}
	path := filepath.Join(s.root, id, task.WriteupFileName)
	if _, err := os.Stat(path); err != nil {
		return "", "", false
	}
	return path, task.WriteupFileName, true
}

func (s *Store) MarkContainerClosed(id string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.ContainerKept = false
		task.ContainerName = ""
		task.OpenCodeSession = ""
		task.LastStep = containerClosedMessage
		if task.Status == StatusRunning || task.Status == StatusQueued {
			task.Status = StatusFailed
			task.Error = stringPtr(containerClosedMessage)
			task.FinishedAt = &now
		}
	})
}

func (s *Store) MarkStopped(id string, message string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		task.Status = StatusFailed
		task.Error = &message
		task.LastStep = message
		task.FinishedAt = &now
		task.ContainerKept = false
		task.ContainerName = ""
		task.OpenCodeSession = ""
	})
}

func (s *Store) RecoverableIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Status == StatusQueued || (task.Status == StatusRunning && task.ContainerName == "") {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func (s *Store) MarkRecovered(id string) error {
	return s.update(id, func(task *Task) {
		if task.Status == StatusRunning {
			task.Status = StatusQueued
			task.StartedAt = nil
			task.Error = nil
			task.ContainerKept = false
			task.ContainerName = ""
			task.OpenCodeSession = ""
		}
	})
}

func (s *Store) MarkInterruptedContainerRetained(id string) error {
	return s.update(id, func(task *Task) {
		now := time.Now().UTC()
		msg := "service restarted while task was running; container retained for manual inspection"
		task.Status = StatusFailed
		task.Error = &msg
		task.LastStep = msg
		task.FinishedAt = &now
		task.ContainerKept = true
	})
}

func (s *Store) update(id string, fn func(*Task)) error {
	if !isSafeTaskID(id) {
		return errors.New("invalid task id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		return errors.New("task not found")
	}
	fn(task)
	return s.writeTask(task)
}

func (s *Store) writeTask(task *Task) error {
	if task == nil {
		return errors.New("task is nil")
	}
	if !isSafeTaskID(task.ID) {
		return errors.New("invalid task id")
	}
	if err := os.MkdirAll(filepath.Join(s.root, task.ID), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.root, task.ID, "meta.json"), data, 0o644)
}

func (s *Store) readTask(dir string, dirID string) (*Task, error) {
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return nil, err
	}
	var task Task
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	if task.ID == "" {
		return nil, errors.New("missing task id")
	}
	if !isSafeTaskID(task.ID) || task.ID != dirID {
		return nil, errors.New("invalid task id")
	}
	if task.AttachmentsDir == "" {
		task.AttachmentsDir = filepath.Join(dir, "attachments")
	}
	if stat, err := os.Stat(filepath.Join(dir, "logs.txt")); err == nil {
		task.LogSize = int(stat.Size())
	}
	if task.Flag != nil && task.Status != StatusSolved {
		task.Status = StatusSolved
		task.Error = nil
		task.ContainerKept = false
	}
	return &task, nil
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := make([]byte, 0, 12)
	for value > 0 {
		buf = append(buf, byte('0'+value%10))
		value /= 10
	}
	if negative {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

func stringPtr(value string) *string {
	return &value
}

func isSafeTaskID(id string) bool {
	if id == "" || id != strings.TrimSpace(id) || id == "." || id == ".." {
		return false
	}
	if filepath.Base(id) != id || strings.ContainsAny(id, `/\`) {
		return false
	}
	for _, char := range id {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafeStoredFilename(name string) bool {
	if name == "" || name != strings.TrimSpace(name) || name == "." || name == ".." {
		return false
	}
	if filepath.IsAbs(name) || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return false
	}
	return true
}
