package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	finalReadableOutputMarker   = "Observation: final readable OpenCode output:"
	solvedFlagOutputMarker      = "这道题目已经解出"
	generatedWriteupBeginMarker = "-----BEGIN_CTF_AGENT_WRITEUP-----"
	generatedWriteupEndMarker   = "-----END_CTF_AGENT_WRITEUP-----"
	liveFlagCaptureMarker       = "[dispatcher] captured solved flag from live OpenCode output"
	maxGeneratedWriteupBytes    = 512 * 1024
)

var openCodeSessionPattern = regexp.MustCompile(`Observation:\s+OpenCode session=([A-Za-z0-9_-]+)`)
var flagTokenPattern = regexp.MustCompile("(?i)\\b[A-Za-z0-9_-]*flag\\{[^`'\"<>\\s]+\\}")

type taskEvent struct {
	Type   string `json:"type"`
	TaskID string `json:"task_id,omitempty"`
}

type providerSelectionState struct {
	Format string `json:"format"`
}

var errTaskMessageBusy = errors.New("当前回合运行中，结束后可继续发送")

type dockerTaskRunner func(Config, *Task, LogSink, func(string)) (DockerResult, error)
type dockerHintRunner func(Config, *Task, string, LogSink) (DockerResult, error)

type Service struct {
	cfg   Config
	store *Store
	hub   *Hub
	queue chan string
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once

	providerMu           sync.RWMutex
	activeProviderFormat string

	runDockerTask dockerTaskRunner
	runDockerHint dockerHintRunner
}

func NewService(cfg Config) (*Service, error) {
	cfg = cfg.withOpenCodeProviderDefaults()
	if err := os.MkdirAll(cfg.ChallengeDir, 0o755); err != nil {
		return nil, err
	}
	store, err := NewStore(cfg.ChallengeDir)
	if err != nil {
		return nil, err
	}
	service := &Service{
		cfg:   cfg,
		store: store,
		hub:   NewHub(),
		queue: make(chan string, max(1, cfg.MaxContainers)*4),
		done:  make(chan struct{}),

		runDockerTask: RunDockerTask,
		runDockerHint: RunDockerHint,
	}
	service.activeProviderFormat = service.loadProviderFormat()
	service.repairFinishedTaskFlags()
	service.repairInvalidSolvedFlags()
	service.repairSparseWriteups()
	service.repairSolvedTaskMetadata()
	service.repairFalseLiveCapturedFlags()
	service.recoverInterruptedRunningContainers()
	for i := 0; i < max(1, cfg.MaxContainers); i++ {
		service.wg.Add(1)
		go service.worker(i)
	}
	for _, id := range store.RecoverableIDs() {
		if err := store.MarkRecovered(id); err != nil {
			log.Printf("recover task %s: %v", id, err)
			continue
		}
		service.AppendLog(id, "[dispatcher] recovered queued task after service startup\n")
		service.enqueue(id)
	}
	return service, nil
}

func (s *Service) Close() {
	s.once.Do(func() {
		close(s.done)
		s.wg.Wait()
	})
}

func (s *Service) NewTaskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return strconvItoa(int(time.Now().UnixNano()))
	}
	return hex.EncodeToString(buf[:])
}

func (s *Service) Submit(task *Task) error {
	if _, err := s.activeOpenCodeProvider(); err != nil {
		return err
	}
	if err := s.store.Add(task); err != nil {
		return err
	}
	s.publishTaskChanged(task.ID)
	s.enqueue(task.ID)
	s.AppendLog(task.ID, "[dispatcher] queued workers="+strconvItoa(max(1, s.cfg.MaxContainers))+"\n")
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
	return os.WriteFile(s.providerStatePath(), data, 0o644)
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
	return response
}

