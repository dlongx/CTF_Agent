package app

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Addr                   string
	DataDir                string
	ChallengeDir           string
	DockerImage            string
	CategoryImages         map[string]string
	MemLimit               string
	CPUs                   string
	MaxContainers          int
	PidsLimit              string
	TaskTimeoutSeconds     int
	DisableNetwork         bool
	AgentScript            string
	SkillsDir              string
	OpenCodeTimeoutSeconds string
	OpenCodeProviderID     string
	OpenCodeProviderName   string
	OpenCodeProviderNPM    string
	OpenCodeBaseURL        string
	OpenCodeAPIKey         string
	OpenCodeModel          string
	OpenCodeWebEnabled     bool
	OpenCodeWebBindIP      string
	OpenCodeWebPublicBase  string
	OpenCodeServerPassword string
}

func LoadConfig() Config {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	dataDir := absPath(root, getenv("CTF_AGENT_DATA_DIR", "data"))
	defaultImage := getenv("CTF_AGENT_DOCKER_IMAGE", "ctf-agent-opencode:latest")
	openCodeTimeoutSeconds := getenvInt("OPENCODE_TIMEOUT_SECONDS", 180)
	return Config{
		Addr:                   getenv("CTF_AGENT_GO_ADDR", "127.0.0.1:8000"),
		DataDir:                dataDir,
		ChallengeDir:           filepath.Join(dataDir, "challenges"),
		DockerImage:            defaultImage,
		CategoryImages:         loadCategoryImages(defaultImage),
		MemLimit:               getenv("CTF_AGENT_MEM_LIMIT", "512m"),
		CPUs:                   getenv("CTF_AGENT_CPUS", "1.0"),
		MaxContainers:          getenvInt("CTF_AGENT_MAX_CONTAINERS", 4),
		PidsLimit:              getenv("CTF_AGENT_PIDS_LIMIT", "1024"),
		TaskTimeoutSeconds:     taskTimeoutSeconds(openCodeTimeoutSeconds),
		DisableNetwork:         getenvBool("CTF_AGENT_DISABLE_NETWORK", false),
		AgentScript:            absPath(root, getenv("CTF_AGENT_AGENT_SCRIPT", filepath.Join("runtime", "opencode", "bridge.py"))),
		SkillsDir:              absPath(root, getenv("CTF_AGENT_SKILLS_DIR", filepath.Join("runtime", "opencode", "skills"))),
		OpenCodeTimeoutSeconds: strconv.Itoa(openCodeTimeoutSeconds),
		OpenCodeProviderID:     strings.TrimSpace(os.Getenv("OPENCODE_PROVIDER_ID")),
		OpenCodeProviderName:   getenv("OPENCODE_PROVIDER_NAME", "CTF Model Gateway"),
		OpenCodeProviderNPM:    getenv("OPENCODE_PROVIDER_NPM", "@ai-sdk/openai-compatible"),
		OpenCodeBaseURL:        strings.TrimSpace(os.Getenv("OPENCODE_BASE_URL")),
		OpenCodeAPIKey:         strings.TrimSpace(os.Getenv("OPENCODE_API_KEY")),
		OpenCodeModel:          strings.TrimSpace(os.Getenv("OPENCODE_MODEL")),
		OpenCodeWebEnabled:     getenvBool("CTF_AGENT_OPENCODE_WEB_ENABLED", true),
		OpenCodeWebBindIP:      getenv("CTF_AGENT_OPENCODE_BIND_IP", "127.0.0.1"),
		OpenCodeWebPublicBase:  getenv("CTF_AGENT_OPENCODE_PUBLIC_BASE_URL", "http://127.0.0.1"),
		OpenCodeServerPassword: strings.TrimSpace(os.Getenv("OPENCODE_SERVER_PASSWORD")),
	}
}

func taskTimeoutSeconds(openCodeTimeoutSeconds int) int {
	minimum := openCodeTimeoutSeconds + 60
	configured := getenvInt("CTF_AGENT_TASK_TIMEOUT", 0)
	if configured < minimum {
		return minimum
	}
	return configured
}

func (c Config) ImageForCategory(category string) string {
	if image, ok := c.CategoryImages[normalizeCategory(category)]; ok {
		return image
	}
	return c.DockerImage
}

func (c Config) OpenCodeWebURL(hostPort string) string {
	hostPort = strings.TrimSpace(hostPort)
	if hostPort == "" {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(c.OpenCodeWebPublicBase), "/")
	if base == "" {
		base = "http://127.0.0.1"
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	return base + ":" + hostPort
}

func (c Config) OpenCodeSessionURL(hostPort string, sessionID string) string {
	base := c.OpenCodeWebURL(hostPort)
	sessionID = strings.TrimSpace(sessionID)
	if base == "" || sessionID == "" {
		return base
	}
	workspaceSlug := base64.RawURLEncoding.EncodeToString([]byte("/workspace"))
	return base + "/" + workspaceSlug + "/session/" + sessionID
}

func getenv(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getenvBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func absPath(root string, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(root, value)
}

func normalizeCategory(category string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(category)), "-", "_")
}

func loadCategoryImages(defaultImage string) map[string]string {
	images := map[string]string{}
	for _, category := range []string{"misc", "web", "pwn", "crypto", "reverse", "forensics"} {
		if image := strings.TrimSpace(os.Getenv("CTF_AGENT_IMAGE_" + strings.ToUpper(category))); image != "" {
			images[category] = image
		}
	}
	for _, item := range strings.Split(os.Getenv("CTF_AGENT_CATEGORY_IMAGES"), ",") {
		category, image, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(image) == "" {
			continue
		}
		images[normalizeCategory(category)] = strings.TrimSpace(image)
	}
	if len(images) > 0 {
		if _, ok := images["misc"]; !ok {
			images["misc"] = defaultImage
		}
	}
	return images
}
