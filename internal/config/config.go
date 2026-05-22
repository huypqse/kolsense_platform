package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Provider represents an AI provider backend.
type Provider string

const (
	ProviderOllama    Provider = "ollama"
	ProviderDashScope Provider = "dashscope"
	ProviderGemini    Provider = "gemini"
)

// Config is the top-level application configuration.
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	Embedding EmbeddingConfig
	LLM       LLMConfig
	Parser    ParserConfig
	Search    SearchConfig
	Log       LogConfig
}

type ServerConfig struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	URL      string
	MaxConns int32
	MinConns int32
}

type EmbeddingConfig struct {
	Provider   Provider
	Dimensions int

	// Ollama
	OllamaBaseURL string
	OllamaModel   string

	// DashScope
	DashScopeAPIKey string
	DashScopeModel  string
}

type LLMConfig struct {
	Provider Provider

	// Ollama
	OllamaBaseURL string
	OllamaModel   string

	// DashScope
	DashScopeAPIKey string
	DashScopeModel  string
	MaxTokens       int
	Temperature     float64

	// Gemini
	GeminiAPIKey string
	GeminiModel  string
}

type ParserConfig struct {
	// ConfidenceThreshold: below this score, LLM fallback is triggered.
	ConfidenceThreshold float64
}

type SearchConfig struct {
	DefaultTopK  int
	HNSWEfSearch int
}

type LogConfig struct {
	Level  string // debug | info | warn | error
	Format string // json | console
}

// Load reads configuration from environment variables (with .env file support).
// Precedence: env vars > .env file > defaults.
func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("SERVER_PORT", 8080)
	v.SetDefault("SERVER_HOST", "0.0.0.0")
	v.SetDefault("DATABASE_MAX_CONNS", 20)
	v.SetDefault("DATABASE_MIN_CONNS", 2)
	v.SetDefault("EMBEDDING_PROVIDER", "ollama")
	v.SetDefault("EMBEDDING_DIMENSIONS", 1024)
	v.SetDefault("OLLAMA_BASE_URL", "http://localhost:11434")
	v.SetDefault("OLLAMA_EMBEDDING_MODEL", "bge-m3")
	v.SetDefault("OLLAMA_LLM_MODEL", "qwen2.5:14b")
	v.SetDefault("DASHSCOPE_EMBEDDING_MODEL", "text-embedding-v3")
	v.SetDefault("DASHSCOPE_LLM_MODEL", "qwen-plus")
	v.SetDefault("DASHSCOPE_LLM_MAX_TOKENS", 2048)
	v.SetDefault("DASHSCOPE_LLM_TEMPERATURE", 0.3)
	v.SetDefault("GEMINI_LLM_MODEL", "gemini-1.5-flash")
	v.SetDefault("LLM_PROVIDER", "gemini")
	v.SetDefault("PARSER_CONFIDENCE_THRESHOLD", 0.75)
	v.SetDefault("SEARCH_DEFAULT_TOP_K", 10)
	v.SetDefault("SEARCH_HNSW_EF_SEARCH", 64)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")

	// .env file (optional)
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() // ignore error if .env doesn't exist

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	cfg := &Config{
		Server: ServerConfig{
			Host:         v.GetString("SERVER_HOST"),
			Port:         v.GetInt("SERVER_PORT"),
			ReadTimeout:  600 * time.Second,
			WriteTimeout: 600 * time.Second,
		},
		Database: DatabaseConfig{
			URL:      v.GetString("DATABASE_URL"),
			MaxConns: int32(v.GetInt("DATABASE_MAX_CONNS")),
			MinConns: int32(v.GetInt("DATABASE_MIN_CONNS")),
		},
		Embedding: EmbeddingConfig{
			Provider:        Provider(v.GetString("EMBEDDING_PROVIDER")),
			Dimensions:      v.GetInt("EMBEDDING_DIMENSIONS"),
			OllamaBaseURL:   v.GetString("OLLAMA_BASE_URL"),
			OllamaModel:     v.GetString("OLLAMA_EMBEDDING_MODEL"),
			DashScopeAPIKey: v.GetString("DASHSCOPE_API_KEY"),
			DashScopeModel:  v.GetString("DASHSCOPE_EMBEDDING_MODEL"),
		},
		LLM: LLMConfig{
			Provider:        Provider(v.GetString("LLM_PROVIDER")),
			OllamaBaseURL:   v.GetString("OLLAMA_BASE_URL"),
			OllamaModel:     v.GetString("OLLAMA_LLM_MODEL"),
			DashScopeAPIKey: v.GetString("DASHSCOPE_API_KEY"),
			DashScopeModel:  v.GetString("DASHSCOPE_LLM_MODEL"),
			MaxTokens:       v.GetInt("DASHSCOPE_LLM_MAX_TOKENS"),
			Temperature:     v.GetFloat64("DASHSCOPE_LLM_TEMPERATURE"),
			GeminiAPIKey:    v.GetString("GEMINI_API_KEY"),
			GeminiModel:     v.GetString("GEMINI_LLM_MODEL"),
		},
		Parser: ParserConfig{
			ConfidenceThreshold: v.GetFloat64("PARSER_CONFIDENCE_THRESHOLD"),
		},
		Search: SearchConfig{
			DefaultTopK:  v.GetInt("SEARCH_DEFAULT_TOP_K"),
			HNSWEfSearch: v.GetInt("SEARCH_HNSW_EF_SEARCH"),
		},
		Log: LogConfig{
			Level:  v.GetString("LOG_LEVEL"),
			Format: v.GetString("LOG_FORMAT"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("config: DATABASE_URL is required")
	}
	if c.Embedding.Provider == ProviderDashScope && c.Embedding.DashScopeAPIKey == "" {
		return fmt.Errorf("config: DASHSCOPE_API_KEY is required when EMBEDDING_PROVIDER=dashscope")
	}
	if c.LLM.Provider == ProviderDashScope && c.LLM.DashScopeAPIKey == "" {
		return fmt.Errorf("config: DASHSCOPE_API_KEY is required when LLM_PROVIDER=dashscope")
	}
	if c.LLM.Provider == ProviderGemini && c.LLM.GeminiAPIKey == "" {
		return fmt.Errorf("config: GEMINI_API_KEY is required when LLM_PROVIDER=gemini")
	}
	return nil
}

// Addr returns the "host:port" string for the HTTP server.
func (c *ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
