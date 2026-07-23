package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var openCodeSessionPattern = regexp.MustCompile(`Observation:\s+OpenCode session=([A-Za-z0-9_-]+)`)
var errTaskQueueFull = errors.New("任务队列已满，请稍后再提交")
var errContinueUnavailable = errors.New("OpenCode未解出且当前容器或session不可继续")

const (
	bridgeRunTimeoutExitCode  = 124
	bridgeIdleTimeoutExitCode = 125
)

type taskEvent struct {
	Type   string `json:"type"`
	TaskID string `json:"task_id,omitempty"`
}

type providerSelectionState struct {
	Format string `json:"format"`
}

var errTaskMessageBusy = errors.New("当前回合运行中，结束后可继续发送")

type dockerTaskRunnerContext func(context.Context, Config, *Task, LogSink, func(string)) (DockerResult, error)
type dockerHintRunnerContext func(context.Context, Config, *Task, string, LogSink) (DockerResult, error)

type Service struct {
	cfg            Config
	store          *Store
	hub            *Hub
	queue          chan string
	done           chan struct{}
	wg             sync.WaitGroup
	once           sync.Once
	runMu          sync.Mutex
	runs           map[string]context.CancelFunc
	busy           map[string]bool
	queuedMessages map[string]string

	providerMu           sync.RWMutex
	activeProviderFormat string
	maintenanceMu        sync.RWMutex
	lastCleanupAt        *time.Time
	lastCleanupRemoved   int

	runDockerTask dockerTaskRunnerContext
	runDockerHint dockerHintRunnerContext
}

func NewService(cfg Config) (*Service, error) {
	cfg = cfg.withOpenCodeProviderDefaults()
	if err := os.MkdirAll(cfg.ChallengeDir, 0o755); err != nil {
		return nil, err
	}
	store, err := NewStoreWithOptions(cfg.ChallengeDir, cfg.LogMaxBytes, defaultLogMaxBackups)
	if err != nil {
		return nil, err
	}
	service := &Service{
		cfg:            cfg,
		store:          store,
		hub:            NewHub(),
		queue:          make(chan string, max(1, cfg.MaxContainers)*4),
		done:           make(chan struct{}),
		runs:           map[string]context.CancelFunc{},
		busy:           map[string]bool{},
		queuedMessages: map[string]string{},

		runDockerTask: RunDockerTask,
		runDockerHint: RunDockerHint,
	}
	service.activeProviderFormat = service.loadProviderFormat()
	service.recoverInterruptedRunningContainers()
	for i := 0; i < max(1, cfg.MaxContainers); i++ {
		service.wg.Add(1)
		go service.worker(i)
	}
	service.wg.Add(1)
	go service.retentionCleaner()
	for _, id := range store.RecoverableIDs() {
		if err := store.MarkRecovered(id); err != nil {
			slog.Error("recover queued task", "task_id", id, "error", err)
			continue
		}
		service.AppendLog(id, "[dispatcher] recovered queued task after service startup\n")
		service.enqueue(id)
	}
	return service, nil
}

func (s *Service) Close() {
	s.once.Do(func() {
		s.cancelAllRuns()
		close(s.done)
		waited := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(waited)
		}()
		select {
		case <-waited:
			return
		case <-time.After(10 * time.Second):
			slog.Warn("service shutdown exceeded grace period; forcing managed container cleanup", "grace", "10s")
		}
		for _, task := range s.store.List() {
			if task.Status == StatusRunning && task.ContainerName != "" {
				_ = CloseTaskContainer(task.ContainerName)
			}
		}
	})
}

func (s *Service) NewTaskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconv.Itoa(int(time.Now().UnixNano()))
	}
	return hex.EncodeToString(buf[:])
}

func (s *Service) Submit(task *Task) error {
	if _, err := s.activeOpenCodeProvider(); err != nil {
		return err
	}
	if s.queueIsFull() {
		return errTaskQueueFull
	}
	if err := s.store.Add(task); err != nil {
		return err
	}
	s.publishTaskChanged(task.ID)
	if !s.enqueue(task.ID) {
		_ = s.store.MarkFailed(task.ID, errTaskQueueFull.Error())
		s.publishTaskChanged(task.ID)
		return errTaskQueueFull
	}
	s.AppendLog(task.ID, "[dispatcher] queued workers="+strconv.Itoa(max(1, s.cfg.MaxContainers))+"\n")
	return nil
}

