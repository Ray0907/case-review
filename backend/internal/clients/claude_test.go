package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeExtractUsesProxyAndSessionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Header.Get("X-Spanbox-Session") != "eval-1" {
			t.Errorf("session header %q", r.Header.Get("X-Spanbox-Session"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "claude-opus-5" {
			t.Errorf("model %v", body["model"])
		}
		system := body["system"].([]any)[0].(map[string]any)["text"].(string)
		for _, rule := range []string{
			"buy-now-pay-later installments are not included in monthly_debt and are counted separately in bnpl_hits",
			`Do not mark a value uncertain because other documents are needed to corroborate it; cross-document checks happen elsewhere.`,
			`Put a field in "uncertain" only when its printed value is illegible or ambiguous, or when figures on this document do not reconcile with each other`,
		} {
			if !strings.Contains(system, rule) {
				t.Errorf("extraction system prompt missing %q", rule)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5",
			"stop_reason": "tool_use", "usage": map[string]int{"input_tokens": 10, "output_tokens": 10},
			"content": []any{map[string]any{"type": "tool_use", "id": "tu_1", "name": "record_fields",
				"input": map[string]any{
					"fields":    map[string]any{"employer_name": "Northwind", "tax_year": 2025, "box1_wages": 86400, "box2_fed_tax": 11230},
					"uncertain": []any{map[string]any{"field": "box2_fed_tax", "reason": "smudged"}},
				}}},
		})
	}))
	defer srv.Close()
	c := NewClaude(ClaudeOptions{BaseURL: srv.URL, SpanboxSession: "eval-1", APIKey: "test", Model: "claude-opus-5"})
	ex, err := c.Extract(context.Background(), "w2", "W-2 text")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Fields["box1_wages"] != 86400.0 || ex.Uncertain["box2_fed_tax"] != "smudged" {
		t.Fatalf("%+v", ex)
	}
}

func TestBankWithdrawalIsOptionalInExtractionSchema(t *testing.T) {
	schema := fieldSchema("bank_statement")
	props := schema["properties"].(map[string]any)
	if props["total_withdrawals"] == nil {
		t.Fatal("withdrawals not extractable")
	}
	for _, name := range schema["required"].([]string) {
		if name == "total_withdrawals" {
			t.Fatal("withdrawals required for old statements")
		}
	}
}

func TestClaudeRefusalIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "m", "type": "message", "role": "assistant", "model": "claude-opus-5",
			"stop_reason": "refusal", "content": []any{}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 0}})
	}))
	defer srv.Close()
	c := NewClaude(ClaudeOptions{BaseURL: srv.URL, APIKey: "test", Model: "claude-opus-5"})
	if _, err := c.Extract(context.Background(), "w2", "x"); err == nil {
		t.Fatal("want refusal error")
	}
}
