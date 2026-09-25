package clients

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"tidalwave/backend/internal/pipeline"
	"tidalwave/backend/internal/schemas"
)

type ClaudeOptions struct {
	APIKey         string
	BaseURL        string
	SpanboxSession string
	SpanboxToken   string
	Model          string
}

type Claude struct {
	client anthropic.Client
	model  string
}

func NewClaude(o ClaudeOptions) *Claude {
	var opts []option.RequestOption
	if o.APIKey != "" {
		opts = append(opts, option.WithAPIKey(o.APIKey))
	}
	if o.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(o.BaseURL))
	}
	if o.SpanboxSession != "" {
		opts = append(opts, option.WithHeader("X-Spanbox-Session", o.SpanboxSession))
	}
	if o.SpanboxToken != "" {
		opts = append(opts, option.WithHeader("X-Spanbox-Token", o.SpanboxToken))
	}
	if o.Model == "" {
		o.Model = "claude-opus-5"
	}
	return &Claude{client: anthropic.NewClient(opts...), model: o.Model}
}

func fieldSchema(docType string) map[string]any {
	props := map[string]any{}
	var required []string
	for _, f := range schemas.Registry[docType] {
		t := "string"
		switch f.Kind {
		case schemas.KindNumber:
			t = "number"
		case schemas.KindInt:
			t = "integer"
		}
		props[f.Key] = map[string]any{"type": t, "description": f.Label}
		if !schemas.IsOptional(docType, f.Key) {
			required = append(required, f.Key)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

const extractSystem = `You extract fields from mortgage documents for an underwriter. Copy values exactly as printed; convert money to plain numbers without symbols or commas. For pay stubs, monthly_income is gross pay normalized to a month (weekly x 52 / 12, biweekly x 26 / 12, semimonthly x 2). For bank statements, monthly_debt is the sum of recurring debt payments (loans, card minimums, housing); buy-now-pay-later installments are not included in monthly_debt and are counted separately in bnpl_hits. Put a field in "uncertain" only when its printed value is illegible or ambiguous, or when figures on this document do not reconcile with each other; give a one-sentence reason. Do not mark a value uncertain because other documents are needed to corroborate it; cross-document checks happen elsewhere. Always answer by calling record_fields.`

func (c *Claude) Extract(ctx context.Context, docType, text string) (pipeline.Extraction, error) {
	tool := anthropic.ToolParam{
		Name:        "record_fields",
		Description: anthropic.String("Record the extracted fields and any uncertain ones."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"fields": fieldSchema(docType),
				"uncertain": map[string]any{"type": "array", "items": map[string]any{
					"type": "object", "required": []string{"field", "reason"},
					"properties": map[string]any{"field": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}},
				}},
			},
			Required: []string{"fields", "uncertain"},
		},
	}
	resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4000,
		System:    []anthropic.TextBlockParam{{Text: extractSystem}},
		Tools:     []anthropic.ToolUnionParam{{OfTool: &tool}},
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(
			fmt.Sprintf("Document type: %s\n\n<document>\n%s\n</document>", schemas.Label(docType), text)))},
	})
	if err != nil {
		return pipeline.Extraction{}, err
	}
	if resp.StopReason == "refusal" {
		return pipeline.Extraction{}, fmt.Errorf("claude declined to extract this document")
	}
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok && tu.Name == "record_fields" {
			var in struct {
				Fields    map[string]any                   `json:"fields"`
				Uncertain []struct{ Field, Reason string } `json:"uncertain"`
			}
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &in); err != nil {
				return pipeline.Extraction{}, fmt.Errorf("claude tool input: %w", err)
			}
			ex := pipeline.Extraction{Fields: in.Fields, Uncertain: map[string]string{}}
			for _, u := range in.Uncertain {
				ex.Uncertain[u.Field] = u.Reason
			}
			return ex, nil
		}
	}
	return pipeline.Extraction{}, fmt.Errorf("claude did not call record_fields (stop_reason %s)", resp.StopReason)
}