func (s *Service) ProviderSettings() providerSettingsResponse {
	activeFormat := s.ActiveProviderFormat()
	options := make([]providerOptionResponse, 0, 2)
	for _, format := range []string{ProviderFormatOpenAICompatible, ProviderFormatAnthropic} {
		provider, ok := s.cfg.ProviderForFormat(format)
		if !ok {
			continue
		}
		options = append(options, providerOptionResponse{
			Format:            provider.Format,
			Label:             provider.Label,
			ProviderID:        provider.ProviderID,
			ProviderName:      provider.ProviderName,
			ProviderNPM:       provider.ProviderNPM,
			Model:             provider.Model,
			BaseURLConfigured: strings.TrimSpace(provider.BaseURL) != "",
			APIKeyConfigured:  strings.TrimSpace(provider.APIKey) != "",
			Configured:        provider.IsConfigured(),
			Active:            provider.Format == activeFormat,
		})
	}
	return providerSettingsResponse{ActiveFormat: activeFormat, Options: options}
}

func (s *Service) ActiveProviderFormat() string {
	s.providerMu.RLock()
	defer s.providerMu.RUnlock()
	return s.activeProviderFormat
}

func (s *Service) SetProviderFormat(format string) (providerSettingsResponse, error) {
	provider, ok := s.cfg.ProviderForFormat(format)
	if !ok {
		return providerSettingsResponse{}, errors.New("unsupported provider format")
	}
	if !provider.IsConfigured() {
		return providerSettingsResponse{}, errors.New("provider is not fully configured")
	}
	if err := s.saveProviderFormat(provider.Format); err != nil {
		return providerSettingsResponse{}, err
	}
	s.providerMu.Lock()
	s.activeProviderFormat = provider.Format
	s.providerMu.Unlock()
	return s.ProviderSettings(), nil
}

func (s *Service) activeOpenCodeProvider() (OpenCodeProviderConfig, error) {
	s.providerMu.RLock()
	format := s.activeProviderFormat
	s.providerMu.RUnlock()
	provider, ok := s.cfg.ProviderForFormat(format)
	if !ok {
		return OpenCodeProviderConfig{}, errors.New("unsupported provider format")
	}
	if !provider.IsConfigured() {
		return OpenCodeProviderConfig{}, errors.New("active provider is not fully configured")
	}
	return provider, nil
}

func (s *Service) activeDockerConfig() Config {
	provider, err := s.activeOpenCodeProvider()
	if err != nil {
		return s.cfg
	}
	return s.cfg.WithOpenCodeProvider(provider)
}

func (s *Service) loadProviderFormat() string {
	if value := strings.TrimSpace(os.Getenv("OPENCODE_PROVIDER_FORMAT")); value != "" {
		format := normalizeProviderFormat(value)
		if provider, ok := s.cfg.ProviderForFormat(format); ok && provider.IsConfigured() {
			return provider.Format
		}
	}
	path := s.providerStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return s.cfg.OpenCodeProviderFormat
	}
	var state providerSelectionState
	if err := json.Unmarshal(data, &state); err != nil {
		return s.cfg.OpenCodeProviderFormat
	}
	provider, ok := s.cfg.ProviderForFormat(state.Format)
	if !ok || !provider.IsConfigured() {
		return s.cfg.OpenCodeProviderFormat
	}
	return provider.Format
}

