package httpapi

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"tidalwave/backend/internal/pipeline/fake"
)

func readyCase(t *testing.T, lowQuality bool) (*testEnv, string, string) {
	env := newTestEnvWithRunner(t, fake.Stages())
	id := env.createCase(t)
	bank := "bank-statement.pdf"
	if lowQuality {
		bank = "bank-statement-lowq.png"
	}
	var bankID string
	for _, n := range []string{"w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", bank} {
		content := minimalPDF
		if n == bank && lowQuality {
			content = pngBytes
		}
		doc := decode[map[string]any](t, env.upload(t, id, n, content))
		if n == bank {
			bankID = doc["id"].(string)
		}
	}
	env.realRunner.Wait()
	return env, id, bankID
}

func TestApproveBlockedByUnresolvedFlag(t *testing.T) {
	env, id, _ := readyCase(t, true)
	res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
	body := decode[map[string]any](t, res)
	if body["unresolved"].(float64) != 1 {
		t.Fatalf("body %v", body)
	}
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	if detail["case"].(map[string]any)["status"] != "needs_review" {
		t.Fatal("status must be unchanged")
	}
}

func TestConfirmUnchangedFlaggedValue(t *testing.T) {
	env, id, bankID := readyCase(t, true)
	res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": 18482})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("confirm unchanged value: %d", res.StatusCode)
	}
	detail := decode[map[string]any](t, res)
	for _, doc := range detail["documents"].([]any) {
		for _, raw := range doc.(map[string]any)["fields"].([]any) {
			field := raw.(map[string]any)
			if field["key"] == "ending_balance" && (field["edited"] != true || field["value"] != float64(18482)) {
				t.Fatalf("field not confirmed: %v", field)
			}
		}
	}
	audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil))
	if len(audit) != 1 || audit[0]["action"] != "field_edited" || audit[0]["note"] != "Bank statement · Ending balance: $18,482.00 confirmed" {
		t.Fatalf("audit: %v", audit)
	}
}

func TestEditClearsFlagUpdatesDTIThenApprove(t *testing.T) {
	env, id, bankID := readyCase(t, true)
	res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/monthly_debt", map[string]any{"value": 2900})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit monthly_debt %d", res.StatusCode)
	}
	res = env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": 18432})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("edit ending_balance %d", res.StatusCode)
	}
	env.realRunner.Wait()
	auditBefore := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil))
	if auditBefore[0]["note"] != "Bank statement · Ending balance: $18,482.00 → $18,432.00" {
		t.Fatalf("edit note: %v", auditBefore[0]["note"])
	}
	detail := decode[map[string]any](t, env.do(t, "GET", "/api/cases/"+id, nil))
	if dti := detail["assessment"].(map[string]any)["dti"].(float64); dti != 0.4028 {
		t.Fatalf("dti after edit %v", dti)
	}
	res = env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve", "note": "Verified balance against source"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve %d", res.StatusCode)
	}
	audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil))
	if len(audit) != 3 || audit[0]["action"] != "approved" || audit[0]["user_name"] != "Maya Park" {
		t.Fatalf("audit %v", audit)
	}
}

func TestEditSupersededDocumentIsRejected(t *testing.T) {
	env, id, oldID := readyCase(t, false)
	res := env.upload(t, id, "bank-statement-second.pdf", minimalPDF)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("replacement upload %d", res.StatusCode)
	}
	env.realRunner.Wait()
	old, err := env.store.GetDocument(oldID)
	if err != nil || old.Status != "superseded" {
		t.Fatalf("old document %+v, err %v", old, err)
	}
	before, err := env.store.FieldValues(oldID)
	if err != nil {
		t.Fatal(err)
	}
	res = env.do(t, "PATCH", "/api/documents/"+oldID+"/fields/ending_balance", map[string]any{"value": 1})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
	if body := decode[map[string]string](t, res); body["error"] != "this document was replaced by a newer upload; edit the current one" {
		t.Fatalf("unexpected response %v", body)
	}
	after, err := env.store.FieldValues(oldID)
	if err != nil || after["ending_balance"] != before["ending_balance"] {
		t.Fatalf("superseded field changed: before %v after %v err %v", before, after, err)
	}
	if audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil)); len(audit) != 0 {
		t.Fatalf("rejected edit wrote audit %+v", audit)
	}
}

func TestEditRejectsWrongKind(t *testing.T) {
	env, _, bankID := readyCase(t, false)
	res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": "lots"})
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
}

