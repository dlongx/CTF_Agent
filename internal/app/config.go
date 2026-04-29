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
	DisableNetwork         bool
	AgentScript            string
	SkillsDir              string
	OpenCodeProviderFormat string
	OpenCodeProviders      map[string]OpenCodeProviderConfig
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

const (
	ProviderFormatOpenAICompatible = "openai-compatible"
	ProviderFormatAnthropic        = "anthropic"
)

type OpenCodeProviderConfig struct {
	Format       string
	Label        string
	ProviderID   string
	ProviderName string
	ProviderNPM  string
	BaseURL      string
	APIKey       string
	Model        string
}

func LoadConfig() Config {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}
	dataDir := absPath(root, getenv("CTF_AGENT_DATA_DIR", "data"))
	defaultImage := getenv("CTF_AGENT_DOCKER_IMAGE", "ctf-agent-opencode:latest")
	providers := loadOpenCodeProviders()
	providerFormat := defaultProviderFormat()
	activeProvider := providers[providerFormat]
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
		DisableNetwork:         getenvBool("CTF_AGENT_DISABLE_NETWORK", false),
		AgentScript:            absPath(root, getenv("CTF_AGENT_AGENT_SCRIPT", filepath.Join("runtime", "opencode", "bridge.py"))),
		SkillsDir:              absPath(root, getenv("CTF_AGENT_SKILLS_DIR", filepath.Join("runtime", "opencode", "skills"))),
		OpenCodeProviderFormat: providerFormat,
		OpenCodeProviders:      providers,
		OpenCodeProviderID:     activeProvider.ProviderID,
		OpenCodeProviderName:   activeProvider.ProviderName,
		OpenCodeProviderNPM:    activeProvider.ProviderNPM,
		OpenCodeBaseURL:        activeProvider.BaseURL,
		OpenCodeAPIKey:         activeProvider.APIKey,
		OpenCodeModel:          activeProvider.Model,
		OpenCodeWebEnabled:     getenvBool("CTF_AGENT_OPENCODE_WEB_ENABLED", true),
		OpenCodeWebBindIP:      getenv("CTF_AGENT_OPENCODE_BIND_IP", "127.0.0.1"),
		OpenCodeWebPublicBase:  getenv("CTF_AGENT_OPENCODE_PUBLIC_BASE_URL", "http://127.0.0.1"),
		OpenCodeServerPassword: strings.TrimSpace(os.Getenv("OPENCODE_SERVER_PASSWORD")),
	}
}

func (c Config) withOpenCodeProviderDefaults() Config {
	if c.OpenCodeProviders == nil {
		c.OpenCodeProviders = loadOpenCodeProviders()
	}
	if strings.TrimSpace(c.OpenCodeProviderFormat) == "" {
		c.OpenCodeProviderFormat = defaultProviderFormat()
	}
	if provider, ok := c.ProviderForFormat(c.OpenCodeProviderFormat); ok {
		return c.WithOpenCodeProvider(provider)
	}
	return c
}

func (c Config) ProviderForFormat(format string) (OpenCodeProviderConfig, bool) {
	normalized, ok := canonicalProviderFormat(format)
	if !ok {
		return OpenCodeProviderConfig{}, false
	}
	provider, ok := c.OpenCodeProviders[normalized]
	return provider, ok
}

func (c Config) WithOpenCodeProvider(provider OpenCodeProviderConfig) Config {
	copy := c
	copy.OpenCodeProviderFormat = provider.Format
	copy.OpenCodeProviderID = provider.ProviderID
	copy.OpenCodeProviderName = provider.ProviderName
	copy.OpenCodeProviderNPM = provider.ProviderNPM
	copy.OpenCodeBaseURL = provider.BaseURL
	copy.OpenCodeAPIKey = provider.APIKey
	copy.OpenCodeModel = provider.Model
	return copy
}