func (s *Service) saveProviderFormat(format string) error {
	if err := os.MkdirAll(s.cfg.DataDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(providerSelectionState{Format: format}, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.providerStatePath(), data, 0o644)
}

func (s *Service) providerStatePath() string {
	return filepath.Join(s.cfg.DataDir, "provider.json")
}

func (s *Service) ListManagedContainers() containerListResponse {
	tasks := s.store.List()
	dockerContainers, err := ListDockerContainers()
	dockerAvailable := err == nil
	if dockerContainers == nil {
		dockerContainers = map[string]DockerContainer{}
	}
	response := containerListResponse{
		Containers:      buildManagedContainerResponses(tasks, dockerContainers, dockerAvailable, s.cfg),
		DockerAvailable: dockerAvailable,
		LiveCount:       len(dockerContainers),
	}
	if err != nil {
		response.DockerError = err.Error()
	}
	response.TrackedCount = len(response.Containers)
	s.maintenanceMu.RLock()
	response.LastCleanupAt = s.lastCleanupAt
	response.CleanupRemoved = s.lastCleanupRemoved
	s.maintenanceMu.RUnlock()
	return response
}

func buildManagedContainerResponses(tasks []*Task, dockerContainers map[string]DockerContainer, dockerAvailable bool, cfg Config) []containerResponse {
	containers := make([]containerResponse, 0)
	for _, task := range tasks {
		containerName := task.ContainerName
		if containerName == "" {
			containerName = "ctf-agent-" + task.ID
		}
		dockerInfo, dockerFound := dockerContainers[containerName]
		if dockerAvailable && !dockerFound {
			continue
		}
		if !dockerAvailable && task.ContainerName == "" {
			continue
		}
		switch {
		case dockerFound:
		case task.Status == StatusRunning || task.ContainerKept:
		default:
			continue
		}
		containerState := managedContainerState(task, dockerInfo, dockerFound, dockerAvailable)
		image := cfg.ImageForCategory(task.Category)
		if dockerInfo.Image != "" {
			image = dockerInfo.Image
		}
		openCodeStatus, openCodeMessage, openCodeAvailable := openCodeState(task)
		canSendMessage, messageStatus := terminalMessageState(task)
		hasWriteup := task.Status == StatusSolved && task.WriteupFileName != ""
		containers = append(containers, containerResponse{
			TaskID:            task.ID,
			TaskName:          task.Name,
			Category:          task.Category,
			TaskStatus:        task.Status,
			ContainerName:     containerName,
			ContainerState:    containerState,
			Image:             image,
			DockerStatus:      dockerInfo.Status,
			DiskUsage:         dockerInfo.Size,
			DockerFound:       dockerFound,
			DockerRunning:     dockerInfo.Running,
			LastStep:          task.LastStep,
			OpenCodeSession:   task.OpenCodeSession,
			OpenCodeAvailable: openCodeAvailable,
			OpenCodeStatus:    openCodeStatus,
			OpenCodeMessage:   openCodeMessage,
			CanSendMessage:    canSendMessage,
			MessageStatus:     messageStatus,
			HasWriteup:        hasWriteup,
			CreatedAt:         task.CreatedAt,
			StartedAt:         task.StartedAt,
			FinishedAt:        task.FinishedAt,
			LogSize:           task.LogSize,
		})
	}
	return containers
}

func managedContainerState(task *Task, dockerInfo DockerContainer, dockerFound bool, dockerAvailable bool) string {
	if dockerAvailable && !dockerFound {
		return "missing"
	}
	if dockerFound && !dockerInfo.Running {
		return "exited"
	}
	if task.Status == StatusRunning || dockerInfo.Running {
		return "running"
	}
	return "retained"
}

func (s *Service) AppendLog(taskID string, text string) {
	if err := s.store.AppendLog(taskID, text); err != nil {
		slog.Error("append task log", "task_id", taskID, "error", err)
		return
	}
	if sessionID := extractOpenCodeSessionID(text); sessionID != "" {
		if err := s.store.MarkOpenCodeSession(taskID, sessionID); err != nil {
			slog.Error("mark opencode session", "task_id", taskID, "error", err)
		} else {
			s.publishTaskChanged(taskID)
		}
	}
	s.hub.Publish(taskID, text)
}

func (s *Service) publishTaskChanged(taskID string) {
	payload, err := json.Marshal(taskEvent{Type: "task_changed", TaskID: taskID})
	if err != nil {
		return
	}
	s.hub.PublishEvent(string(payload))
}

func (s *Service) queueIsFull() bool {
	return len(s.queue) >= cap(s.queue)
}

func (s *Service) enqueue(id string) bool {
	select {
	case s.queue <- id:
		return true
	case <-s.done:
		return false
	default:
		return false
	}
}

func extractOpenCodeSessionID(text string) string {
	match := openCodeSessionPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func (s *Service) worker(index int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			return
		default:
		}
		select {
		case <-s.done:
			return
		case id := <-s.queue:
			select {
			case <-s.done:
				return
			default:
			}
			if message, ok := s.takeQueuedMessage(id); ok {
				s.runContinuation(index, id, message)
				s.releaseTaskSlot(id)
				continue
			}
			s.runTask(index, id)
		}
	}
}

