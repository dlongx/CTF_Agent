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
	"time"
)

type LogSink func(string)

type DockerResult struct {
	ExitCode         int
	ContainerName    string
	Retained         bool
	OpenCodeHostPort string
	OpenCodeWebURL   string
}

type DockerEndpoint struct {
	ContainerName    string
	OpenCodeHostPort string
	OpenCodeWebURL   string
}

func RunDockerTask(cfg Config, task *Task, logSink LogSink, endpointSink func(DockerEndpoint)) (DockerResult, error) {
	if cfg.AgentScript == "" {
		return DockerResult{ExitCode: 2}, errors.New("CTF_AGENT_AGENT_SCRIPT is empty")
	}
	if cfg.SkillsDir == "" {
		return DockerResult{ExitCode: 2}, errors.New("CTF_AGENT_SKILLS_DIR is empty")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TaskTimeoutSeconds)*time.Second)
	defer cancel()

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
	if cfg.OpenCodeWebEnabled && !cfg.DisableNetwork {
		args = append(args, "-p", cfg.OpenCodeWebBindIP+"::4096")
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
		"-e", "OPENCODE_TIMEOUT_SECONDS="+cfg.OpenCodeTimeoutSeconds,
		"-e", "OPENCODE_PROVIDER_ID="+cfg.OpenCodeProviderID,
		"-e", "OPENCODE_PROVIDER_NAME="+cfg.OpenCodeProviderName,
		"-e", "OPENCODE_PROVIDER_NPM="+cfg.OpenCodeProviderNPM,
		"-e", "OPENCODE_BASE_URL="+cfg.OpenCodeBaseURL,
		"-e", "OPENCODE_API_KEY="+cfg.OpenCodeAPIKey,
		"-e", "OPENCODE_MODEL="+cfg.OpenCodeModel,
	)
	if cfg.OpenCodeServerPassword != "" {
		args = append(args, "-e", "OPENCODE_SERVER_PASSWORD="+cfg.OpenCodeServerPassword)
	}
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
	endpoint := inspectOpenCodeEndpoint(cfg, containerName, logSink)
	if endpointSink != nil {
		endpointSink(endpoint)
	}
	result, err := runAgentInContainer(ctx, containerName, nil, logSink)
	result.ContainerName = containerName
	result.OpenCodeHostPort = endpoint.OpenCodeHostPort
	result.OpenCodeWebURL = endpoint.OpenCodeWebURL
	if ctx.Err() == context.DeadlineExceeded {
		logSink("[runner] container timed out after " + strconvItoa(cfg.TaskTimeoutSeconds) + "s; retained for inspection\n")
		return DockerResult{
			ExitCode:         124,
			ContainerName:    containerName,
			Retained:         true,
			OpenCodeHostPort: endpoint.OpenCodeHostPort,
			OpenCodeWebURL:   endpoint.OpenCodeWebURL,
		}, nil
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TaskTimeoutSeconds)*time.Second)
	defer cancel()
	logSink("[runner] continuing retained container_name=" + task.ContainerName + "\n")
	result, err := runAgentInContainer(ctx, task.ContainerName, []string{
		"CTF_AGENT_USER_HINT=" + hint,
	}, logSink)
	if ctx.Err() == context.DeadlineExceeded {
		logSink("[runner] retained container timed out after " + strconvItoa(cfg.TaskTimeoutSeconds) + "s; still retained for inspection\n")
		return DockerResult{
			ExitCode:         124,
			ContainerName:    task.ContainerName,
			Retained:         true,
			OpenCodeHostPort: task.OpenCodeHostPort,
			OpenCodeWebURL:   task.OpenCodeWebURL,
		}, nil
	}
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
	output, err := exec.Command("docker", "rm", "-f", containerName).CombinedOutput()
	if err != nil {
		return errors.New(string(output))
	}
	return nil
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

func inspectOpenCodeEndpoint(cfg Config, containerName string, logSink LogSink) DockerEndpoint {
	endpoint := DockerEndpoint{ContainerName: containerName}
	if !cfg.OpenCodeWebEnabled || cfg.DisableNetwork {
		return endpoint
	}
	output, err := exec.Command("docker", "port", containerName, "4096/tcp").CombinedOutput()
	if err != nil {
		logSink("[runner] opencode web port inspect failed: " + strings.TrimSpace(string(output)) + "\n")
		return endpoint
	}
	hostPort := parseDockerHostPort(string(output))
	if hostPort == "" {
		logSink("[runner] opencode web port was not published\n")
		return endpoint
	}
	endpoint.OpenCodeHostPort = hostPort
	endpoint.OpenCodeWebURL = cfg.OpenCodeWebURL(hostPort)
	logSink("[runner] opencode web url=" + endpoint.OpenCodeWebURL + "\n")
	return endpoint
}

func parseDockerHostPort(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if index := strings.LastIndex(line, ":"); index >= 0 && index+1 < len(line) {
			return strings.TrimSpace(line[index+1:])
		}
	}
	return ""
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
