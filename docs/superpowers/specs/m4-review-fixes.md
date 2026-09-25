# Milestone 4 review fixes

Source: `/impeccable critique` (dual-agent, 20/40) + tester E2E journeys on `m4-frontend`. Snapshot: `.impeccable/critique/2026-09-23T21-18-15Z__frontend-src.md`.

Rules: test or failing check first where logic changes; one commit per numbered group; keep mockup tokens and class names; copy below is exact.

## 1. Confirm a correct flagged value (P0)
- `Fields.tsx`: the flagged row shows the input pre-filled in the same money format as other fields (`$18,482.00`) plus a **Confirm value** button. Confirm sends the current value (parsed number) via `api.editField`, even when unchanged. Enter in the input does the same. Editing then Confirm saves the new value.
- Backend already accepts an unchanged value (sets `edited = 1`, writes audit). Add an HTTP test proving PATCH with the same value clears the flag and writes a `field_edited` audit row.
- Fake reason text (`pipeline/fake/fake.go`): `Scan is unclear on the tens digit (3 or 8). Check the source, then confirm or correct the value.`

## 2. Approve requires a complete file (P0)
- Backend `decide`: `approve` returns 409 when any of the five required types is missing, any active document is `failed`, or any is still processing. Error: `Upload the <labels joined with ", " and " and "> before approving.` / `Retry or remove the failed document before approving.` / `Wait for document processing to finish.` Reject and send back stay allowed. Test first.
- Frontend: Approve disabled with the same message; document strip shows each missing type as a dashed empty slot labeled with the type name (not clickable, `aria-label="W-2 not uploaded"`).
- Status: when the assessment has no reasons, case status is `ready` and the pill reads **Ready for decision** (already mapped; confirm it shows).

## 3. Queue shows what blocks each case (P1)
- `CaseSummary` gains `blocker: string` from the server (first applicable of: `<n> documents missing`, `<n> field(s) to verify` with correct plural, `Document failed`, `Processing`, `DTI near 43% limit`, `DTI above 43%`, `Ready for decision`, or the decision label). Queue shows it under the borrower name instead of repeating the loan product; dot color: warn for review blockers, bad for failed/above limit, good for ready, none for decided. Dot `aria-label` = blocker text.

## 4. Decision bar clarity (P1)
- `.btn:disabled { opacity: .5; cursor: not-allowed; }` for every button variant.
- Note placeholder `Note for the file`; helper text under it `Required to reject or send back.`
- The Approve-blocked reason sits directly under the action row at 12.5px, `--ink-soft`, weight 500 (not `.queue-loan`), `role="status"`.
- Action row stays on one line at ≥ 900px; below that it stacks cleanly (note full width, buttons in one row).

## 5. Bugs (P1)
- Brand mark stroke uses `currentColor` with the mark's `color: var(--surface)` so it shows in both themes (Login.tsx, Workspace.tsx).
- "Add" upload becomes a real `<button>` that opens a hidden `<input type=file>` via ref; keyboard-focusable, `aria-label="Add documents"`.
- No horizontal overflow at 390px: `document.documentElement.scrollWidth <= 390` on login, empty queue, and a full case. Topbar wraps; reviewer name truncates.

## 6. Copy and consistency (P2/P3)
- Recommendation reasons: backend builds them with human labels (`OCR extraction quality`, `Document authenticity`, `Field completeness`, `Income consistency`) and correct singular/plural (`1 field needs verification`, `2 fields need verification`, `1 document failed processing`).
- DTI null state: `Not computed` plus a line naming the cause from the assessment (`Monthly income missing: upload a pay stub or W‑2.` / `Monthly debt missing: upload a bank statement.`).
- Confidence card: two labeled groups, **This document** (selected document's judgments) and **Whole case** (case judgments). Group labels are plain 12.5px 600 text, not uppercase eyebrows.
- Preview failure: if the iframe or image fails, show `Preview unavailable.` and a link `Open the original file` to `/api/documents/{id}/file` (new tab). Images use `onError`; PDFs: keep the iframe, and add the link under it always.
- Timestamps: one formatter, `en-US`, `Sep 24, 2026, 9:14 AM` style, used in audit and decided line; audit lines show date and time.
- Login error shown as `Email or password is incorrect. Check both and try again.` (frontend maps 401 on login; server text unchanged).
- Dark theme: redefine the verdict shadow in both dark token blocks (no teal glow); `.audit` gets `padding-top: 10px`.
- DTI bar: single fill color by band (`--good` below 41%, `--warn` 41–43%, `--bad` above) instead of the 3-stop gradient.

## 7. Testability: failed document path
- Fake parser fails the first parse of any file whose name contains `fail-once` (`fake.Stages()` sets `FailOnce: {"fail-once": true}` per server process — use a per-file map keyed by document path so each such file fails once). Add fixture `frontend/e2e/fixtures/pay-stub-fail-once.pdf`.

## 8. Demo data matches extraction
- E2E fixtures print the exact values the fake extractor returns (so a side-by-side check agrees). Align the fake pay stub with the plan's gendata boundary scenario: `gross_pay 3600`, `ytd_gross 57600`, `monthly_income 7200`. Fixture PDFs must be valid, readable PDFs containing those values (fpdf-generated via a small `backend/cmd/gendata`-independent helper or a checked-in generated file) — no 60-byte stubs.

## E2E additions (`frontend/e2e/review.ego.mjs`)
- Confirm-unchanged path: a second lowq case where the reviewer presses Confirm value without editing → Approve enabled.
- Missing documents: case with 2 documents shows dashed slots and Approve disabled with the upload message.
- Failed document: upload `pay-stub-fail-once.pdf` → reason and Retry visible → Retry → document processed.
- Phone width: at 390×844 assert `scrollWidth <= 390` on the case page; screenshot.
- Each new step writes a screenshot and a PASS/FAIL line.