func (s *Service) runTask(workerIndex int, taskID string) {
	task, ok := s.store.Get(taskID)
	if !ok {
		return
	}
	if task.Status != StatusQueued {
		return
	}
	if err := s.store.MarkRunning(taskID); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to mark running: "+err.Error()+"\n")
		return
	}
	s.publishTaskChanged(taskID)
	s.AppendLog(taskID, "[dispatcher] worker="+strconv.Itoa(workerIndex)+" picked task="+taskID+"\n")

	ctx, cancel := s.taskRunContext(taskID)
	defer cancel()
	defer s.clearTaskCancel(taskID, cancel)

	result, err := s.initialDockerRun(
		ctx,
		task,
		func(containerName string) {
			if err := s.store.MarkRuntimeContainer(taskID, containerName); err != nil {
				s.AppendLog(taskID, "[dispatcher] failed to save runtime container: "+err.Error()+"\n")
			} else {
				s.publishTaskChanged(taskID)
			}
		},
	)
	if !s.finishRun(ctx, taskID, result, err, "task") {
		return
	}
}

func (s *Service) SendTaskMessage(taskID string, message string) error {
	task, ok := s.store.Get(taskID)
	if !ok {
		return os.ErrNotExist
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("message is required")
	}
	if task.Status == StatusRunning || task.Status == StatusQueued {
		return errTaskMessageBusy
	}
	if !task.ContainerKept || task.ContainerName == "" {
		return errors.New("容器未保留，不能继续发送")
	}
	if strings.TrimSpace(task.OpenCodeSession) == "" {
		return errors.New("缺少OpenCode终端会话，不能继续发送")
	}
	if _, err := s.activeOpenCodeProvider(); err != nil {
		return err
	}
	if !s.reserveTaskMessage(taskID, message) {
		return errTaskMessageBusy
	}
	if !s.enqueue(taskID) {
		s.releaseTaskSlot(taskID)
		return errTaskQueueFull
	}
	s.AppendLog(taskID, "[dispatcher] continuation queued\n")
	return nil
}

func (s *Service) runContinuation(workerIndex int, taskID string, message string) {
	current, ok := s.store.Get(taskID)
	if !ok || current.Status == StatusQueued || current.Status == StatusRunning ||
		!current.ContainerKept || current.ContainerName == "" || strings.TrimSpace(current.OpenCodeSession) == "" {
		return
	}
	if err := s.store.MarkRunning(taskID); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to mark continuation running: "+err.Error()+"\n")
		return
	}
	s.publishTaskChanged(taskID)
	s.AppendLog(taskID, "[dispatcher] worker="+strconv.Itoa(workerIndex)+" picked continuation\n")
	s.AppendLog(taskID, "\n[dispatcher] user message: "+message+"\n")

	ctx, cancel := s.taskRunContext(taskID)
	defer cancel()
	defer s.clearTaskCancel(taskID, cancel)
	result, err := s.continueDockerRun(ctx, current, message)
	s.finishRun(ctx, taskID, result, err, "continuation")
}

func (s *Service) reserveTaskMessage(taskID string, message string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.busy == nil {
		s.busy = map[string]bool{}
	}
	if s.queuedMessages == nil {
		s.queuedMessages = map[string]string{}
	}
	if s.busy[taskID] {
		return false
	}
	s.busy[taskID] = true
	s.queuedMessages[taskID] = message
	return true
}

func (s *Service) takeQueuedMessage(taskID string) (string, bool) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	message, ok := s.queuedMessages[taskID]
	if ok {
		delete(s.queuedMessages, taskID)
	}
	return message, ok
}

func (s *Service) releaseTaskSlot(taskID string) {
	s.runMu.Lock()
	delete(s.busy, taskID)
	delete(s.queuedMessages, taskID)
	s.runMu.Unlock()
}

