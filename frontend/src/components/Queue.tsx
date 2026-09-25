import type { CaseSummary } from "../api";
const order: Record<string, number> = { needs_review: 0, processing: 1, ready: 2, approved: 3, rejected: 3, sent_back: 3 };
const dot = (blocker: string) => blocker === "Document failed" || blocker === "DTI above 43%" ? "bad"
  : blocker === "Ready for decision" ? "good"
  : blocker.startsWith("Check ") || blocker.includes("missing") || blocker.includes("verify") || blocker.includes("unsupported document") || blocker.includes("near 43%") ? "warn" : "";

export default function Queue({ cases, selected, onSelect, onNew }: {
  cases: CaseSummary[]; selected?: string; onSelect: (id: string) => void; onNew: () => void;
}) {
  const sorted = [...cases].sort((a, b) => order[a.status] - order[b.status] || b.created_at - a.created_at);
  return (
    <aside className="queue" aria-label="Review queue">
      <div className="queue-row1" style={{ padding: "0 6px 10px" }}>
        <span className="queue-title" style={{ padding: 0 }}>Review queue</span>
        <button className="btn btn-ghost" style={{ padding: "6px 10px", fontSize: 12 }} onClick={onNew}>New case</button>
      </div>
      {sorted.length === 0 && <p className="empty">No cases yet. Create one to start a review.</p>}
      {sorted.map((c) => (
        <button key={c.id} type="button" className={`queue-item${c.id === selected ? " active" : ""}`}
          style={{ textAlign: "left", font: "inherit", background: undefined }}
          aria-current={c.id === selected ? "page" : undefined} onClick={() => onSelect(c.id)}>
          <span className="queue-row1">
            <span className="queue-name">{c.borrower_name}</span>
            {dot(c.blocker) && <span className={`dot ${dot(c.blocker)}`} aria-label={c.blocker} />}
          </span>
          <span className="queue-loan">{c.blocker}</span>
        </button>
      ))}
    </aside>
  );
}