func buildManagedContainerResponses(tasks []*Task, dockerContainers map[string]DockerContainer, dockerAvailable bool, cfg Config) []containerResponse {
	containers := make([]containerResponse, 0)
	for _, task := range tasks {
		if task.Flag != nil {
			continue
		}
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
		containers = append(containers, containerResponse{
			TaskID:            task.ID,
			TaskName:          task.Name,
			Category:          task.Category,
			TaskStatus:        task.Status,
			ContainerName:     containerName,
			ContainerState:    containerState,
			Image:             image,
			DockerStatus:      dockerInfo.Status,
			DockerFound:       dockerFound,
			DockerRunning:     dockerInfo.Running,
			LastStep:          task.LastStep,
			OpenCodeSession:   task.OpenCodeSession,
			OpenCodeAvailable: openCodeAvailable,
			OpenCodeStatus:    openCodeStatus,
			OpenCodeMessage:   openCodeMessage,
			CanSendMessage:    canSendMessage,
			MessageStatus:     messageStatus,
			HasWriteup:        task.Status == StatusSolved && task.WriteupFileName != "",
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
		log.Printf("append log %s: %v", taskID, err)
		return
	}
	if sessionID := extractOpenCodeSessionID(text); sessionID != "" {
		if err := s.store.MarkOpenCodeSession(taskID, sessionID); err != nil {
			log.Printf("mark opencode session %s: %v", taskID, err)
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

func (s *Service) enqueue(id string) {
	select {
	case s.queue <- id:
	case <-s.done:
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
			s.runTask(index, id)
		}
	}
}

func (s *Service) runTask(workerIndex int, taskID string) {
	task, ok := s.store.Get(taskID)
	if !ok {
		return
	}
	if err := s.store.MarkRunning(taskID); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to mark running: "+err.Error()+"\n")
		return
	}
	s.publishTaskChanged(taskID)
	s.AppendLog(taskID, "[dispatcher] worker="+strconvItoa(workerIndex)+" picked task="+taskID+"\n")

	runDockerTask := s.runDockerTask
	if runDockerTask == nil {
		runDockerTask = RunDockerTask
	}
	result, err := runDockerTask(
		s.activeDockerConfig(),
		task,
		func(text string) {
			s.AppendLog(taskID, text)
		},
		func(containerName string) {
			if err := s.store.MarkRuntimeContainer(taskID, containerName); err != nil {
				s.AppendLog(taskID, "[dispatcher] failed to save runtime container: "+err.Error()+"\n")
			} else {
				s.publishTaskChanged(taskID)
			}
		},
	)
	if err != nil {
		if s.taskWasManuallyClosed(taskID) {
			s.AppendLog(taskID, "[dispatcher] task stopped because container was closed manually\n")
			s.publishTaskChanged(taskID)
			return
		}
		s.AppendLog(taskID, "[dispatcher] task failed before completion: "+err.Error()+"\n")
		_ = s.store.MarkFailed(taskID, err.Error())
		s.publishTaskChanged(taskID)
		s.AppendLog(taskID, "[dispatcher] task status=failed\n")
		return
	}
	if s.taskWasManuallyClosed(taskID) {
		s.AppendLog(taskID, "[dispatcher] task stopped because container was closed manually\n")
		s.publishTaskChanged(taskID)
		return
	}

	logs, _ := s.store.Logs(taskID)
	flag := s.extractFlagForTask(task, logs)
	lastStep := summarizeLastStep(logs)
	solved := flag != nil
	if !solved {
		if openCodeErr := extractOpenCodeBridgeError(logs); openCodeErr != "" {
			lastStep = openCodeErr
		}
	}
	containerKept := result.Retained && !solved
	if solved && result.ContainerName != "" {
		if err := CloseTaskContainer(result.ContainerName); err != nil {
			s.AppendLog(taskID, "[dispatcher] failed to remove solved container: "+err.Error()+"\n")
			containerKept = true
		} else {
			s.AppendLog(taskID, "[runner] removing solved container_name="+result.ContainerName+"\n")
		}
	}
	if err := s.store.MarkFinished(taskID, result.ExitCode, flag, lastStep, result.ContainerName, containerKept); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to mark finished: "+err.Error()+"\n")
		return
	}
	s.saveWriteup(taskID)
	s.publishTaskChanged(taskID)
	finished, _ := s.store.Get(taskID)
	s.AppendLog(taskID, "[dispatcher] task status="+string(finished.Status)+"\n")
}

func (s *Service) ContinueTask(taskID string, hint string) error {
	return s.SendTaskMessage(taskID, hint)
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
	if task.Status == StatusSolved {
		return errors.New("任务已解出，不能继续发送")
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
	if err := s.store.MarkRunning(taskID); err != nil {
		return err
	}
	s.publishTaskChanged(taskID)
	s.AppendLog(taskID, "\n[dispatcher] user message: "+message+"\n")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		current, ok := s.store.Get(taskID)
		if !ok {
			return
		}
		runDockerHint := s.runDockerHint
		if runDockerHint == nil {
			runDockerHint = RunDockerHint
		}
		result, err := runDockerHint(s.activeDockerConfig(), current, message, func(text string) {
			s.AppendLog(taskID, text)
		})
		if err != nil {
			if s.taskWasManuallyClosed(taskID) {
				s.AppendLog(taskID, "[dispatcher] continuation stopped because container was closed manually\n")
				s.publishTaskChanged(taskID)
				return
			}
			s.AppendLog(taskID, "[dispatcher] continuation failed: "+err.Error()+"\n")
			_ = s.store.MarkFailed(taskID, err.Error())
			s.publishTaskChanged(taskID)
			return
		}
		if s.taskWasManuallyClosed(taskID) {
			s.AppendLog(taskID, "[dispatcher] continuation stopped because container was closed manually\n")
			s.publishTaskChanged(taskID)
			return
		}
		logs, _ := s.store.Logs(taskID)
		flag := s.extractFlagForTask(current, logs)
		lastStep := summarizeLastStep(logs)
		solved := flag != nil
		if !solved {
			if openCodeErr := extractOpenCodeBridgeError(logs); openCodeErr != "" {
				lastStep = openCodeErr
			}
		}
		containerKept := result.Retained && !solved
		if solved && result.ContainerName != "" {
			if err := CloseTaskContainer(result.ContainerName); err != nil {
				s.AppendLog(taskID, "[dispatcher] failed to remove solved container: "+err.Error()+"\n")
				containerKept = true
			} else {
				s.AppendLog(taskID, "[runner] removing solved container_name="+result.ContainerName+"\n")
			}
		}
		if err := s.store.MarkFinished(taskID, result.ExitCode, flag, lastStep, result.ContainerName, containerKept); err != nil {
			s.AppendLog(taskID, "[dispatcher] failed to mark continuation finished: "+err.Error()+"\n")
			return
		}
		s.saveWriteup(taskID)
		s.publishTaskChanged(taskID)
		finished, _ := s.store.Get(taskID)
		s.AppendLog(taskID, "[dispatcher] task status="+string(finished.Status)+"\n")
	}()
	return nil
}

func (s *Service) taskWasManuallyClosed(taskID string) bool {
	task, ok := s.store.Get(taskID)
	return ok && task.Status == StatusFailed && task.ContainerName == "" && task.LastStep == containerClosedMessage
}

func (s *Service) CloseTaskContainer(taskID string) error {
	task, ok := s.store.Get(taskID)
	if !ok {
		return os.ErrNotExist
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

func (s *Service) saveWriteup(taskID string) {
	task, ok := s.store.Get(taskID)
	if !ok {
		return
	}
	if task.Status != StatusSolved || task.Flag == nil {
		return
	}
	logs, _ := s.store.Logs(taskID)
	filename := safeWriteupFilename(task.Name)
	content := buildStoredWriteup(task, logs)
	if err := s.store.SaveWriteup(taskID, filename, content); err != nil {
		s.AppendLog(taskID, "[dispatcher] failed to write wp: "+err.Error()+"\n")
		return
	}
	s.AppendLog(taskID, "[dispatcher] writeup saved: "+filename+"\n")
}

func (s *Service) repairSparseWriteups() {
	for _, task := range s.store.List() {
		if task.Status != StatusSolved {
			continue
		}
		path, _, ok := s.store.WriteupPath(task.ID)
		needsRepair := !ok
		if ok {
			content, err := os.ReadFile(path)
			needsRepair = err != nil || writeupNeedsRepair(task, string(content))
		}
		if !needsRepair {
			continue
		}
		logs, _ := s.store.Logs(task.ID)
		if err := s.store.SaveWriteup(task.ID, safeWriteupFilename(task.Name), buildStoredWriteup(task, logs)); err != nil {
			log.Printf("repair writeup %s: %v", task.ID, err)
		}
	}
}

func (s *Service) repairFinishedTaskFlags() {
	for _, task := range s.store.List() {
		if task.Status == StatusQueued || task.Status == StatusRunning {
			continue
		}
		logs, _ := s.store.Logs(task.ID)
		flag := s.extractFlagForTask(task, logs)
		if flag == nil {
			continue
		}
		if task.Flag != nil && *task.Flag == *flag {
			continue
		}
		if err := s.store.MarkFlag(task.ID, *flag); err != nil {
			log.Printf("repair flag %s: %v", task.ID, err)
			continue
		}
		repaired, ok := s.store.Get(task.ID)
		if !ok {
			continue
		}
		if err := s.store.SaveWriteup(task.ID, safeWriteupFilename(repaired.Name), buildStoredWriteup(repaired, logs)); err != nil {
			log.Printf("repair writeup after flag %s: %v", task.ID, err)
		}
	}
}

func (s *Service) repairInvalidSolvedFlags() {
	for _, task := range s.store.List() {
		if task.Flag == nil {
			continue
		}
		invalid := !isUsableFinalFlag(*task.Flag)
		if !invalid {
			if logs, ok := s.store.Logs(task.ID); ok &&
				strings.Contains(logs, finalReadableOutputMarker) &&
				extractFinalLineFlag(logs) == "" {
				invalid = true
			}
		}
		if !invalid {
			continue
		}
		if err := s.store.MarkInvalidFlag(task.ID, "captured flag is invalid; OpenCode output did not contain a usable final flag"); err != nil {
			log.Printf("repair invalid flag %s: %v", task.ID, err)
		}
	}
}

func (s *Service) repairSolvedTaskMetadata() {
	for _, task := range s.store.List() {
		if task.Status != StatusSolved || task.Flag == nil {
			continue
		}
		if task.FinishedAt != nil && strings.TrimSpace(task.LastStep) != "" && task.LastStep != "任务正在执行" {
			continue
		}
		if err := s.store.MarkFlag(task.ID, *task.Flag); err != nil {
			log.Printf("repair solved metadata %s: %v", task.ID, err)
			continue
		}
		repaired, ok := s.store.Get(task.ID)
		if !ok {
			continue
		}
		logs, _ := s.store.Logs(task.ID)
		if err := s.store.SaveWriteup(task.ID, safeWriteupFilename(repaired.Name), buildStoredWriteup(repaired, logs)); err != nil {
			log.Printf("repair solved metadata writeup %s: %v", task.ID, err)
		}
	}
}

func (s *Service) repairFalseLiveCapturedFlags() {
	containers, err := ListDockerContainers()
	if err != nil {
		return
	}
	s.repairFalseLiveCapturedFlagsFromContainers(containers)
}

func (s *Service) repairFalseLiveCapturedFlagsFromContainers(containers map[string]DockerContainer) {
	if len(containers) == 0 {
		return
	}
	const message = "历史运行中Flag误捕获已纠正，容器已保留，可继续同一OpenCode会话"
	for _, task := range s.store.List() {
		if task.Status != StatusSolved || task.Flag == nil || task.ExitCode != nil {
			continue
		}
		if task.ContainerName == "" || !isManagedContainerName(task.ContainerName) {
			continue
		}
		if _, ok := containers[task.ContainerName]; !ok {
			continue
		}
		logs, ok := s.store.Logs(task.ID)
		if !ok || !strings.Contains(logs, liveFlagCaptureMarker) {
			continue
		}
		if err := s.store.MarkFalseLiveCapture(task.ID, message); err != nil {
			log.Printf("repair false live flag capture %s: %v", task.ID, err)
			continue
		}
		if err := s.store.AppendLog(task.ID, "[dispatcher] "+message+"\n"); err != nil {
			log.Printf("append false live capture repair log %s: %v", task.ID, err)
		}
		s.publishTaskChanged(task.ID)
	}
}

func extractOpenCodeExportText(data []byte) string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return ""
	}
	blocks := make([]string, 0)
	var walk func(any, string)
	walk = func(node any, inheritedRole string) {
		switch item := node.(type) {
		case map[string]any:
			role := inheritedRole
			if info, ok := item["info"].(map[string]any); ok {
				if value, ok := info["role"].(string); ok {
					role = strings.ToLower(strings.TrimSpace(value))
				}
			}
			for _, key := range []string{"role", "speaker"} {
				if value, ok := item[key].(string); ok {
					role = strings.ToLower(strings.TrimSpace(value))
				}
			}
			for _, key := range []string{"text", "content", "message", "output", "result", "error"} {
				value, ok := item[key].(string)
				if !ok {
					continue
				}
				clean := strings.TrimSpace(value)
				if clean == "" || role == "user" || role == "system" || looksLikePromptEcho(clean) {
					continue
				}
				blocks = append(blocks, clean)
			}
			for _, child := range item {
				walk(child, role)
			}
		case []any:
			for _, child := range item {
				walk(child, inheritedRole)
			}
		}
	}
	walk(value, "")
	return strings.Join(dedupeStrings(blocks), "\n\n")
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func extractFlagToken(text string) string {
	matches := flagTokenPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1])
}

func tailText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[len(runes)-maxRunes:])
}

