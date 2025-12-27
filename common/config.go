package common

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
)

type Config struct {
	ListenAddr               string   `json:"listen_addr"`
	MinInputTokens           int      `json:"min_input_tokens"`
	MaxInputTokens           int      `json:"max_input_tokens"`
	MaxOutputTokens          int      `json:"max_output_tokens"`
	RedisAddr                string   `json:"redis_addr"`
	RedisPassword            string   `json:"redis_password"`
	RedisPrefix              string   `json:"redis_prefix"`
	EnabledProcessors        []string `json:"enabled_processors"`
	EnabledModels            []string `json:"enabled_models"`
	DefaultModel             string   `json:"default_model"`
	UnsplashAPIAccessKey     string   `json:"unsplash_api_access_key"`
	UnsplashAPISecretKey     string   `json:"unsplash_api_secret_key"`
	MockResponse             bool     `json:"mock_response"`
	PostProcessMockResponses bool     `json:"post_process_mock_responses"`
	MockContent              string   `json:"mock_content"`
	SaveResponses            bool     `json:"save_responses"`

	ApiKey       string `json:"api_key"`
	ApiKeySecret string `json:"api_key_secret"`

	ApiFrontendKey string `json:"api_frontend_key"`

	enabledProcessorsMap map[string]struct{}
	enabledModelsMap     map[string]struct{}
}

func LoadConfig(dir string) (*Config, error) {
	cfg := DefaultConfig()

	// Load config (JSON + env overrides)
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = DEFAULT_CONFIG_FILE
	}

	if !strings.HasPrefix(configPath, "/") && dir != "" {
		configPath = path.Join(dir, configPath)
	}

	if _, err := os.Stat(configPath); err == nil {
		fileCfg, err := LoadConfigFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
		cfg.applyConfigOverrides(fileCfg)
	}

	cfg.applyEnvOverrides()

	cfg.updateMaps()

	return cfg, nil
}

func LoadConfigFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		f, err := os.Open(path)
		if err == nil {
			defer f.Close()
			dec := json.NewDecoder(f)
			_ = dec.Decode(&cfg) // ignore error, fallback to env/defaults
		}
	}
	// cfg.applyEnvOverrides()
	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		ApiKey:                   "",
		ApiKeySecret:             "",
		ApiFrontendKey:           "",
		MinInputTokens:           DEFAULT_MIN_INPUT_TOKENS,
		MaxInputTokens:           DEFAULT_MAX_INPUT_TOKENS,
		MaxOutputTokens:          DEFAULT_MAX_OUTPUT_TOKENS,
		RedisAddr:                DEFAULT_REDIS_ADDR,
		RedisPassword:            "",
		RedisPrefix:              DEFAULT_REDIS_PREFIX,
		ListenAddr:               DEFAULT_LISTEN_ADDR,
		EnabledModels:            strings.Split(DEFAULT_ENABLED_MODELS, ","),
		DefaultModel:             DEFAULT_MODEL,
		UnsplashAPIAccessKey:     "",
		UnsplashAPISecretKey:     "",
		MockResponse:             false,
		PostProcessMockResponses: false,
		MockContent:              "",
		SaveResponses:            false,
	}
}

