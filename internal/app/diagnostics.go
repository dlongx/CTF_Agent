package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type readinessCheck struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

type readinessResponse struct {
	OK        bool                      `json:"ok"`
	Checks    map[string]readinessCheck `json:"checks"`
	CheckedAt time.Time                 `json:"checked_at"`
}

type providerTestResponse struct {
	Format    string    `json:"format"`
	OK        bool      `json:"ok"`
	LatencyMS int64     `json:"latency_ms"`
	ErrorCode string    `json:"error_code,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func (s *Service) Readiness(ctx context.Context) readinessResponse {
	checks := map[string]readinessCheck{}
	checks["data"] = checkWritableDirectory(s.cfg.ChallengeDir)
	checks["provider"] = readinessCheck{OK: true, Reason: "configured"}
	if _, err := s.activeOpenCodeProvider(); err != nil {
		checks["provider"] = readinessCheck{OK: false, Reason: "provider_not_configured"}
	}

	dockerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(dockerCtx, "docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		checks["docker"] = readinessCheck{OK: false, Reason: "docker_unavailable"}
	} else {
		checks["docker"] = readinessCheck{OK: true, Reason: "available"}
		for _, image := range configuredImages(s.cfg) {
			key := "image:" + image
			if err := exec.CommandContext(dockerCtx, "docker", "image", "inspect", image).Run(); err != nil {
				checks[key] = readinessCheck{OK: false, Reason: "image_missing"}
			} else {
				checks[key] = readinessCheck{OK: true, Reason: "available"}
			}
		}
	}

	response := readinessResponse{OK: true, Checks: checks, CheckedAt: time.Now().UTC()}
	for _, check := range checks {
		if !check.OK {
			response.OK = false
			break
		}
	}
	return response
}

func checkWritableDirectory(dir string) readinessCheck {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return readinessCheck{OK: false, Reason: "data_directory_unavailable"}
	}
	file, err := os.CreateTemp(dir, ".readiness-*")
	if err != nil {
		return readinessCheck{OK: false, Reason: "data_directory_not_writable"}
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return readinessCheck{OK: false, Reason: "data_directory_not_writable"}
	}
	if err := os.Remove(name); err != nil {
		return readinessCheck{OK: false, Reason: "data_directory_cleanup_failed"}
	}
	return readinessCheck{OK: true, Reason: "writable"}
}

func configuredImages(cfg Config) []string {
	unique := map[string]struct{}{}
	if image := strings.TrimSpace(cfg.DockerImage); image != "" {
		unique[image] = struct{}{}
	}
	for _, image := range cfg.CategoryImages {
		if image = strings.TrimSpace(image); image != "" {
			unique[image] = struct{}{}
		}
	}
	images := make([]string, 0, len(unique))
	for image := range unique {
		images = append(images, image)
	}
	sort.Strings(images)
	return images
}

func (s *Service) TestProvider(ctx context.Context, format string) providerTestResponse {
	started := time.Now()
	response := providerTestResponse{Format: normalizeProviderFormat(format), CheckedAt: started.UTC()}
	provider, ok := s.cfg.ProviderForFormat(format)
	if !ok {
		response.ErrorCode = "unsupported_provider"
		return response
	}
	response.Format = provider.Format
	if !provider.IsConfigured() {
		response.ErrorCode = "provider_not_configured"
		return response
	}

	testCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(testCtx, http.MethodGet, strings.TrimRight(provider.BaseURL, "/")+"/models", nil)
	if err != nil {
		response.ErrorCode = "invalid_provider_config"
		return response
	}
	if provider.Format == ProviderFormatAnthropic {
		req.Header.Set("x-api-key", provider.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	httpResponse, err := client.Do(req)
	response.LatencyMS = time.Since(started).Milliseconds()
	response.CheckedAt = time.Now().UTC()
	if err != nil {
		response.ErrorCode = providerNetworkErrorCode(testCtx, err)
		return response
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResponse.Body, 64<<10))
		response.ErrorCode = providerHTTPErrorCode(httpResponse.StatusCode)
		return response
	}
	var payload any
	decoder := json.NewDecoder(io.LimitReader(httpResponse.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		response.ErrorCode = "invalid_response"
		return response
	}
	response.OK = true
	return response
}

func providerNetworkErrorCode(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "network_error"
}

func providerHTTPErrorCode(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "unauthorized"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		return "upstream_http_" + strconv.Itoa(status)
	}
}
