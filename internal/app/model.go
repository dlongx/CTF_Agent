package app

import "time"

type TaskStatus string

const (
	StatusQueued  TaskStatus = "queued"
	StatusRunning TaskStatus = "running"
	StatusSolved  TaskStatus = "solved"
	StatusFailed  TaskStatus = "failed"
)

type Task struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Category         string     `json:"category"`
	Description      string     `json:"description"`
	TargetIP         string     `json:"target_ip"`
	AttachmentsDir   string     `json:"attachments_dir"`
	AttachmentCount  int        `json:"attachment_count"`
	Status           TaskStatus `json:"status"`
	Flag             *string    `json:"flag"`
	ExitCode         *int       `json:"exit_code"`
	Error            *string    `json:"error"`
	LastStep         string     `json:"last_step"`
	WriteupFileName  string     `json:"writeup_file_name"`
	ContainerName    string     `json:"container_name"`
	ContainerKept    bool       `json:"container_kept"`
	OpenCodeWebURL   string     `json:"opencode_web_url"`
	OpenCodeHostPort string     `json:"opencode_host_port"`
	OpenCodeSession  string     `json:"opencode_session"`
	CreatedAt        time.Time  `json:"created_at"`
	StartedAt        *time.Time `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	LogSize          int        `json:"log_size"`
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
	OpenCodeWebURL    string     `json:"opencode_web_url"`
	OpenCodeHostPort  string     `json:"opencode_host_port"`
	OpenCodeSession   string     `json:"opencode_session"`
	OpenCodeAvailable bool       `json:"opencode_available"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at"`
	FinishedAt        *time.Time `json:"finished_at"`
	AttachmentCount   int        `json:"attachment_count"`
	LogSize           int        `json:"log_size"`
}

func newTaskResponse(task *Task) taskResponse {
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
		HasWriteup:        task.WriteupFileName != "",
		ContainerName:     task.ContainerName,
		ContainerKept:     task.ContainerKept,
		OpenCodeWebURL:    task.OpenCodeWebURL,
		OpenCodeHostPort:  task.OpenCodeHostPort,
		OpenCodeSession:   task.OpenCodeSession,
		OpenCodeAvailable: task.OpenCodeWebURL != "" && (task.Status == StatusRunning || task.ContainerKept),
		CreatedAt:         task.CreatedAt,
		StartedAt:         task.StartedAt,
		FinishedAt:        task.FinishedAt,
		AttachmentCount:   task.AttachmentCount,
		LogSize:           task.LogSize,
	}
}
