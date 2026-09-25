# Milestone 4 review fixes — round 2

Source: confirmation `/impeccable critique` (22/36, #7 n/a) + detector overlay + tester journeys on `m4-frontend@477fe4c`. Round 1 groups 1, 5, 6, 7 are confirmed fixed; this list closes what remains. Same rules: failing test first for backend logic, one commit per numbered group, mockup tokens and class names only, copy below is exact.

## 1. Status and next step visible without scrolling (P1)
- Case header: directly under the borrower name row, show the case's blocker text (same `blocker` string the queue uses) in 13px `--ink-soft` weight 600, next to the status pill. Examples: `1 field to verify`, `3 documents missing`, `Ready for decision`.
- When Approve is enabled but the recommendation is `needs_review`, show under the action row (same slot as the Approve-blocked reason, `--ink-soft`, 12.5px): `Check before approving: <reasons joined with "; ">.` Example for a near-limit case: `Check before approving: DTI is 42.8%, within 2 points of the 43% limit.` (backend reason text changes to this form: `DTI is <pct>%, within 2 points of the 43% limit`).

## 2. Remove the impossible "remove" instruction (P1)
- Copy everywhere (`review.go`, `Decision.tsx`): `Retry the failed document before approving.` No delete feature.

## 3. A failed document is one document (P1)
- A failed or processing document of a type counts as present for missing-slot purposes: no `Pay stub not uploaded` slot when a pay stub exists in any active status.
- Its thumb label is `<Type label> (failed)` when failed, `<Type label>` otherwise; unclassified shows `Processing…`.
- Remove `role="status"` from the empty slots.
- Friendly failure copy in the document panel: `This document couldn't be read. Retry to process it again.` with the technical reason underneath in 12px `--ink-mute` prefixed `Details: `.

## 4. Side-by-side review on laptop widths (P1)
- Document and fields columns stay two-up from 900px wide.
- Below 1220px the queue is no longer a left rail: it becomes a horizontal row of case chips above the case (same data, scrolls horizontally inside its own container, no page overflow).
- Below 900px: when a case is open, hide the queue and show a `← All cases` link at the top; when no case is open, show the queue list.

## 5. Buttons at the mockup size (P2)
- Remove the 11.5px / 8px button shrink (`app.css`); buttons use the mockup's 13px and padding.
- The action row may wrap: note input takes its own full-width line, buttons follow on one line.
- Disabled opacity applies only to `.btn-ghost` and `.btn-bad`; `.btn-primary:disabled` keeps the mockup's own disabled style (no extra dimming).

## 6. Copy fixes (P2/P3)
- `1 document missing` / `<n> documents missing`; same singular rule anywhere a count is rendered.
- Type labels inside sentences are lowercase except proper form names: `W-2`, `Form 1040`, `Form 1003`, `pay stub`, `bank statement` (`Upload the pay stub and bank statement before approving.`).
- Field status label for any reviewed field: `Verified by reviewer` (covers confirm and correct).
- Audit notes use human labels and formatted values: `Bank statement · Ending balance: $18,482.00 confirmed` when unchanged, `Bank statement · Ending balance: $18,482.00 → $18,432.00` when changed. Backend builds the note; test first.
- Low-confidence reasons name the document: `Low confidence: OCR extraction quality on the bank statement (54%)`.
- Confidence group label: `This document: <Type label>`.
- DTI legend: `QM threshold 43%` sits under the 43% marker (position it at `left: 43%` with `translateX(-50%)`).
- Helper text `Required to reject or send back.` and the audit lines render at 12px `--ink-soft` (not 11px).

## 7. Keyboard and focus (P2)
- After Confirm value or saving a field: focus moves to the next flagged field's input, or to Approve if none remain.
- After a decision: focus moves to the decided line (`tabIndex=-1`).
- Document strip: plain buttons with `aria-pressed` for the selected document (drop `role=tab`/`tablist` since there is no arrow-key support).
- Visible `:focus-visible` ring (2px `--ink`, offset 2px) on the note input, field inputs, and links.
- A `Skip to case` link as the first focusable element when a case is open.
- Login shell keeps 16px side padding at 390px.

## 8. Demo fixture completeness
- Add `frontend/e2e/fixtures/bank-statement.pdf`: a valid, readable PDF printing exactly the fake extractor's bank values. (Per-borrower documents and varied DTI across cases are handled in Milestone 5 / Task 15, not here.)

## E2E additions
- Case header shows the blocker text before scrolling (assert it is inside the first viewport: `getBoundingClientRect().top < innerHeight`).
- Near-limit case shows `Check before approving:` with Approve enabled.
- Failed pay stub: no `Pay stub not uploaded` slot; thumb reads `Pay stub (failed)`; friendly copy visible.
- 1024×768: document preview and fields are side by side (their `top` values within 10px).
- 390×844: open case hides the queue and shows `← All cases`; `scrollWidth <= 390`.
- Keyboard: after Confirm value, `document.activeElement` is Approve.

## 9. E2E resilience to ego-browser timeouts
- The tester's independent run failed one step with `CDP request timed out: Runtime.evaluate` (ego-browser transport, not an app assertion). Wrap every `page.evaluate` and `page.waitForFunction` in the same one-retry helper as screenshots: retry once only on `CdpRequestTimeoutError` / messages containing `timed out`; any other error or a second timeout fails the step with the original message. Record retries in `results.json` (`retried: true`) so flakiness stays visible.
