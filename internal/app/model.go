package app

import (
	"strings"
	"time"
)

type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusSolved    TaskStatus = "solved"
	StatusFailed    TaskStatus = "failed"
)

type Task struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Category        string     `json:"category"`
	Description     string     `json:"description"`
	TargetIP        string     `json:"target_ip"`
	AttachmentsDir  string     `json:"attachments_dir"`
	AttachmentCount int        `json:"attachment_count"`
	Status          TaskStatus `json:"status"`
	Flag            *string    `json:"flag"`
	ExitCode        *int       `json:"exit_code"`
	Error           *string    `json:"error"`
	LastStep        string     `json:"last_step"`
	WriteupFileName string     `json:"writeup_file_name"`
	ContainerName   string     `json:"container_name"`
	ContainerKept   bool       `json:"container_kept"`
	OpenCodeSession string     `json:"opencode_session"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	LogSize         int        `json:"log_size"`
}

type taskResponse struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Category          string     `json:"category"`
	Description       string     `json:"description"`
	TargetIP          string     `json:"target_ip"`
	Status            TaskStatus `json:"status"`
	Flag              *string    `json:"flag"`
	ExitCode          *int       `json:"exit_code"`
	Error             *string    `json:"error"`
	LastStep          string     `json:"last_step"`
	WriteupFileName   string     `json:"writeup_file_name"`
	HasWriteup        bool       `json:"has_writeup"`
	ContainerName     string     `json:"container_name"`
	ContainerKept     bool       `json:"container_kept"`
	OpenCodeSession   string     `json:"opencode_session"`
	OpenCodeAvailable bool       `json:"opencode_available"`
	OpenCodeStatus    string     `json:"opencode_status"`
	OpenCodeMessage   string     `json:"opencode_message,omitempty"`
	CanSendMessage    bool       `json:"can_send_message"`
	MessageStatus     string     `json:"terminal_message_status"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	AttachmentCount   int        `json:"attachment_count"`
	LogSize           int        `json:"log_size"`
}

type containerResponse struct {
	TaskID            string     `json:"task_id"`
	TaskName          string     `json:"task_name"`
	Category          string     `json:"category"`
	TaskStatus        TaskStatus `json:"task_status"`
	ContainerName     string     `json:"container_name"`
	ContainerState    string     `json:"container_state"`
	Image             string     `json:"image"`
	DockerStatus      string     `json:"docker_status"`
	DockerFound       bool       `json:"docker_found"`
	DockerRunning     bool       `json:"docker_running"`
	LastStep          string     `json:"last_step"`
	OpenCodeSession   string     `json:"opencode_session"`
	OpenCodeAvailable bool       `json:"opencode_available"`
	OpenCodeStatus    string     `json:"opencode_status"`
	OpenCodeMessage   string     `json:"opencode_message,omitempty"`
	CanSendMessage    bool       `json:"can_send_message"`
	MessageStatus     string     `json:"terminal_message_status"`
	HasWriteup        bool       `json:"has_writeup"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	LogSize           int        `json:"log_size"`
}

type containerListResponse struct {
	Containers      []containerResponse `json:"containers"`
	DockerAvailable bool                `json:"docker_available"`
	DockerError     string              `json:"docker_error,omitempty"`
	LiveCount       int                 `json:"live_count"`
	TrackedCount    int                 `json:"tracked_count"`
}

type providerSettingsResponse struct {
	ActiveFormat string                   `json:"active_format"`
	Options      []providerOptionResponse `json:"options"`
}

type providerOptionResponse struct {
	Format            string `json:"format"`
	Label             string `json:"label"`
	ProviderID        string `json:"provider_id"`
	ProviderName      string `json:"provider_name"`
	ProviderNPM       string `json:"provider_npm"`
	Model             string `json:"model"`
	BaseURLConfigured bool   `json:"base_url_configured"`
	APIKeyConfigured  bool   `json:"api_key_configured"`
	Configured        bool   `json:"configured"`
	Active            bool   `json:"active"`
}

func newTaskResponse(task *Task) taskResponse {
	openCodeStatus, openCodeMessage, openCodeAvailable := openCodeState(task)
	canSendMessage, messageStatus := terminalMessageState(task)
	hasWriteup := task != nil && task.Status == StatusSolved && task.WriteupFileName != ""
	return taskResponse{
		ID:                task.ID,
		Name:              task.Name,
		Category:          task.Category,
		Description:       task.Description,
		TargetIP:          task.TargetIP,
		Status:            task.Status,
		Flag:              task.Flag,
		ExitCode:          task.ExitCode,
		Error:             task.Error,
		LastStep:          task.LastStep,
		WriteupFileName:   task.WriteupFileName,
		HasWriteup:        hasWriteup,
		ContainerName:     task.ContainerName,
		ContainerKept:     task.ContainerKept,
		OpenCodeSession:   task.OpenCodeSession,
		OpenCodeAvailable: openCodeAvailable,
		OpenCodeStatus:    openCodeStatus,
		OpenCodeMessage:   openCodeMessage,
		CanSendMessage:    canSendMessage,
		MessageStatus:     messageStatus,
		CreatedAt:         task.CreatedAt,
		StartedAt:         task.StartedAt,
		FinishedAt:        task.FinishedAt,
		AttachmentCount:   task.AttachmentCount,
		LogSize:           task.LogSize,
	}
}

func openCodeState(task *Task) (string, string, bool) {
	if task == nil {
		return "unavailable", "任务不存在", false
	}
	if message := openCodeErrorMessage(task); message != "" {
		return "error", message, false
	}
	if task.Status == StatusRunning && task.ContainerName != "" {
		if task.OpenCodeSession != "" {
			return "ready", "OpenCode终端会话运行中", true
		}
		return "starting", "OpenCode终端正在启动", false
	}
	if task.OpenCodeSession != "" {
		return "ready", "OpenCode终端会话已记录", true
	}
	return "unavailable", "OpenCode服务不可用", false
}

func terminalMessageState(task *Task) (bool, string) {
	if task == nil {
		return false, "任务不存在"
	}
	switch task.Status {
	case StatusQueued, StatusRunning:
		return false, "当前回合运行中，结束后可继续发送"
	}
	if !task.ContainerKept || task.ContainerName == "" {
		return false, "容器未保留，不能继续发送"
	}
	if strings.TrimSpace(task.OpenCodeSession) == "" {
		return false, "缺少OpenCode终端会话，不能继续发送"
	}
	return true, "可以继续向OpenCode发送消息"
}

func openCodeErrorMessage(task *Task) string {
	for _, value := range []string{stringPtrValue(task.Error), task.LastStep} {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if strings.Contains(normalized, "opencode") ||
			strings.Contains(normalized, "open code") ||
			strings.Contains(normalized, "model is not configured") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