func (p OpenCodeProviderConfig) IsConfigured() bool {
	return strings.TrimSpace(p.ProviderID) != "" &&
		strings.TrimSpace(p.ProviderNPM) != "" &&
		strings.TrimSpace(p.BaseURL) != "" &&
		strings.TrimSpace(p.APIKey) != "" &&
		strings.TrimSpace(p.Model) != ""
}

func (c Config) ImageForCategory(category string) string {
	if image, ok := c.CategoryImages[normalizeCategory(category)]; ok {
		return image
	}
	return c.DockerImage
}

func loadOpenCodeProviders() map[string]OpenCodeProviderConfig {
	legacyAnthropic := strings.EqualFold(strings.TrimSpace(os.Getenv("OPENCODE_PROVIDER_NPM")), "@ai-sdk/anthropic")
	anthropicAPIKey := strings.TrimSpace(os.Getenv("OPENCODE_ANTHROPIC_API_KEY"))
	anthropicModel := strings.TrimSpace(os.Getenv("OPENCODE_ANTHROPIC_MODEL"))
	if legacyAnthropic && anthropicAPIKey == "" {
		anthropicAPIKey = strings.TrimSpace(os.Getenv("OPENCODE_API_KEY"))
	}
	if legacyAnthropic && anthropicModel == "" {
		anthropicModel = strings.TrimSpace(os.Getenv("OPENCODE_MODEL"))
	}
	openAI := OpenCodeProviderConfig{
		Format:       ProviderFormatOpenAICompatible,
		Label:        "OpenAI兼容",
		ProviderID:   getenv("OPENCODE_OPENAI_PROVIDER_ID", getenv("OPENCODE_PROVIDER_ID", "ctf")),
		ProviderName: getenv("OPENCODE_OPENAI_PROVIDER_NAME", getenv("OPENCODE_PROVIDER_NAME", "CTF Model Gateway")),
		ProviderNPM:  getenv("OPENCODE_OPENAI_PROVIDER_NPM", getenv("OPENCODE_PROVIDER_NPM", "@ai-sdk/openai-compatible")),
		BaseURL:      strings.TrimSpace(getenv("OPENCODE_OPENAI_BASE_URL", os.Getenv("OPENCODE_BASE_URL"))),
		APIKey:       strings.TrimSpace(getenv("OPENCODE_OPENAI_API_KEY", os.Getenv("OPENCODE_API_KEY"))),
		Model:        strings.TrimSpace(getenv("OPENCODE_OPENAI_MODEL", os.Getenv("OPENCODE_MODEL"))),
	}
	anthropic := OpenCodeProviderConfig{
		Format:       ProviderFormatAnthropic,
		Label:        "Anthropic",
		ProviderID:   getenv("OPENCODE_ANTHROPIC_PROVIDER_ID", "anthropic"),
		ProviderName: getenv("OPENCODE_ANTHROPIC_PROVIDER_NAME", "Anthropic"),
		ProviderNPM:  getenv("OPENCODE_ANTHROPIC_PROVIDER_NPM", "@ai-sdk/anthropic"),
		BaseURL:      getenv("OPENCODE_ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"),
		APIKey:       anthropicAPIKey,
		Model:        anthropicModel,
	}
	return map[string]OpenCodeProviderConfig{
		openAI.Format:    openAI,
		anthropic.Format: anthropic,
	}
}

func defaultProviderFormat() string {
	if value := strings.TrimSpace(os.Getenv("OPENCODE_PROVIDER_FORMAT")); value != "" {
		return normalizeProviderFormat(value)
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OPENCODE_PROVIDER_NPM")), "@ai-sdk/anthropic") {
		return ProviderFormatAnthropic
	}
	return ProviderFormatOpenAICompatible
}

func normalizeProviderFormat(format string) string {
	if normalized, ok := canonicalProviderFormat(format); ok {
		return normalized
	}
	return ProviderFormatOpenAICompatible
}

func canonicalProviderFormat(format string) (string, bool) {
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "anthropic", "claude":
		return ProviderFormatAnthropic, true
	case "openai", "openai-compatible", "compatible", "openai_compatible":
		return ProviderFormatOpenAICompatible, true
	default:
		return "", false
	}
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
