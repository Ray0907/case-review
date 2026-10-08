import { useEffect, useState } from "react";
import { api, type AuditEntry, type CaseDetail } from "../api";
import { decisionLabels } from "../format";
import When from "./When";

const first_count = 5;
const sentence = (action: string) => { const t = action.replace("_", " "); return t.charAt(0).toUpperCase() + t.slice(1); };

export default function AuditLog({ detail }: { detail: CaseDetail }) {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [failed, setFailed] = useState(false);
  const [all, setAll] = useState(false);
  useEffect(() => {
    setFailed(false);
    api.audit(detail.case.id).then(setEntries).catch(() => setFailed(true));
  }, [detail]);
  const shown = all ? entries : entries.slice(0, first_count);
  return (
    <section className="card card-quiet audit" aria-label="Review history">
      <div className="card-head"><h2>Review history</h2><span>Decisions and field edits, with reviewer and time</span></div>
      <div className="card-body">
        {failed && <p className="form-error" role="alert">Could not load the review history. Refresh this case to try again.</p>}
        {!failed && entries.length === 0 && <p className="empty">No review activity yet.</p>}
        <ol className="audit-list">
          {shown.map((e) => (
            <li key={e.id} className={`audit-item${e.action in decisionLabels ? " audit-decision" : ""}`}>
              <span className="audit-dot" aria-hidden="true" />
              <div className="audit-main">
                <span className="audit-action">{decisionLabels[e.action] ?? sentence(e.action)}</span>
                <span className="audit-who">{e.user_name}</span>
                <When seconds={e.created_at} />
              </div>
              {e.note && <p className="audit-detail">{e.note}</p>}
            </li>
          ))}
        </ol>
        {entries.length > first_count && (
          <button type="button" className="chart-toggle" aria-pressed={all} onClick={() => setAll(!all)}>
            {all ? "Show fewer" : `Show all ${entries.length}`}
          </button>
        )}
      </div>
    </section>
  );
}
