package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	Port             string
	DataDir          string
	StaticDir        string
	PipelineMode     string
	AnthropicBaseURL string
	SpanboxSession   string
	SpanboxToken     string
	LlamaParseKey    string
	LlamaParseURL    string
	TypeSafeKey      string
	SeedEmail        string
	SeedPassword     string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// The user's .env uses LLAMAPARSE_API and TYPESAVE_API; the *_KEY names are accepted too.
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func Load() Config {
	return Config{
		Port:             env("PORT", "8080"),
		DataDir:          env("DATA_DIR", filepath.Join(".", "data")),
		StaticDir:        env("STATIC_DIR", filepath.Join("..", "frontend", "dist")),
		PipelineMode:     env("PIPELINE_MODE", "real"),
		AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
		SpanboxSession:   os.Getenv("SPANBOX_SESSION"),
		SpanboxToken:     os.Getenv("SPANBOX_TOKEN"),
		LlamaParseKey:    firstEnv("LLAMAPARSE_API_KEY", "LLAMAPARSE_API"),
		LlamaParseURL:    env("LLAMAPARSE_BASE_URL", "https://api.cloud.llamaindex.ai"),
		TypeSafeKey:      firstEnv("TYPESAFE_API_KEY", "TYPESAVE_API"),
		SeedEmail:        env("SEED_EMAIL", "reviewer@casereview.test"),
		SeedPassword:     env("SEED_PASSWORD", "casereview-demo"),
	}
}