func (s *Service) recoverInterruptedRunningContainers() {
	for _, task := range s.store.List() {
		if task.Status != StatusRunning || task.ContainerName == "" {
			continue
		}
		if err := s.store.MarkInterruptedContainerRetained(task.ID); err != nil {
			log.Printf("retain interrupted container %s: %v", task.ID, err)
			continue
		}
		s.AppendLog(task.ID, "[dispatcher] service restarted; existing container retained for manual inspection\n")
	}
}

func (s *Service) extractFlagForTask(task *Task, logs string) *string {
	if flag := extractMarkedSolvedFlag(logs); flag != "" {
		return &flag
	}
	if flag := extractFinalLineFlag(logs); flag != "" {
		return &flag
	}
	return nil
}

func extractMarkedSolvedFlag(logs string) string {
	output := extractReadableOutput(logs)
	if output == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for index, line := range lines {
		if strings.TrimSpace(stripANSI(line)) != solvedFlagOutputMarker {
			continue
		}
		if markerLooksInstructional(lines, index) {
			continue
		}
		for _, candidate := range lines[index+1:] {
			cleaned := strings.TrimSpace(stripANSI(candidate))
			if cleaned == "" || isPlatformLogLine(cleaned) {
				continue
			}
			if flag := normalizeFinalFlagLine(cleaned); flag != "" {
				return flag
			}
			break
		}
	}
	return ""
}