func TestSendBackRequiresNote(t *testing.T) {
	env, id, _ := readyCase(t, false)
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "send_back"}); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "send_back", "note": "Need 2024 W-2"}); res.StatusCode != http.StatusOK {
		t.Fatalf("want 200 got %d", res.StatusCode)
	}
}

func TestConcurrentApprovalsRecordOneDecision(t *testing.T) {
	env, id, _ := readyCase(t, false)
	start := make(chan struct{})
	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		go func() {
			<-start
			req, err := http.NewRequest("POST", env.url+"/api/cases/"+id+"/decision", strings.NewReader(`{"action":"approve"}`))
			if err != nil {
				results <- 0
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := env.client.Do(req)
			if err != nil {
				results <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			results <- res.StatusCode
		}()
	}
	close(start)
	approved, conflicts := 0, 0
	for i := 0; i < 10; i++ {
		switch code := <-results; code {
		case http.StatusOK:
			approved++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected decision response %d", code)
		}
	}
	if approved != 1 || conflicts != 9 {
		t.Fatalf("want one 200 and nine 409s; got %d and %d", approved, conflicts)
	}
	audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil))
	if len(audit) != 1 || audit[0]["action"] != "approved" {
		t.Fatalf("audit %+v", audit)
	}
}

func TestDecidedCaseIsFrozen(t *testing.T) {
	env, id, bankID := readyCase(t, false)
	env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "reject", "note": "DTI too high after review"})
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"}); res.StatusCode != http.StatusConflict {
		t.Fatalf("second decision want 409 got %d", res.StatusCode)
	}
	if res := env.do(t, "PATCH", "/api/documents/"+bankID+"/fields/ending_balance", map[string]any{"value": 1}); res.StatusCode != http.StatusConflict {
		t.Fatalf("edit after decision want 409 got %d", res.StatusCode)
	}
	if audit := decode[[]map[string]any](t, env.do(t, "GET", "/api/cases/"+id+"/audit", nil)); len(audit) != 1 {
		t.Fatalf("audit %v", audit)
	}
}

func TestApproveRequiresCompleteFile(t *testing.T) {
	t.Run("missing documents", func(t *testing.T) {
		env := newTestEnvWithRunner(t, fake.Stages())
		id := env.createCase(t)
		for _, name := range []string{"w2-2025.pdf", "form-1040.pdf"} {
			env.upload(t, id, name, minimalPDF)
		}
		env.realRunner.Wait()
		res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"})
		if res.StatusCode != 409 {
			t.Fatalf("missing types: %d", res.StatusCode)
		}
		body := decode[map[string]any](t, res)
		if body["error"] != "Upload the Form 1003, pay stub and bank statement before approving." {
			t.Fatalf("%v", body)
		}
		if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "reject", "note": "Incomplete file"}); res.StatusCode != 200 {
			t.Fatalf("reject incomplete: %d", res.StatusCode)
		}
	})
	t.Run("failed document", func(t *testing.T) {
		env, id, bankID := readyCase(t, false)
		if err := env.store.SetDocumentStatus(bankID, "failed", "simulated parser outage"); err != nil {
			t.Fatal(err)
		}
		if err := env.realRunner.Refresh(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"})
		if res.StatusCode != 409 {
			t.Fatalf("failed doc: %d", res.StatusCode)
		}
		if body := decode[map[string]any](t, res); body["error"] != "Retry the failed document before approving." {
			t.Fatalf("%v", body)
		}
	})
	t.Run("active document", func(t *testing.T) {
		env, id, bankID := readyCase(t, false)
		if err := env.store.SetDocumentStatus(bankID, "parsing", ""); err != nil {
			t.Fatal(err)
		}
		res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"})
		if res.StatusCode != 409 {
			t.Fatalf("active doc: %d", res.StatusCode)
		}
		if body := decode[map[string]any](t, res); body["error"] != "Wait for document processing to finish." {
			t.Fatalf("%v", body)
		}
	})
	t.Run("send back while processing", func(t *testing.T) {
		env := newTestEnv(t)
		id := env.createCase(t)
		res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "send_back", "note": "Please upload the documents"})
		if res.StatusCode != 200 {
			t.Fatalf("send back while processing: %d", res.StatusCode)
		}
	})
}

func TestDecisionWhileProcessing(t *testing.T) {
	env := newTestEnv(t)
	id := env.createCase(t)
	if res := env.do(t, "POST", "/api/cases/"+id+"/decision", map[string]string{"action": "approve"}); res.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 got %d", res.StatusCode)
	}
}
