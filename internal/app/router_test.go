package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(Config{
		ChallengeDir:       t.TempDir(),
		DockerImage:        "ctf-agent-opencode:latest",
		MaxContainers:      1,
		TaskTimeoutSeconds: 1,
		MemLimit:           "128m",
		CPUs:               "0.5",
		PidsLimit:          "64",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}