func markerLooksInstructional(lines []string, index int) bool {
	start := max(0, index-4)
	context := strings.ToLower(strings.Join(lines[start:index], "\n"))
	for _, marker := range []string{
		"prompt要求",
		"final output contract",
		"flag output contract",
		"strictly output",
		"marker block",
		"最终严格输出",
		"按以下两行",
		"格式收尾",
		"格式要求",
		"示例",
		"example",
		"<exact flag>",
		"<captured-or-last-line>",
	} {
		if strings.Contains(context, marker) {
			return true
		}
	}
	return false
}

func extractFinalLineFlag(logs string) string {
	output := extractReadableOutput(logs)
	for _, line := range reversedLines(output) {
		if flag := normalizeFinalFlagLine(line); flag != "" {
			return flag
		}
	}
	return ""
}

func reversedLines(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}

func normalizeFinalFlagLine(line string) string {
	line = strings.TrimSpace(stripANSI(line))
	if line == "" || isPlatformLogLine(line) {
		return ""
	}
	lineForLabel := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "))
	if value, ok := flagValueAfterLabel(lineForLabel); ok {
		line = value
		if code := lastBacktickValue(line); code != "" {
			line = code
		}
	} else if code := wholeBacktickValue(line); code != "" {
		line = code
	}
	line = strings.Trim(strings.TrimSpace(line), "`'\"，。；;")
	if !isUsableFinalFlag(line) {
		return ""
	}
	return line
}

