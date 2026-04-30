package app

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type LogSink func(string)

type DockerResult struct {
	ExitCode      int
	ContainerName string
	Retained      bool
}

type DockerContainer struct {
	Name    string
	Image   string
	Status  string
	Ports   string
	Running bool
}

func ListDockerContainers() (map[string]DockerContainer, error) {
	output, err := exec.Command(
		"docker",
		"ps",
		"-a",
		"--filter",
		"name=ctf-agent-",
		"--format",
		"{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}",
	).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	containers := map[string]DockerContainer{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		status := strings.TrimSpace(parts[2])
		containers[name] = DockerContainer{
			Name:    name,
			Image:   strings.TrimSpace(parts[1]),
			Status:  status,
			Ports:   strings.TrimSpace(parts[3]),
			Running: strings.HasPrefix(strings.ToLower(status), "up "),
		}
	}
	return containers, nil
}

func RunDockerTask(cfg Config, task *Task, logSink LogSink, containerSink func(string)) (DockerResult, error) {
	if cfg.AgentScript == "" {
		return DockerResult{ExitCode: 2}, errors.New("CTF_AGENT_AGENT_SCRIPT is empty")
	}
	if cfg.SkillsDir == "" {
		return DockerResult{ExitCode: 2}, errors.New("CTF_AGENT_SKILLS_DIR is empty")
	}
	ctx := context.Background()

	containerName := "ctf-agent-" + task.ID
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
	args := []string{
		"run", "-d",
		"--name", containerName,
		"--memory", cfg.MemLimit,
		"--cpus", cfg.CPUs,
		"--pids-limit", cfg.PidsLimit,
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"-w", "/workspace",
	}
	args = append(args,
		"-v", dockerMount(cfg.AgentScript)+":/opt/ctf_agent/agent.py:ro",
		"-v", dockerMount(cfg.SkillsDir)+":/skills:ro",
		"-v", dockerMount(task.AttachmentsDir)+":/attachments:ro",
		"-e", "CHALLENGE_NAME="+task.Name,
		"-e", "CHALLENGE_TYPE="+task.Category,
		"-e", "CHALLENGE_DESC="+task.Description,
		"-e", "TARGET_IP="+task.TargetIP,
		"-e", "ATTACHMENT_DIR=/attachments",
		"-e", "CTF_AGENT_SKILLS_DIR=/skills",
		"-e", "CTF_AGENT_SKILL_IDS="+normalizeCategory(task.Category),
		"-e", "PYTHONUNBUFFERED=1",
	)
	if cfg.DisableNetwork {
		args = append(args, "--network", "none")
	}
	args = append(args, cfg.ImageForCategory(task.Category), "tail", "-f", "/dev/null")

	logSink("[runner] starting container image=" + cfg.ImageForCategory(task.Category) +
		" mem_limit=" + cfg.MemLimit + " cpus=" + cfg.CPUs + "\n")
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		return DockerResult{ExitCode: 2, ContainerName: containerName}, errors.New(string(output))
	}
	logSink("[runner] container_name=" + containerName + "\n")
	if containerSink != nil {
		containerSink(containerName)
	}
	result, err := runAgentInContainer(ctx, containerName, openCodeProviderEnv(cfg), logSink)
	result.ContainerName = containerName
	if err != nil {
		return result, err
	}
	if result.ExitCode == 0 {
		result.Retained = true
		return result, nil
	}
	result.Retained = true
	logSink("[runner] failed container retained for hints container_name=" + containerName + "\n")
	return result, nil
}

func RunDockerHint(cfg Config, task *Task, hint string, logSink LogSink) (DockerResult, error) {
	if task.ContainerName == "" || !task.ContainerKept {
		return DockerResult{ExitCode: 2}, errors.New("no retained container for this task")
	}
	if !isManagedContainerName(task.ContainerName) {
		return DockerResult{ExitCode: 2}, errors.New("refusing unmanaged container name")
	}
	ctx := context.Background()
	logSink("[runner] continuing retained container_name=" + task.ContainerName + "\n")
	env := append(openCodeProviderEnv(cfg), "CTF_AGENT_USER_HINT="+hint)
	if task.OpenCodeSession != "" {
		env = append(env, "OPENCODE_SESSION_ID="+task.OpenCodeSession)
	}
	result, err := runAgentInContainer(ctx, task.ContainerName, env, logSink)
	if err != nil {
		return result, err
	}
	if result.ExitCode == 0 {
		result.Retained = true
		return result, nil
	}
	result.Retained = true
	logSink("[runner] retained container still available for another hint container_name=" + task.ContainerName + "\n")
	return result, nil
}

func CloseTaskContainer(containerName string) error {
	if strings.TrimSpace(containerName) == "" {
		return errors.New("container name is empty")
	}
	if !isManagedContainerName(containerName) {
		return errors.New("refusing unmanaged container name")
	}
	output, err := exec.Command("docker", "rm", "-f", containerName).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(message), "no such container") {
			return nil
		}
		if message == "" {
			message = err.Error()
		}
		return errors.New(message)
	}
	return nil
}

func openCodeProviderEnv(cfg Config) []string {
	return []string{
		"OPENCODE_PROVIDER_ID=" + cfg.OpenCodeProviderID,
		"OPENCODE_PROVIDER_NAME=" + cfg.OpenCodeProviderName,
		"OPENCODE_PROVIDER_NPM=" + cfg.OpenCodeProviderNPM,
		"OPENCODE_BASE_URL=" + cfg.OpenCodeBaseURL,
		"OPENCODE_API_KEY=" + cfg.OpenCodeAPIKey,
		"OPENCODE_MODEL=" + cfg.OpenCodeModel,
	}
}

func runAgentInContainer(ctx context.Context, containerName string, env []string, logSink LogSink) (DockerResult, error) {
	args := []string{"exec", "-w", "/workspace"}
	for _, item := range env {
		args = append(args, "-e", item)
	}
	args = append(args, containerName, "python", "/opt/ctf_agent/agent.py")
	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return DockerResult{ExitCode: 2, ContainerName: containerName}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return DockerResult{ExitCode: 2, ContainerName: containerName}, err
	}
	if err := cmd.Start(); err != nil {
		return DockerResult{ExitCode: 2, ContainerName: containerName}, err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go streamPipe(&wg, stdout, logSink)
	go streamPipe(&wg, stderr, logSink)
	err = cmd.Wait()
	wg.Wait()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logSink("[runner] agent exited with status=" + strconvItoa(exitErr.ExitCode()) + "\n")
			return DockerResult{ExitCode: exitErr.ExitCode(), ContainerName: containerName}, nil
		}
		return DockerResult{ExitCode: 2, ContainerName: containerName}, err
	}
	logSink("[runner] agent exited with status=0\n")
	return DockerResult{ExitCode: 0, ContainerName: containerName}, nil
}

func streamPipe(wg *sync.WaitGroup, reader io.Reader, logSink LogSink) {
	defer wg.Done()
	buffer := make([]byte, 4096)
	bufReader := bufio.NewReader(reader)
	for {
		n, err := bufReader.Read(buffer)
		if n > 0 {
			logSink(string(buffer[:n]))
		}
		if err != nil {
			return
		}
	}
}

func dockerMount(path string) string {
	cleaned := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(cleaned, "\\", "/")
	}
	return cleaned
}

func isManagedContainerName(name string) bool {
	if !strings.HasPrefix(name, "ctf-agent-") {
		return false
	}
	id := strings.TrimPrefix(name, "ctf-agent-")
	return isSafeTaskID(id)
}