func (s *Service) initialDockerRun(ctx context.Context, task *Task, containerSink func(string)) (DockerResult, error) {
	runDockerTask := s.runDockerTask
	if runDockerTask == nil {
		runDockerTask = RunDockerTask
	}
	result, err := runDockerTask(
		ctx,
		s.activeDockerConfig(),
		task,
		func(text string) {
			s.AppendLog(task.ID, text)
		},
		containerSink,
	)
	return s.enrichRunnerResult(ctx, task.ID, result), err
}

func (s *Service) continueDockerRun(ctx context.Context, task *Task, message string) (DockerResult, error) {
	runDockerHint := s.runDockerHint
	if runDockerHint == nil {
		runDockerHint = RunDockerHint
	}
	result, err := runDockerHint(ctx, s.activeDockerConfig(), task, message, func(text string) {
		s.AppendLog(task.ID, text)
	})
	return s.enrichRunnerResult(ctx, task.ID, result), err
}

func (s *Service) finishRun(ctx context.Context, taskID string, result DockerResult, err error, label string) bool {
	if err != nil {
		if s.taskWasStopped(taskID) {
			s.AppendLog(taskID, "[dispatcher] "+label+" stopped by user\n")
			s.publishTaskChanged(taskID)
			return false
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			_ = s.store.MarkFailed(taskID, taskTimeoutMessage)
			s.publishTaskChanged(taskID)
			s.AppendLog(taskID, "[dispatcher] "+label+" status=failed reason=timeout\n")
			return false
		}
		s.AppendLog(taskID, "[dispatcher] "+label+" failed before completion: "+err.Error()+"\n")
		_ = s.store.MarkFailed(taskID, err.Error())
		s.publishTaskChanged(taskID)
		s.AppendLog(taskID, "[dispatcher] task status=failed\n")
		return false
	}
	if s.taskWasStopped(taskID) {
		s.AppendLog(taskID, "[dispatcher] "+label+" stopped by user\n")
		s.publishTaskChanged(taskID)
		return false
	}
	result, err = s.runUntilSolved(ctx, taskID, result)
	if err != nil {
		if s.taskWasStopped(taskID) {
			s.AppendLog(taskID, "[dispatcher] task stopped by user\n")
			s.publishTaskChanged(taskID)
			return false
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			_ = s.store.MarkFailed(taskID, taskTimeoutMessage)
			s.publishTaskChanged(taskID)
			s.AppendLog(taskID, "[dispatcher] task status=failed reason=timeout\n")
			return false
		}
		s.AppendLog(taskID, "[dispatcher] task failed before completion: "+err.Error()+"\n")
		_ = s.store.MarkFailed(taskID, err.Error())
		s.publishTaskChanged(taskID)
		return false
	}
	if s.taskWasStopped(taskID) {
		s.AppendLog(taskID, "[dispatcher] task stopped by user\n")
		s.publishTaskChanged(taskID)
		return false
	}
	if err := s.markRunnerResult(taskID, result, label); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to mark finished: "+err.Error()+"\n")
		return false
	}
	s.publishTaskChanged(taskID)
	finished, _ := s.store.Get(taskID)
	s.AppendLog(taskID, "[dispatcher] task status="+string(finished.Status)+"\n")
	return true
}

func (s *Service) runUntilSolved(ctx context.Context, taskID string, result DockerResult) (DockerResult, error) {
	if result.Solved || result.ExitCode != 0 {
		return result, nil
	}
	for round := 1; round <= s.cfg.AutoContinueRounds; round++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		current, ok := s.store.Get(taskID)
		if !ok {
			return result, os.ErrNotExist
		}
		if s.taskWasStopped(taskID) {
			return result, nil
		}
		if result.Retained && result.ContainerName != "" {
			current.ContainerKept = true
			current.ContainerName = result.ContainerName
		}
		if !current.ContainerKept || current.ContainerName == "" || strings.TrimSpace(current.OpenCodeSession) == "" {
			return result, errContinueUnavailable
		}
		message := autoContinuePrompt(round)
		s.AppendLog(taskID, "[dispatcher] auto-continue round="+strconv.Itoa(round)+" until solved\n")
		next, err := s.continueDockerRun(ctx, current, message)
		if err != nil {
			return next, err
		}
		result = next
		if result.Solved || result.ExitCode != 0 {
			return result, nil
		}
	}
	if s.cfg.AutoContinueRounds > 0 {
		s.AppendLog(taskID, "[dispatcher] auto-continue limit reached rounds="+strconv.Itoa(s.cfg.AutoContinueRounds)+"\n")
	}
	return result, nil
}