func flagValueAfterLabel(line string) (string, bool) {
	for _, separator := range []string{"最终 Flag:", "最终Flag:", "Final Flag:", "Flag:", "flag:", "：", ":"} {
		if index := strings.LastIndex(line, separator); index >= 0 {
			prefix := strings.TrimSpace(line[:index])
			if strings.Contains(strings.ToLower(prefix), "flag") || strings.Contains(prefix, "最终") {
				return strings.TrimSpace(line[index+len(separator):]), true
			}
		}
	}
	return "", false
}

func wholeBacktickValue(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "`") || !strings.HasSuffix(line, "`") {
		return ""
	}
	if match := regexp.MustCompile("^`([^`]+)`$").FindStringSubmatch(line); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func lastBacktickValue(line string) string {
	match := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(line, -1)
	if len(match) == 0 {
		return ""
	}
	return strings.TrimSpace(match[len(match)-1][1])
}

func isUsableFinalFlag(value string) bool {
	value = strings.TrimSpace(value)
	runeCount := len([]rune(value))
	if runeCount < 4 || runeCount > 300 {
		return false
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if strings.Contains(value, ":") {
		return false
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	if strings.ContainsAny(value, "()[]") {
		return false
	}
	hasAlphaNum := false
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' {
			hasAlphaNum = true
			break
		}
	}
	if !hasAlphaNum {
		return false
	}
	return true
}

func isPlatformLogLine(line string) bool {
	return strings.HasPrefix(line, "[dispatcher]") ||
		strings.HasPrefix(line, "[runner]") ||
		strings.HasPrefix(line, "[opencode]") ||
		strings.HasPrefix(line, "\x1b[36m[agent]")
}

func stripANSI(text string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(text, "")
}

func extractOpenCodeBridgeError(logs string) string {
	const marker = "Final: opencode bridge failed:"
	for _, line := range reversedLines(logs) {
		cleaned := strings.TrimSpace(stripANSI(line))
		if strings.HasPrefix(cleaned, marker) {
			return strings.TrimSpace(cleaned)
		}
	}
	return ""
}

func buildStoredWriteup(task *Task, logs string) string {
	if content, ok := extractGeneratedWriteup(logs); ok {
		return content
	}
	return buildWriteup(task, logs)
}

func extractGeneratedWriteup(logs string) (string, bool) {
	normalized := strings.ReplaceAll(logs, "\r\n", "\n")
	searchEnd := len(normalized)
	for searchEnd > 0 {
		end := strings.LastIndex(normalized[:searchEnd], generatedWriteupEndMarker)
		if end < 0 {
			return "", false
		}
		begin := strings.LastIndex(normalized[:end], generatedWriteupBeginMarker)
		if begin < 0 {
			searchEnd = end
			continue
		}
		raw := normalized[begin+len(generatedWriteupBeginMarker) : end]
		content := normalizeGeneratedWriteup(raw)
		if isValidGeneratedWriteup(content) {
			return content, true
		}
		searchEnd = begin
	}
	return "", false
}

func normalizeGeneratedWriteup(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripANSI(text)
	text = strings.ReplaceAll(text, generatedWriteupBeginMarker, "")
	text = strings.ReplaceAll(text, generatedWriteupEndMarker, "")
	text = strings.TrimSpace(text)
	if len([]byte(text)) > maxGeneratedWriteupBytes {
		data := []byte(text)
		text = strings.ToValidUTF8(string(data[:maxGeneratedWriteupBytes]), "")
		text = strings.TrimSpace(text) + "\n\n> WP内容超过平台限制，后续内容已截断。"
	}
	return textWithTrailingNewline(text)
}

func isValidGeneratedWriteup(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if len([]rune(trimmed)) < 40 {
		return false
	}
	if looksLikePromptEcho(trimmed) {
		return false
	}
	for _, prefix := range []string{
		"Observation:",
		"Thought:",
		"Action:",
		"[dispatcher]",
		"[runner]",
		"[opencode]",
	} {
		if strings.HasPrefix(trimmed, prefix) {
			return false
		}
	}
	return true
}

