package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jevServer(t *testing.T, answers map[string]any, check func(body map[string]any)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("bad request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "jev-latest" {
			t.Errorf("model %v", body["model"])
		}
		if check != nil {
			check(body)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "jev-1.13.0", "answers": answers})
	}))
}

func TestJevClassify(t *testing.T) {
	srv := jevServer(t, map[string]any{"doc_type": map[string]any{"type": "choice", "choice": "bank_statement", "confidence": 0.91}},
		func(body map[string]any) {
			q := body["questions"].(map[string]any)["doc_type"].(map[string]any)
			crit := q["criteria"].(map[string]any)
			if q["type"] != "choice" || len(crit) != 6 {
				t.Errorf("question %v", q)
			}
		})
	defer srv.Close()
	ty, conf, err := NewJev("k", srv.URL).Classify(context.Background(), "Chase statement ...")
	if err != nil || ty != "bank_statement" || conf != 0.91 {
		t.Fatalf("%s %v %v", ty, conf, err)
	}
}

func TestJevClassifyRejectsUnknownChoice(t *testing.T) {
	srv := jevServer(t, map[string]any{"doc_type": map[string]any{"type": "choice", "choice": "passport", "confidence": 0.9}}, nil)
	defer srv.Close()
	if _, _, err := NewJev("k", srv.URL).Classify(context.Background(), "x"); err == nil {
		t.Fatal("want error for choice outside criteria")
	}
}

func TestJevJudgeDocument(t *testing.T) {
	srv := jevServer(t, map[string]any{
		"field_completeness":    map[string]any{"type": "noul", "noul": 0.96},
		"document_authenticity": map[string]any{"type": "noul", "noul": 0.91},
		"ocr_quality":           map[string]any{"type": "noul", "noul": 0.54},
	}, nil)
	defer srv.Close()
	js, err := NewJev("k", srv.URL).JudgeDocument(context.Background(), "bank_statement", "text", map[string]any{"ending_balance": 1.0})
	if err != nil || len(js) != 3 {
		t.Fatalf("%v %v", js, err)
	}
	for _, j := range js {
		if j.Name == "ocr_quality" && j.Score != 0.54 {
			t.Fatalf("ocr %+v", j)
		}
	}
}
