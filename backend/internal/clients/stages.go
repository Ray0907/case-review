package clients

import (
	"errors"

	"tidalwave/backend/internal/config"
	"tidalwave/backend/internal/pipeline"
)

func RealStages(cfg config.Config) (pipeline.Stages, error) {
	// ANTHROPIC_API_KEY is not required here: the SDK also resolves an `ant auth login` profile.
	var missing []string
	if cfg.LlamaParseKey == "" {
		missing = append(missing, "LLAMAPARSE_API_KEY")
	}
	if cfg.TypeSafeKey == "" {
		missing = append(missing, "TYPESAFE_API_KEY")
	}
	if len(missing) > 0 {
		return pipeline.Stages{}, errors.New("missing env: " + joinComma(missing) + " (or run with PIPELINE_MODE=fake)")
	}
	jev := NewJev(cfg.TypeSafeKey, "")
	return pipeline.Stages{
		Parser:     NewLlamaParse(cfg.LlamaParseURL, cfg.LlamaParseKey),
		Classifier: jev,
		Extractor: NewClaude(ClaudeOptions{BaseURL: cfg.AnthropicBaseURL, SpanboxSession: cfg.SpanboxSession,
			SpanboxToken: cfg.SpanboxToken, Model: "claude-opus-5"}),
		Judge: jev,
	}, nil
}

func joinComma(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ", "
		}
		out += v
	}
	return out
}
