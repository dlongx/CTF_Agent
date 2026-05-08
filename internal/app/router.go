package app

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
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
	router.Use(gin.Logger(), gin.Recovery(), corsMiddleware(service.cfg.AllowedOrigins))
	webDir := findWebDir()
	router.Static("/static", filepath.Join(webDir, "static"))
	router.LoadHTMLGlob(filepath.Join(webDir, "templates", "*.html"))

	router.GET("/health", healthHandler)
	router.Use(authMiddleware(service.cfg.AccessToken))
	router.GET("/", service.indexPage)
	router.GET("/containers", service.containersPage)
	router.GET("/tasks/:id", service.taskPage)
	router.GET("/api/events", service.taskEvents)
	router.GET("/api/containers", service.listContainers)
	router.GET("/api/settings/provider", service.providerSettings)
	router.POST("/api/settings/provider", service.updateProviderSettings)
	router.POST("/api/maintenance/clear-results", service.clearResults)
	router.GET("/api/tasks", service.listTasks)
	router.POST("/api/tasks", service.createTask)
	router.OPTIONS("/api/tasks", optionsHandler)
	router.OPTIONS("/api/events", optionsHandler)
	router.OPTIONS("/api/containers", optionsHandler)
	router.OPTIONS("/api/settings/provider", optionsHandler)
	router.OPTIONS("/api/maintenance/clear-results", optionsHandler)
	router.GET("/api/tasks/:id", service.taskDetail)
	router.GET("/api/tasks/:id/logs", service.taskLogs)
	router.GET("/api/tasks/:id/writeup", service.taskWriteup)
	router.POST("/api/tasks/:id/messages", service.taskMessage)
	router.POST("/api/tasks/:id/hints", service.taskHint)
	router.POST("/api/tasks/:id/stop", service.taskStop)
	router.POST("/api/tasks/:id/container/close", service.taskCloseContainer)
	router.OPTIONS("/api/tasks/:id", optionsHandler)
	router.OPTIONS("/api/tasks/:id/logs", optionsHandler)
	router.OPTIONS("/api/tasks/:id/writeup", optionsHandler)
	router.OPTIONS("/api/tasks/:id/messages", optionsHandler)
	router.OPTIONS("/api/tasks/:id/hints", optionsHandler)
	router.OPTIONS("/api/tasks/:id/stop", optionsHandler)
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

func (s *Service) containersPage(c *gin.Context) {
	c.HTML(200, "containers.html", gin.H{
		"title": "Docker管理",
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

func (s *Service) listContainers(c *gin.Context) {
	c.JSON(200, s.ListManagedContainers())
}

func (s *Service) providerSettings(c *gin.Context) {
	c.JSON(200, s.ProviderSettings())
}

func (s *Service) updateProviderSettings(c *gin.Context) {
	var payload struct {
		Format string `json:"format"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"detail": "invalid provider payload"})
		return
	}
	settings, err := s.SetProviderFormat(payload.Format)
	if err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(200, settings)
}

func (s *Service) clearResults(c *gin.Context) {
	tasksUpdated, filesRemoved, err := s.store.ClearResultData()
	if err != nil {
		c.JSON(500, gin.H{"detail": err.Error()})
		return
	}
	for _, task := range s.store.List() {
		s.publishTaskChanged(task.ID)
	}
	c.JSON(200, gin.H{
		"tasks_updated": tasksUpdated,
		"files_removed": filesRemoved,
	})
}

func (s *Service) createTask(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMultipartFormBytes)
	if err := c.Request.ParseMultipartForm(maxMultipartMemoryBytes); err != nil {
		status := 400
		detail := "invalid multipart form"
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
			detail = "multipart form exceeds 256MiB limit"
		}
		c.JSON(status, gin.H{"detail": detail})
		return
	}
	name := strings.TrimSpace(c.Request.FormValue("name"))
	category := strings.TrimSpace(c.Request.FormValue("type"))
	description := strings.TrimSpace(c.Request.FormValue("description"))
	if name == "" || category == "" || description == "" {
		c.JSON(400, gin.H{"detail": "name, type and description are required"})
		return
	}
	if _, err := s.activeOpenCodeProvider(); err != nil {
		c.JSON(400, gin.H{"detail": err.Error()})
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
		if errors.Is(err, errUploadTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"detail": err.Error()})
			return
		}
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
		if errors.Is(err, errTaskQueueFull) {
			c.JSON(http.StatusTooManyRequests, gin.H{"detail": err.Error()})
			return
		}
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
	if !s.handleTaskMessage(c, payload.Hint) {
		return
	}
	task, _ := s.store.Get(c.Param("id"))
	c.JSON(202, newTaskResponse(task))
}

func (s *Service) taskMessage(c *gin.Context) {
	var payload struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"detail": "invalid message payload"})
		return
	}
	if !s.handleTaskMessage(c, payload.Message) {
		return
	}
	task, _ := s.store.Get(c.Param("id"))
	c.JSON(202, newTaskResponse(task))
}

func (s *Service) handleTaskMessage(c *gin.Context, message string) bool {
	if err := s.SendTaskMessage(c.Param("id"), message); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"detail": "task not found"})
			return false
		}
		if errors.Is(err, errTaskMessageBusy) {
			c.JSON(http.StatusConflict, gin.H{"detail": err.Error()})
			return false
		}
		c.JSON(400, gin.H{"detail": err.Error()})
		return false
	}
	return true
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

func (s *Service) taskStop(c *gin.Context) {
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

func authMiddleware(token string) gin.HandlerFunc {
	token = strings.TrimSpace(token)
	return func(c *gin.Context) {
		if token == "" || c.Request.Method == http.MethodOptions || requestHasValidAccessToken(c.Request, token) {
			c.Next()
			return
		}
		c.Header("WWW-Authenticate", `Basic realm="CTF Agent"`)
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"detail": "authentication required"})
	}
}

func requestHasValidAccessToken(req *http.Request, token string) bool {
	if constantTimeStringEqual(req.Header.Get("X-CTF-Agent-Token"), token) {
		return true
	}
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") &&
		constantTimeStringEqual(strings.TrimSpace(auth[len("bearer "):]), token) {
		return true
	}
	_, password, ok := req.BasicAuth()
	return ok && constantTimeStringEqual(password, token)
}

func constantTimeStringEqual(got string, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if origin = strings.TrimSpace(origin); origin != "" && origin != "*" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
			if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-CTF-Agent-Token")
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