func textWithTrailingNewline(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return text + "\n"
}

func buildWriteup(task *Task, logs string) string {
	var builder strings.Builder
	builder.WriteString("# " + task.Name + "\n\n")
	builder.WriteString("## 基本信息\n\n")
	builder.WriteString("- 题型:" + task.Category + "\n")
	builder.WriteString("- 状态:" + writeupStatus(task.Status) + "\n")
	if task.TargetIP != "" {
		builder.WriteString("- 目标:" + task.TargetIP + "\n")
	}
	if task.Flag != nil {
		builder.WriteString("- Flag:" + *task.Flag + "\n")
	}
	if task.Error != nil {
		builder.WriteString("- 错误:" + *task.Error + "\n")
	}
	builder.WriteString("\n## 题目描述\n\n")
	builder.WriteString(task.Description + "\n\n")
	builder.WriteString("## 解题过程\n\n")
	body := formatWriteupBody(localizeWriteupText(cleanWriteupLogs(logs)))
	if shouldUseWriteupFallback(task, body) {
		body = fallbackWriteupBody(task)
	}
	builder.WriteString(body)
	builder.WriteString("\n")
	return builder.String()
}

func writeupStatus(status TaskStatus) string {
	switch status {
	case StatusQueued:
		return "正在解题"
	case StatusRunning:
		return "正在解题"
	case StatusSolved:
		return "已解出"
	case StatusFailed:
		return "未解出"
	default:
		return string(status)
	}
}

func localizeWriteupText(text string) string {
	replacer := strings.NewReplacer(
		"Working on the crypto challenge", "正在分析密码题",
		"I need to solve", "需要解出",
		"My goal is to print the final flag", "目标是输出最终Flag",
		"Since ", "因为",
		"we can recreate", "可以复现",
		"Flag:", "Flag:",
		"Attachments are mounted read-only at /attachments.", "附件以只读方式挂载在/attachments。",
		"Attachment files:", "附件文件:",
		"End of file", "文件结束",
		"total", "共",
		"lines", "行",
	)
	return replacer.Replace(text)
}

func formatWriteupBody(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = convertTaggedFileBlocks(text)
	text = stripStandaloneContentTags(text)
	text = strings.TrimSpace(text)
	if text == "" {
		return "暂无可用解题过程。\n"
	}
	return text + "\n"
}

func shouldUseWriteupFallback(task *Task, body string) bool {
	if task.Status == StatusSolved {
		return false
	}
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || trimmed == "暂无可用解题过程。" {
		return true
	}
	if len([]rune(trimmed)) < 80 && !strings.Contains(strings.ToLower(trimmed), "flag") {
		return true
	}
	return false
}

func writeupNeedsRepair(task *Task, content string) bool {
	if task.Status != StatusSolved {
		return false
	}
	body := writeupProcessBody(content)
	trimmed := strings.TrimSpace(body)
	if trimmed == "" || trimmed == "暂无可用解题过程。" {
		return true
	}
	if len([]rune(trimmed)) < 120 &&
		(strings.HasPrefix(trimmed, "Thought: challenge=") ||
			strings.HasPrefix(trimmed, "Action: start opencode")) {
		return true
	}
	return false
}

func writeupProcessBody(content string) string {
	if _, body, ok := strings.Cut(content, "## 解题过程"); ok {
		return body
	}
	return content
}

func fallbackWriteupBody(task *Task) string {
	var builder strings.Builder
	builder.WriteString("平台尚未捕获到OpenCode最终可读输出。\n\n")
	builder.WriteString("可能原因:\n\n")
	builder.WriteString("- 模型仍在OpenCode会话中运行，但尚未输出可识别Flag。\n")
	builder.WriteString("- OpenCode会话已经产生过程，但桥接脚本尚未成功导出最终文本。\n")
	builder.WriteString("- 模型输出没有包含可识别的Flag或完整中文WP。\n\n")
	if task.OpenCodeSession != "" || task.ContainerKept {
		builder.WriteString("建议继续操作:\n\n")
	}
	if task.OpenCodeSession != "" {
		builder.WriteString("- OpenCode会话:" + task.OpenCodeSession + "\n")
	}
	if task.ContainerKept {
		builder.WriteString("- 容器仍保留，可以在任务详情终端下方发送消息继续解题，或在Docker管理页销毁容器。\n")
	}
	if task.LastStep != "" {
		builder.WriteString("\n最后捕获步骤:\n\n")
		builder.WriteString(task.LastStep + "\n")
	}
	return builder.String()
}

