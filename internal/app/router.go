package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func NewRouter(service *Service) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware())
	webDir := findWebDir()
	router.Static("/static", filepath.Join(webDir, "static"))
	router.LoadHTMLGlob(filepath.Join(webDir, "templates", "*.html"))

	router.GET("/", service.indexPage)
	router.GET("/tasks/:id", service.taskPage)
	router.GET("/health", healthHandler)
	router.GET("/api/events", service.taskEvents)
	router.GET("/api/tasks", service.listTasks)
	router.POST("/api/tasks", service.createTask)
	router.OPTIONS("/api/tasks", optionsHandler)
	router.OPTIONS("/api/events", optionsHandler)
	router.GET("/api/tasks/:id", service.taskDetail)
	router.GET("/api/tasks/:id/logs", service.taskLogs)
	router.GET("/api/tasks/:id/opencode", service.taskOpenCode)
	router.GET("/api/tasks/:id/writeup", service.taskWriteup)
	router.POST("/api/tasks/:id/hints", service.taskHint)
	router.POST("/api/tasks/:id/container/close", service.taskCloseContainer)
	router.OPTIONS("/api/tasks/:id", optionsHandler)
	router.OPTIONS("/api/tasks/:id/logs", optionsHandler)
	router.OPTIONS("/api/tasks/:id/opencode", optionsHandler)
	router.OPTIONS("/api/tasks/:id/writeup", optionsHandler)
	router.OPTIONS("/api/tasks/:id/hints", optionsHandler)
	router.OPTIONS("/api/tasks/:id/container/close", optionsHandler)
	router.GET("/ws/tasks/:id/logs", func(c *gin.Context) {
		service.websocketHandler(c.Writer, c.Request)
	})
	return router
}

func (s *Service) taskEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sub := s.hub.SubscribeEvents()
	defer s.hub.UnsubscribeEvents(sub)

	_, _ = fmt.Fprint(c.Writer, "event: ready\ndata: {}\n\n")
	c.Writer.Flush()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-sub:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(c.Writer, "event: task\ndata: %s\n\n", event)
			c.Writer.Flush()
		}
	}
}

func findWebDir() string {
	for _, candidate := range []string{
		"web",
		filepath.Join("..", "web"),
		filepath.Join("..", "..", "web"),
		filepath.Join("..", "..", "..", "web"),
	} {
		if stat, err := os.Stat(filepath.Join(candidate, "templates")); err == nil && stat.IsDir() {
			return candidate
		}
	}
	return "web"
}

func healthHandler(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

func optionsHandler(c *gin.Context) {
	c.Status(204)
}

func (s *Service) indexPage(c *gin.Context) {
	c.HTML(200, "index.html", gin.H{
		"title": "CTF Agent",
	})
}

func (s *Service) taskPage(c *gin.Context) {
	id := c.Param("id")
	if _, ok := s.store.Get(id); !ok {
		c.HTML(404, "task.html", gin.H{
			"title":    "任务不存在",
			"taskID":   id,
			"notFound": true,
		})
		return
	}
	c.HTML(200, "task.html", gin.H{
		"title":  "任务详情",
		"taskID": id,
	})
}

func (s *Service) listTasks(c *gin.Context) {
	tasks := s.store.List()
	response := struct {
		Tasks []taskResponse `json:"tasks"`
	}{Tasks: make([]taskResponse, 0, len(tasks))}
	for _, task := range tasks {
		response.Tasks = append(response.Tasks, newTaskResponse(task))
	}
	c.JSON(200, response)
}

func (s *Service) createTask(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(256 << 20); err != nil {
		c.JSON(400, gin.H{"detail": "invalid multipart form"})
		return
	}
	name := strings.TrimSpace(c.Request.FormValue("name"))
	category := strings.TrimSpace(c.Request.FormValue("type"))
	description := strings.TrimSpace(c.Request.FormValue("description"))
	if name == "" || category == "" || description == "" {
		c.JSON(400, gin.H{"detail": "name, type and description are required"})
		return
	}
	taskID := s.NewTaskID()
	attachmentsDir, err := prepareTaskDirs(s.cfg.ChallengeDir, taskID)
	if err != nil {
		c.JSON(500, gin.H{"detail": err.Error()})
		return
	}
	files := c.Request.MultipartForm.File["attachments"]
	count, err := saveUploadedFiles(files, attachmentsDir)
	if err != nil {
		c.JSON(500, gin.H{"detail": err.Error()})
		return
	}
	task := &Task{
		ID:              taskID,
		Name:            name,
		Category:        category,
		Description:     description,
		TargetIP:        strings.TrimSpace(c.Request.FormValue("target_ip")),
		AttachmentsDir:  attachmentsDir,
		AttachmentCount: count,
		Status:          StatusQueued,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.Submit(task); err != nil {
		c.JSON(500, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(202, newTaskResponse(task))
}

func (s *Service) taskDetail(c *gin.Context) {
	task, ok := s.store.Get(c.Param("id"))
	if !ok {
		c.JSON(404, gin.H{"detail": "task not found"})
		return
	}
	c.JSON(200, newTaskResponse(task))
}

func (s *Service) taskLogs(c *gin.Context) {
	id := c.Param("id")
	logs, ok := s.store.Logs(id)
	if !ok {
		c.JSON(404, gin.H{"detail": "task not found"})
		return
	}
	if tail := c.Query("tail"); tail != "" {
		if limit, err := strconv.Atoi(tail); err == nil && limit > 0 && len(logs) > limit {
			logs = logs[len(logs)-limit:]
		}
	}
	c.JSON(200, gin.H{"task_id": id, "logs": logs})
}

func (s *Service) taskOpenCode(c *gin.Context) {
	task, ok := s.store.Get(c.Param("id"))
	if !ok {
		c.JSON(404, gin.H{"detail": "task not found"})
		return
	}
	if task.OpenCodeWebURL == "" || (task.Status != StatusRunning && !task.ContainerKept) {
		c.JSON(404, gin.H{"detail": "opencode web is not available for this task"})
		return
	}
	c.JSON(200, gin.H{
		"task_id":   task.ID,
		"url":       task.OpenCodeWebURL,
		"host_port": task.OpenCodeHostPort,
	})
}

func (s *Service) taskWriteup(c *gin.Context) {
	path, filename, ok := s.store.WriteupPath(c.Param("id"))
	if !ok {
		c.JSON(404, gin.H{"detail": "writeup not found"})
		return
	}
	c.FileAttachment(path, filename)
}

func (s *Service) taskHint(c *gin.Context) {
	var payload struct {
		Hint string `json:"hint"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"detail": "invalid hint payload"})
		return
	}
	if err := s.ContinueTask(c.Param("id"), payload.Hint); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"detail": "task not found"})
			return
		}
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	task, _ := s.store.Get(c.Param("id"))
	c.JSON(202, newTaskResponse(task))
}

func (s *Service) taskCloseContainer(c *gin.Context) {
	if err := s.CloseTaskContainer(c.Param("id")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"detail": "task not found"})
			return
		}
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	task, _ := s.store.Get(c.Param("id"))
	c.JSON(200, newTaskResponse(task))
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "*")
		if c.Request.Method == "OPTIONS" {
			c.Status(204)
			c.Abort()
			return
		}
		c.Next()
	}
}

func taskIDFromWebSocketPath(path string) (string, error) {
	rest := strings.TrimPrefix(path, "/ws/tasks/")
	id, suffix, ok := strings.Cut(rest, "/")
	if !ok || suffix != "logs" || id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || filepath.Base(id) != id {
		return "", errors.New("invalid task websocket path")
	}
	return id, nil
}
