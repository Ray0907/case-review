# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

React + Vite + Tailwind CSS v4 (frontend), Go (backend, REST API). Chosen to match a job-application requirement (Tidalwave, Software Engineer Full-Stack + LLM) that names Go and React explicitly.

## Users

Primary user: a mortgage underwriter / loan officer reviewing borrower-submitted documents to assess repayment ability before approving a loan. They work case-by-case, need to trust but verify AI-extracted numbers against source documents, and must be able to correct mistakes before a decision is recorded.

There is no separate borrower-facing surface in this build — document upload and review both happen inside the same reviewer-facing dashboard (single-tenant, single-reviewer demo; not a borrower portal).

## Product Purpose

An AI-assisted mortgage document review agent: borrower documents (W2, 1040, Form 1003, pay stub, bank statement) are OCR'd, classified, and extracted into structured fields, DTI (debt-to-income) is calculated, and the system produces a repayment-ability recommendation with an explainable, per-judgment confidence breakdown. The system never auto-approves or auto-rejects — a human reviewer always makes the final call, which is logged.

This is a portfolio/interview demo project (not a production system), built to demonstrate full-stack + agentic-AI engineering ability for a specific job interview.

## Positioning

Differentiates from existing document-AI products (Ocrolus, Docsumo, Hyperscience, Altyst) and from a comparable Microsoft/Mistral reference pipeline not on "we also do OCR extraction" but on: (1) an explainable confidence layer built from independent judgment primitives (via the Jev/TypeSafe model) rather than a single opaque confidence score, (2) a document-authenticity/fraud check as a first-class judgment, not an afterthought, and (3) mandatory human-in-the-loop decisioning framed explicitly around US mortgage fair-lending compliance (ECOA / Regulation B), not just as a UX safety net.

## Operating Context

- Reviewer logs in (simple native Go session auth — email/password, bcrypt + cookie session, no third-party auth provider), uploads a case's documents, watches live processing status (SSE-pushed: classifying → parsing → extracting → calculating), reviews extracted fields side-by-side with the source document, edits any field before deciding, and approves / rejects / sends-back-for-more-documents. Every decision is written to an audit log (who, when, what).
- Pipeline: upload → Jev (TypeSafe) classifies document type → llamaparse OCR/parses → Claude API extracts structured fields against a per-document-type JSON schema → Jev Noul judgments score document authenticity and field-confidence → Go backend computes DTI and a rule-based qualification threshold → system renders a recommendation (not a decision).
- Failure handling: any pipeline step (llamaparse, Claude API, Jev) can fail; a case then shows a `failed` status with the failure reason and a manual retry action, rather than losing the upload.
- Test/demo data is entirely synthetic, self-generated from public IRS/Fannie Mae form layouts with fake names and numbers — no real borrower PII is ever used.
- Observability: all Claude API calls (main app and eval runs) are proxied through spanbox (a self-built Go LLM-observability tool) for trace/token/cost/latency visibility; eval runs are tagged with a distinct session header so their traces can be filtered and inspected separately from live-demo traffic.
- Quality: a custom Go eval script scores three layers (document-type classification accuracy, field-level extraction accuracy, end-to-end DTI + recommendation accuracy) plus confidence-calibration hit rate, against a fixture set built from the same 5 synthetic test documents used for the live demo.

## Capabilities and Constraints

- Fixed set of 5 supported document types: W2, 1040, Form 1003 (URLA), pay stub, bank statement. Not expanded (deliberate scope decision — depth over breadth).
- Single organization / single reviewer role for this demo; no multi-tenant, no role hierarchy, no borrower-facing login.
- Runs and is demoed entirely on localhost; not deployed to any hosted environment. Local git only — not pushed to GitHub or any remote.
- Storage: SQLite for structured data; uploaded source files stored on local disk (path referenced from the DB), so the reviewer can view the original document alongside extracted fields.
- API keys (Anthropic/Claude, llamaparse) are the user's own responsibility to provision; not tracked here.
- Tests: Go unit tests (DTI calculation, schema validation), API-level integration tests (mocked LLM responses), and at least one Playwright end-to-end flow (upload → review → decide).

## Brand Commitments

No existing brand/identity for this project itself. Visual direction for this build takes inspiration from Tidalwave's (the target employer, tidalwave.com) public marketing site — the user has asked to derive a "seed" from Tidalwave's visual language and use it as a creative jumping-off point for this app's UI/UX, per the current `/impeccable` request. This is aesthetic inspiration, not a claim of affiliation with Tidalwave; the demo is an independent portfolio project.

## Evidence on Hand

- No customer testimonials, case studies, or real financial data exist or should be fabricated. Any numbers shown in the demo are synthetic and clearly reviewer-facing (not borrower-facing marketing claims).
- Industry context gathered via research (2026-09-23): manual mortgage loan processing costs ~$1,500-$3,000/loan (SIFMA 2025 benchmarks); a comparable OCR + human-in-the-loop mortgage underwriting reference demo shipped in the Mistral AI cookbook (merged 2026-09-15, `mistralai/cookbook#407`); document-fraud (fabricated W2s/pay stubs/bank statements) is an active federal enforcement topic; fair-lending / algorithmic-bias concerns (ECOA, Reg B, CFPB scrutiny of AI credit decisions) are a live industry conversation this design explicitly responds to.

## Product Principles

1. AI recommends, humans decide — every AI output is a recommendation with visible reasoning, never a final action, in a domain (mortgage credit decisions) where that boundary is a real compliance requirement, not just a UX nicety.
2. Confidence must be explainable, not a single opaque number — break it into independent, named judgments a reviewer can actually reason about.
3. Depth over breadth — five document types, fully built (editable extraction, failure recovery, audit trail, eval, observability) beats a shallow demo covering more document types.
4. Every design choice should map to a real, citable industry pain point or regulation, so the build reads as informed engineering judgment, not a generic AI-wrapper demo.

## Accessibility & Inclusion

No specific accessibility standard was mandated for this demo; standard web a11y practice (keyboard navigation, contrast, semantic markup) applies by default since this is a professional reviewer tool, but no formal compliance target (e.g. WCAG AA) was set.