func convertTaggedFileBlocks(text string) string {
	var builder strings.Builder
	rest := text
	for {
		pathStart := strings.Index(rest, "<path>")
		if pathStart < 0 {
			builder.WriteString(rest)
			break
		}
		builder.WriteString(rest[:pathStart])
		pathEnd := strings.Index(rest[pathStart:], "</path>")
		if pathEnd < 0 {
			builder.WriteString(rest[pathStart:])
			break
		}
		pathEnd += pathStart
		filePath := strings.TrimSpace(rest[pathStart+len("<path>") : pathEnd])
		afterPath := rest[pathEnd+len("</path>"):]
		contentStart := strings.Index(afterPath, "<content>")
		if contentStart < 0 {
			builder.WriteString(rest[pathStart : pathEnd+len("</path>")])
			rest = afterPath
			continue
		}
		contentEnd := strings.Index(afterPath[contentStart:], "</content>")
		if contentEnd < 0 {
			builder.WriteString(rest[pathStart:])
			break
		}
		contentEnd += contentStart
		prefix := afterPath[:contentStart]
		content := afterPath[contentStart+len("<content>") : contentEnd]
		if strings.Contains(prefix, "<type>file</type>") {
			builder.WriteString(renderFileBlock(filePath, content))
		} else {
			builder.WriteString(rest[pathStart : pathEnd+len("</path>")])
			builder.WriteString(prefix)
			builder.WriteString("<content>")
			builder.WriteString(content)
			builder.WriteString("</content>")
		}
		rest = afterPath[contentEnd+len("</content>"):]
	}
	return builder.String()
}

func renderFileBlock(filePath string, content string) string {
	cleaned := stripNumberedCodeLines(content)
	if cleaned == "" {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n### 文件`" + filePath + "`\n\n")
	builder.WriteString("```" + codeFenceLanguage(filePath) + "\n")
	builder.WriteString(cleaned)
	if !strings.HasSuffix(cleaned, "\n") {
		builder.WriteString("\n")
	}
	builder.WriteString("```\n\n")
	return builder.String()
}

func stripNumberedCodeLines(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "(") && strings.Contains(trimmed, "file") && strings.Contains(trimmed, "line") {
			continue
		}
		cleaned = append(cleaned, stripLineNumberPrefix(line))
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func stripLineNumberPrefix(line string) string {
	trimmedLeft := strings.TrimLeft(line, " \t")
	index := 0
	for index < len(trimmedLeft) && trimmedLeft[index] >= '0' && trimmedLeft[index] <= '9' {
		index++
	}
	if index > 0 && index < len(trimmedLeft) && trimmedLeft[index] == ':' {
		trimmedLeft = trimmedLeft[index+1:]
		if strings.HasPrefix(trimmedLeft, " ") {
			trimmedLeft = trimmedLeft[1:]
		}
		return trimmedLeft
	}
	return line
}

func stripStandaloneContentTags(text string) string {
	replacer := strings.NewReplacer(
		"<type>file</type>", "",
		"<content>", "",
		"</content>", "",
	)
	return replacer.Replace(text)
}

func codeFenceLanguage(filePath string) string {
	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".js"):
		return "javascript"
	case strings.HasSuffix(lower, ".ts"):
		return "typescript"
	case strings.HasSuffix(lower, ".go"):
		return "go"
	case strings.HasSuffix(lower, ".c"), strings.HasSuffix(lower, ".h"):
		return "c"
	case strings.HasSuffix(lower, ".cpp"), strings.HasSuffix(lower, ".cc"), strings.HasSuffix(lower, ".hpp"):
		return "cpp"
	case strings.HasSuffix(lower, ".sh"):
		return "bash"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	case strings.HasSuffix(lower, ".txt"), strings.HasSuffix(lower, ".log"):
		return "text"
	default:
		return ""
	}
}

func safeMarkdownFilename(name string) string {
	return safeMarkdownStem(name, 120) + ".md"
}

func safeWriteupFilename(name string) string {
	stem := safeMarkdownStem(name, 120)
	if strings.HasSuffix(strings.ToLower(stem), "_wp") {
		return stem + ".md"
	}
	return safeMarkdownStem(stem, 117) + "_wp.md"
}

func safeMarkdownStem(name string, maxRunes int) string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer(
		"<", "_",
		">", "_",
		":", "_",
		"\"", "_",
		"/", "_",
		"\\", "_",
		"|", "_",
		"?", "_",
		"*", "_",
		"\r", "_",
		"\n", "_",
		"\t", "_",
	)
	name = replacer.Replace(name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "writeup"
	}
	if maxRunes <= 0 {
		maxRunes = 120
	}
	if len([]rune(name)) > maxRunes {
		runes := []rune(name)
		name = strings.Trim(runeSliceString(runes, maxRunes), " .")
		if name == "" {
			name = "writeup"
		}
	}
	return name
}

func runeSliceString(values []rune, limit int) string {
	if limit > len(values) {
		limit = len(values)
	}
	return string(values[:limit])
}