func (s *Service) enrichRunnerResult(ctx context.Context, taskID string, result DockerResult) DockerResult {
	if result.ContainerName == "" {
		if task, ok := s.store.Get(taskID); ok {
			result.ContainerName = task.ContainerName
			result.Retained = task.ContainerKept
		}
	}
	logs, ok := s.store.Logs(taskID)
	if !ok {
		return result
	}
	parsed := parseRunnerOutput(logs)
	if parsed.Solved {
		result.Solved = true
		result.Flag = parsed.Flag
	}
	if parsed.WriteupFileName != "" {
		result.WriteupFileName = parsed.WriteupFileName
	}
	if result.Solved && result.WriteupFileName != "" && result.WriteupContent == "" &&
		result.ContainerName != "" && ctx.Err() == nil {
		content, err := ReadContainerWorkspaceFile(ctx, result.ContainerName, result.WriteupFileName)
		if err == nil {
			result.WriteupContent = content
		} else {
			s.AppendLog(taskID, "[dispatcher] failed to read writeup from container: "+err.Error()+"\n")
		}
	}
	return result
}

func autoContinuePrompt(round int) string {
	return "继续解这道CTF题。上一轮没有按协议输出“这道题目已经解出”。" +
		"请基于已有文件、脚本和发现继续验证，不要重复无关枚举。第" + strconv.Itoa(round) +
		"次自动续跑。只有确认Flag后，才按两行协议输出：第一行“这道题目已经解出”，第二行输出完整Flag。"
}

func (s *Service) markRunnerResult(taskID string, result DockerResult, label string) error {
	if result.Solved {
		if err := s.store.MarkFinished(taskID, result.ExitCode, &result.Flag, "Flag已捕获", result.ContainerName, false); err != nil {
			return err
		}
		if result.WriteupFileName != "" && result.WriteupContent != "" {
			if err := s.store.SaveWriteup(taskID, result.WriteupFileName, result.WriteupContent); err != nil {
				return err
			}
		}
		_ = CloseTaskContainer(result.ContainerName)
		return nil
	}
	message := runnerFailureMessage(result, label, s.cfg)
	if result.ExitCode == 0 {
		message = "OpenCode尚未解出，等待继续"
		return s.store.MarkCompleted(taskID, result.ExitCode, message, result.ContainerName, result.Retained)
	}
	return s.store.MarkFinishedWithFailureMessage(taskID, result.ExitCode, nil, message, result.ContainerName, result.Retained, message)
}

func runnerFailureMessage(result DockerResult, label string, cfg Config) string {
	if result.OOMKilled {
		return "OpenCode因容器内存不足被系统终止（当前上限" + cfg.MemLimit + "），请提高CTF_AGENT_MEM_LIMIT并重启服务后重新提交任务"
	}
	switch result.ExitCode {
	case bridgeRunTimeoutExitCode:
		return "OpenCode单轮执行超过" + cfg.OpenCodeRunTimeout.String() + "上限，容器已保留，可发送消息继续同一session"
	case bridgeIdleTimeoutExitCode:
		return "OpenCode连续" + cfg.OpenCodeIdleTimeout.String() + "无输出，已停止本轮；容器已保留，可发送消息继续同一session"
	}
	if label == "continuation" {
		return "OpenCode继续执行失败"
	}
	return "OpenCode本轮执行失败"
}

func (s *Service) taskWasStopped(taskID string) bool {
	task, ok := s.store.Get(taskID)
	return ok && task.Status == StatusFailed && task.ContainerName == "" &&
		(task.LastStep == containerClosedMessage || task.LastStep == taskStoppedMessage)
}

func (s *Service) taskRunContext(taskID string) (context.Context, context.CancelFunc) {
	parent := context.Background()
	var parentCancel context.CancelFunc
	if s.cfg.TaskTimeout > 0 {
		parent, parentCancel = context.WithTimeout(parent, s.cfg.TaskTimeout)
	}
	ctx, cancel := context.WithCancel(parent)
	combinedCancel := func() {
		cancel()
		if parentCancel != nil {
			parentCancel()
		}
	}
	s.runMu.Lock()
	if s.runs == nil {
		s.runs = map[string]context.CancelFunc{}
	}
	s.runs[taskID] = combinedCancel
	s.runMu.Unlock()
	select {
	case <-s.done:
		combinedCancel()
	default:
	}
	return ctx, combinedCancel
}

