import type { Assessment, Judgment } from "../api";
import { docLabel, judgmentLabel } from "../format";

const titles = {
  eligible: "Recommendation: eligible",
  ineligible: "Recommendation: ineligible (DTI above 43%)",
  needs_review: "Recommendation: needs review",
} as const;

function Group({ label, judgments }: { label: string; judgments: Judgment[] }) {
  if (!judgments.length) return null;
  return <div>
    <div className="conf-group-label">{label}</div>
    <div className="conf-grid">
      {judgments.map((j) => <div className="conf-row" key={j.name}>
        <div className="conf-meta">
          <div className="conf-name">{judgmentLabel(j.name)}</div>
          <div className="conf-reason">{j.reason}</div>
        </div>
        <span className={`conf-score ${j.low ? "warn" : "good"} tabular`}>{Math.round(j.score * 100)}%</span>
      </div>)}
    </div>
  </div>;
}

export default function Confidence({ documentJudgments, documentType, caseJudgments, a }: { documentJudgments: Judgment[]; documentType?: string; caseJudgments: Judgment[]; a: Assessment | null }) {
  return (
    <>
      <div className="card-head"><h2>AI confidence breakdown</h2></div>
      <div className="card-body tight">
        {documentJudgments.length + caseJudgments.length === 0 && <p className="empty">Scores appear after documents are processed.</p>}
        <Group label={`This document: ${docLabel(documentType ?? "")}`} judgments={documentJudgments} />
        <Group label="Whole case" judgments={caseJudgments} />
      </div>
      {a && (
        <div className="verdict" aria-live="polite">
          <span className="verdict-icon">AI</span>
          <div>
            <h3>{titles[a.recommendation]}</h3>
            <p>
              {a.reasons.length > 0 ? a.reasons.join(". ") + ". " : "All checks passed. "}
              This is a recommendation, not a decision.
            </p>
          </div>
        </div>
      )}
    </>
  );
}