func trimWriteupLogs(logs string) string {
	const maxChars = 60000
	logs = strings.TrimSpace(logs)
	if len(logs) <= maxChars {
		return logs
	}
	return logs[:20000] + "\n\n...[日志过长，中间已省略]...\n\n" + logs[len(logs)-40000:]
}

func cleanWriteupLogs(logs string) string {
	logs = strings.ReplaceAll(logs, "\r\n", "\n")
	logs = stripGeneratedWriteupBlocks(logs)
	if extracted := extractReadableOutput(logs); extracted != "" {
		return trimWriteupLogs(extracted)
	}
	filtered := filterPlatformLogs(logs)
	if looksLikePromptEcho(filtered) {
		return ""
	}
	return trimWriteupLogs(filtered)
}

func stripGeneratedWriteupBlocks(logs string) string {
	for {
		begin := strings.Index(logs, generatedWriteupBeginMarker)
		if begin < 0 {
			return logs
		}
		afterBegin := begin + len(generatedWriteupBeginMarker)
		relativeEnd := strings.Index(logs[afterBegin:], generatedWriteupEndMarker)
		if relativeEnd < 0 {
			return logs[:begin]
		}
		end := afterBegin + relativeEnd + len(generatedWriteupEndMarker)
		logs = logs[:begin] + logs[end:]
	}
}

func extractReadableOutput(logs string) string {
	index := strings.LastIndex(logs, finalReadableOutputMarker)
	if index >= 0 {
		return cleanReadableOutput(logs[index+len(finalReadableOutputMarker):])
	}
	return ""
}

func cleanReadableOutput(text string) string {
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	skipSkillBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Final: opencode bridge completed") ||
			strings.HasPrefix(trimmed, "[runner]") ||
			strings.HasPrefix(trimmed, "[dispatcher]") ||
			strings.HasPrefix(trimmed, "[opencode]") {
			continue
		}
		if strings.HasPrefix(trimmed, "Active CTF skills:") || strings.HasPrefix(trimmed, "--- skill:") {
			skipSkillBlock = true
			continue
		}
		if skipSkillBlock {
			if strings.HasPrefix(trimmed, "Attachments are mounted") ||
				strings.HasPrefix(trimmed, "Attachment files:") ||
				strings.HasPrefix(trimmed, "**") ||
				strings.HasPrefix(trimmed, "<path>") {
				skipSkillBlock = false
			} else {
				continue
			}
		}
		if strings.Contains(trimmed, "[skill truncated]") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	result := strings.TrimSpace(strings.Join(cleaned, "\n"))
	if looksLikePromptEcho(result) {
		return ""
	}
	return result
}

func looksLikePromptEcho(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	markers := 0
	for _, marker := range []string{
		"active ctf skills:",
		"attachments are mounted read-only at /attachments",
		"# ctf miscellaneous",
		"quick reference for miscellaneous ctf challenges",
		"allowed-tools:",
		"compatibility:",
		"license: mit",
		"manual install:",
		"quick start commands",
		"[skill truncated]",
		"you are solving a ctf challenge",
		"prompt要求opencode最终严格输出",
		"final output contract",
		"flag output contract",
		"strictly output",
		"marker block",
		"最终严格输出",
		"按以下两行",
		"格式收尾",
		"这道题目已经解出",
	} {
		if strings.Contains(normalized, marker) {
			markers++
		}
	}
	return markers >= 2
}

func filterPlatformLogs(logs string) string {
	lines := strings.Split(logs, "\n")
	cleaned := make([]string, 0, len(lines))
	skipSkillBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[dispatcher]") ||
			strings.HasPrefix(trimmed, "[runner]") ||
			strings.HasPrefix(trimmed, "[opencode]") ||
			strings.HasPrefix(trimmed, "Action: start opencode") ||
			strings.HasPrefix(trimmed, "Action: send prompt through OpenCode") ||
			strings.HasPrefix(trimmed, "Observation: OpenCode") ||
			strings.HasPrefix(trimmed, "Observation: loaded CTF skill") ||
			strings.HasPrefix(trimmed, "Observation: assistant output updated:") ||
			strings.HasPrefix(trimmed, "Observation: final readable OpenCode output:") ||
			strings.HasPrefix(trimmed, "Thought: wrote OpenCode provider config") ||
			strings.HasPrefix(trimmed, "\x1b[36m[agent]") {
			continue
		}
		if strings.HasPrefix(trimmed, "Active CTF skills:") || strings.HasPrefix(trimmed, "--- skill:") {
			skipSkillBlock = true
			continue
		}
		if skipSkillBlock {
			if strings.HasPrefix(trimmed, "Attachments are mounted") ||
				strings.HasPrefix(trimmed, "Attachment files:") ||
				strings.HasPrefix(trimmed, "<path>") ||
				strings.HasPrefix(trimmed, "**") {
				skipSkillBlock = false
			} else {
				continue
			}
		}
		if strings.Contains(trimmed, "[skill truncated]") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
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

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