func (s *Service) clearTaskCancel(taskID string, cancel context.CancelFunc) {
	s.runMu.Lock()
	delete(s.runs, taskID)
	s.runMu.Unlock()
}

func (s *Service) cancelTask(taskID string) {
	s.runMu.Lock()
	cancel := s.runs[taskID]
	s.runMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Service) cancelAllRuns() {
	s.runMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.runs))
	for _, cancel := range s.runs {
		cancels = append(cancels, cancel)
	}
	s.runMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Service) CloseTaskContainer(taskID string) error {
	task, ok := s.store.Get(taskID)
	if !ok {
		return os.ErrNotExist
	}
	s.cancelTask(taskID)
	s.releaseTaskSlot(taskID)
	if task.Status == StatusQueued {
		if err := s.store.MarkStopped(taskID, taskStoppedMessage); err != nil {
			return err
		}
		s.publishTaskChanged(taskID)
		s.AppendLog(taskID, "[dispatcher] queued task stopped by user\n")
		return nil
	}
	if task.ContainerName == "" || (!task.ContainerKept && task.Status != StatusRunning) {
		return errors.New("task has no closable container")
	}
	if err := CloseTaskContainer(task.ContainerName); err != nil {
		return err
	}
	if err := s.store.MarkContainerClosed(taskID); err != nil {
		return err
	}
	s.publishTaskChanged(taskID)
	s.AppendLog(taskID, "[dispatcher] container closed by user\n")
	return nil
}

func (s *Service) recoverInterruptedRunningContainers() {
	for _, task := range s.store.List() {
		if task.Status != StatusRunning || task.ContainerName == "" {
			continue
		}
		if err := s.store.MarkInterruptedContainerRetained(task.ID); err != nil {
			slog.Error("retain interrupted container", "task_id", task.ID, "container", task.ContainerName, "error", err)
			continue
		}
		s.AppendLog(task.ID, "[dispatcher] service restarted; existing container retained for manual inspection\n")
	}
}

func (s *Service) retentionCleaner() {
	defer s.wg.Done()
	s.cleanupExpiredContainers()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.cleanupExpiredContainers()
		}
	}
}

func (s *Service) cleanupExpiredContainers() {
	if s.cfg.ContainerRetention <= 0 {
		return
	}
	containers, err := ListDockerContainers()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	removed := 0
	tracked := map[string]*Task{}
	for _, task := range s.store.List() {
		if task.ContainerName == "" {
			continue
		}
		tracked[task.ContainerName] = task
		_, exists := containers[task.ContainerName]
		if task.ContainerKept && !exists {
			_ = s.store.MarkContainerClosed(task.ID)
			continue
		}
		if task.Status == StatusRunning || !task.ContainerKept || !exists {
			continue
		}
		retainedAt := task.CreatedAt
		if task.FinishedAt != nil {
			retainedAt = *task.FinishedAt
		}
		if now.Sub(retainedAt) < s.cfg.ContainerRetention {
			continue
		}
		if err := CloseTaskContainer(task.ContainerName); err == nil {
			removed++
			_ = s.store.MarkContainerClosed(task.ID)
			s.AppendLog(task.ID, "[maintenance] expired retained container removed\n")
			s.publishTaskChanged(task.ID)
		}
	}
	for name := range containers {
		if _, ok := tracked[name]; ok {
			continue
		}
		createdAt, err := DockerContainerCreatedAt(name)
		if err == nil && now.Sub(createdAt) >= s.cfg.ContainerRetention {
			if CloseTaskContainer(name) == nil {
				removed++
			}
		}
	}
	s.maintenanceMu.Lock()
	s.lastCleanupAt = &now
	s.lastCleanupRemoved = removed
	s.maintenanceMu.Unlock()
}

func summarizeLastStep(logs string) string {
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[opencode] INFO") || strings.HasPrefix(line, "[opencode] DEBUG") {
			continue
		}
		if len(line) > 220 {
			line = line[:220]
		}
		return line
	}
	return "暂无可用步骤"
}
