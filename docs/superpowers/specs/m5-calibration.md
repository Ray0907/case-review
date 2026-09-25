# Confidence calibration from the real eval

Real eval (eval-1790216627) plus probes on `standard` (should raise no flag):
- field_completeness 0.96–0.98, ocr_quality 0.94–0.97 on all clean docs.
- document_authenticity 0.76–0.91 on clean docs; 0.03–0.07 when Jev judged documents not genuine.
- income_consistency 0.79 with W-2/12 exactly equal to pay-stub monthly income.
- low_quality_scan bank statement: field_completeness 0.03 (correct flag).
A single 0.8 cut flags 5/6 clean scenarios. One commit per item, test first, local git only.

## 1. Per-judgment thresholds
- One source of truth in `backend/internal/pipeline/assess.go`: `field_completeness` 0.8, `ocr_quality` 0.8, `document_authenticity` 0.5, `income_consistency` 0.5; unknown names default 0.8. Exported `IsLow(name string, score float64) bool`.
- Assessment reasons, eval flagging (`backend/cmd/eval`), and the API use it. Judgment JSON gains `low bool`; the confidence card colors a judgment warn only when `low` is true (no frontend threshold constants).
- Tests: 0.76 authenticity and 0.79 income are not low; 0.49 authenticity and 0.79 ocr are low.

## 2. Calibration table in the eval report
- Eval records every judgment score per document (and case-level income_consistency).
- `report.json` gains `calibration_table`: per judgment name, `threshold`, and min/max/count of scores split by fixtures with `should_flag_low_confidence` true vs false. Printed after the layer lines as a small aligned table.
- A judgment whose clean-side min is below its threshold is printed with `false-positive risk`. The flag side shows min only: flagged fixtures also contain clean documents, so their max says nothing. The per-fixture calibration layer checks that each flagged fixture has at least one low signal.

## 3. Tampered scenario
- New gendata scenario `tampered_statement`: all five docs, standard income/debt (DTI 0.2917), but the bank statement's printed ending balance is altered so beginning + deposits − withdrawals ≠ ending (off by $4,000), and the altered line uses a different font/size. Fixture expects `needs_review`, `should_flag_low_confidence: true`.
- Fake mode: the fake judge returns document_authenticity 0.3 for a bank statement whose marker fields break the balance identity, else current values. Fake eval stays 100%.
- One fictional borrower (generic personal name), employer and bank with Example/Sample/Demo names; add to README scenario table.

Run go vet, go test, npm run build, eval -fake, bash frontend/e2e/run.sh. No live APIs. Re-tag m5-data-eval.