func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("API_KEY"); v != "" {
		c.ApiKey = v
	}
	if v := os.Getenv("API_KEY_SECRET"); v != "" {
		c.ApiKeySecret = v
	}
	if v := os.Getenv("API_FRONTEND_KEY"); v != "" {
		c.ApiFrontendKey = v
	}
	if v := os.Getenv("MIN_INPUT_TOKENS"); v != "" {
		c.MinInputTokens = atoiOrDefault(v, c.MinInputTokens)
	}
	if v := os.Getenv("MAX_INPUT_TOKENS"); v != "" {
		c.MaxInputTokens = atoiOrDefault(v, c.MaxInputTokens)
	}
	if v := os.Getenv("MAX_OUTPUT_TOKENS"); v != "" {
		c.MaxOutputTokens = atoiOrDefault(v, c.MaxOutputTokens)
	}
	if v := os.Getenv("REDIS_ADDR"); v != "" {
		c.RedisAddr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.RedisPassword = v
	}
	if v := os.Getenv("REDIS_PREFIX"); v != "" {
		c.RedisPrefix = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		c.ListenAddr = v
	}
	if v := os.Getenv("ENABLED_PROCESSORS"); v != "" {
		c.EnabledProcessors = strings.Split(v, ",")
	}
	if v := os.Getenv("ENABLED_MODELS"); v != "" {
		c.EnabledModels = strings.Split(v, ",")
	}
	if v := os.Getenv("DEFAULT_MODEL"); v != "" {
		c.DefaultModel = v
	}
	if v := os.Getenv("UNSPLASH_API_ACCESS_KEY"); v != "" {
		c.UnsplashAPIAccessKey = v
	}
	if v := os.Getenv("UNSPLASH_API_SECRET_KEY"); v != "" {
		c.UnsplashAPISecretKey = v
	}
	if v := os.Getenv("MOCK_RESPONSE"); v != "" {
		c.MockResponse = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("MOCK_CONTENT"); v != "" {
		c.MockContent = v
	}
	if v := os.Getenv("SAVE_RESPONSES"); v != "" {
		c.SaveResponses = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv("POST_PROCESS_MOCK_RESPONSES"); v != "" {
		c.PostProcessMockResponses = strings.ToLower(v) == "true" || v == "1"
	}
}

func (c *Config) applyConfigOverrides(cfg *Config) {
	if cfg.ApiKey != "" {
		c.ApiKey = cfg.ApiKey
	}
	if cfg.ApiKeySecret != "" {
		c.ApiKeySecret = cfg.ApiKeySecret
	}
	if cfg.ApiFrontendKey != "" {
		c.ApiFrontendKey = cfg.ApiFrontendKey
	}
	if cfg.MinInputTokens != 0 {
		c.MinInputTokens = cfg.MinInputTokens
	}
	if cfg.MaxInputTokens != 0 {
		c.MaxInputTokens = cfg.MaxInputTokens
	}
	if cfg.MaxOutputTokens != 0 {
		c.MaxOutputTokens = cfg.MaxOutputTokens
	}
	if cfg.RedisAddr != "" {
		c.RedisAddr = cfg.RedisAddr
	}
	if cfg.RedisPassword != "" {
		c.RedisPassword = cfg.RedisPassword
	}
	if cfg.RedisPrefix != "" {
		c.RedisPrefix = cfg.RedisPrefix
	}
	if cfg.ListenAddr != "" {
		c.ListenAddr = cfg.ListenAddr
	}
	if len(cfg.EnabledModels) > 0 {
		c.EnabledModels = cfg.EnabledModels
	}
	if len(cfg.EnabledProcessors) > 0 {
		c.EnabledProcessors = cfg.EnabledProcessors
	}
	if cfg.DefaultModel != "" {
		c.DefaultModel = cfg.DefaultModel
	}
	if cfg.UnsplashAPIAccessKey != "" {
		c.UnsplashAPIAccessKey = cfg.UnsplashAPIAccessKey
	}
	if cfg.UnsplashAPISecretKey != "" {
		c.UnsplashAPISecretKey = cfg.UnsplashAPISecretKey
	}
	c.MockResponse = cfg.MockResponse
	c.PostProcessMockResponses = cfg.PostProcessMockResponses
	if cfg.MockContent != "" {
		c.MockContent = cfg.MockContent
	}
	c.SaveResponses = cfg.SaveResponses
}

func (c *Config) updateMaps() {
	c.enabledProcessorsMap = make(map[string]struct{})
	for _, p := range c.EnabledProcessors {
		c.enabledProcessorsMap[p] = struct{}{}
	}

	c.enabledModelsMap = make(map[string]struct{})
	for _, m := range c.EnabledModels {
		c.enabledModelsMap[m] = struct{}{}
	}
}

func atoiOrDefault(s string, def int) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return def
	}
	return n
}

func (c *Config) IsProcessorEnabled(name string) bool {
	_, ok := c.enabledProcessorsMap[name]
	return ok
}

func (c *Config) IsModelEnabled(name string) bool {
	_, ok := c.enabledModelsMap[name]
	return ok
}

func (c *Config) GetDefaultModel() (string, bool) {
	defaultModel := c.DefaultModel

	if !c.IsModelEnabled(defaultModel) {
		slog.Warn("Default model not in enabled models, returning false", "default_model", defaultModel)
		return "", false
	}

	return defaultModel, true
}
