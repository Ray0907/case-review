# Milestone 5 additions (on top of plan Tasks 15–17)

Deferred from the m4 review. Same rules as the plan: failing test first for backend logic, one commit per numbered item, local git only.

## 1. Fake mode reads values from the document
- `gendata` writes the document's extracted fields into the PDF Keywords metadata as `HARBOR-FIELDS:<compact JSON>` (ASCII only, `fpdf.SetKeywords`), alongside the printed values. Printed values and the marker must agree (test: generate, read the file bytes, parse the marker, compare with `docSpec.Fields`).
- Fake parser: when the uploaded bytes contain `HARBOR-FIELDS:{...}`, pass that JSON through to the fake extractor, which returns those fields (and the fake classifier uses the scenario doc type from the marker's `doc_type` key when present). No marker: today's fixed values, unchanged, so m4 E2E fixtures keep working.
- Result: uploading different scenario folders produces different DTI and recommendations in fake mode.

## 2. Demo cases that tell the story
- `gendata` also writes `testdata/documents/<scenario>/` with one fictional borrower per scenario (name, employer, bank consistent across that scenario's five documents; different across scenarios).
- `standard` must render **Ready for decision** in the UI in fake mode (eligible, no flags); `boundary` shows `DTI near 43% limit`; add one `above_limit` scenario (DTI > 0.43, `not_eligible` or the backend's above-limit recommendation) so the queue shows good, warn, and bad dots.
- README demo section: exact steps to create one case per scenario from `testdata/documents/` in fake mode.

## 3. E2E
- Add one step to `frontend/e2e/review.ego.mjs`: create a case from `testdata/documents/standard/` and assert the status pill reads `Ready for decision` and Approve is enabled; create one from `above_limit/` and assert `DTI above 43%`.
