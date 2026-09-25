# Rename: Harbor Underwriting → Case Review, fictional names only

"Harbor Underwriting" collides with real firms (Harbour Underwriting Ltd, London; Harbor.ai), and "Harborview Community Bank" with real banks (Harborview Bank, Harbor Community Bank). "Northwind" is Microsoft's sample-data brand. Demo must not resemble any real organization.

One commit per item, local git only.

## 1. Product name (user-visible)
- Product name everywhere a user or reader sees it: `Case Review`. Covers `frontend/index.html` `<title>`, Login and Workspace brand text, README, `.env.example` comments, the mockup `docs/superpowers/specs/case-review-mockup.html`.
- Brand mark stays (abstract wave glyph, no letters).
- Seeded reviewer email `maya@harbor.test` becomes `reviewer@casereview.test` (reserved `.test` TLD); keep the name `Maya Park`. Update README, E2E, verify scripts, and tests that use it.
- Internal identifiers stay: `HARBOR_LIVE`, `HARBOR-FIELDS:` PDF marker, `.pi/skills/verify-harbor`, Go module paths. They never render in the UI or on documents.

## 2. Synthetic names
- Every bank, employer, and borrower in `backend/cmd/gendata/scenarios.go` (and fake canned values in `backend/internal/pipeline/fake/fake.go`) uses clearly fictional names built from `Example` / `Sample` / `Demo`, e.g. `Example Community Bank`, `Sample Freight LLC`, `Demo Paper Company`. No real brand words (Northwind, Harbor, Harborview, Pinecrest, Summit, Cedar, Linden, Elm Grove). Borrower names stay generic personal names (Jordan Alvarez etc. are fine).
- Regenerate `testdata/` and E2E fixtures; E2E and fake eval stay green.
