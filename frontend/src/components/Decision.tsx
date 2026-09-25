import { useEffect, useRef, useState } from "react";
import { api, ApiError, type AuditEntry, type CaseDetail, type DocumentDetail } from "../api";
import { listLabels, sentenceLabel, requiredTypes, reviewTime } from "../format";

const done: Record<string, string> = { approved: "Approved", rejected: "Rejected", sent_back: "Sent back" };

export default function Decision({ detail, docs, unresolved, onDecided }: { detail: CaseDetail; docs: DocumentDetail[]; unresolved: number; onDecided: (d: CaseDetail) => void }) {
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const decidedLine = useRef<HTMLDivElement>(null);
  const [focusDecided, setFocusDecided] = useState(false);
  const status = detail.case.status;
  const decided = status in done;

  useEffect(() => {
    if (decided && focusDecided) { decidedLine.current?.focus(); setFocusDecided(false); }
  }, [decided, focusDecided]);

  useEffect(() => {
    api.audit(detail.case.id).then(setAudit).catch(() => setError("Could not load the audit trail. Refresh this case to try again."));
  }, [detail]);

  async function decide(action: "approve" | "reject" | "send_back") {
    try {
      onDecided(await api.decide(detail.case.id, action, note));
      setFocusDecided(true);
      setError("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not record decision");
    }
  }

  const processing = status === "processing" || docs.some((d) => ["pending", "parsing", "classifying", "extracting", "judging"].includes(d.status));
  const missing = requiredTypes.filter((type) => !docs.some((d) => d.doc_type === type && d.status === "done"));
  const blockedReason = processing ? "Wait for document processing to finish." : docs.some((d) => d.status === "failed") ? "Retry the failed document before approving." : missing.length ? `Upload the ${listLabels(missing.map(sentenceLabel))} before approving.` : unresolved > 0 ? `Verify ${unresolved} flagged ${unresolved === 1 ? "field" : "fields"} against the source document before approving.` : "";
  const approveBlocked = !!blockedReason;
  return (
    <>
      {decided ? (
        <div className="actions" ref={decidedLine} tabIndex={-1}><strong>{done[status]}</strong><span className="queue-loan">by {audit[0]?.user_name} · {audit[0] && reviewTime(audit[0].created_at)}</span></div>
      ) : (
        <>
          <div className="actions">
            <div className="note-wrap">
              <input className="note" id="decision-note" aria-label="Note for the file" placeholder="Note for the file" value={note} onChange={(e) => setNote(e.target.value)} />
              <span>Required to reject or send back.</span>
            </div>
            <div className="action-buttons">
              <button className="btn btn-bad" disabled={!note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("send_back")}>Send back for documents</button>
              <button className="btn btn-ghost" disabled={!note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("reject")}>Reject</button>
              <button id="approve-button" className="btn btn-primary" disabled={approveBlocked} title={blockedReason || undefined} onClick={() => decide("approve")}>Approve</button>
            </div>
          </div>
          {(blockedReason || (!approveBlocked && detail.assessment?.recommendation === "needs_review" && detail.assessment.reasons.length > 0)) &&
            <p id={blockedReason ? "approve-blocker" : undefined} className="action-blocker" role="status" tabIndex={blockedReason ? -1 : undefined}>{blockedReason || `Check before approving: ${detail.assessment?.reasons.join("; ")}.`}</p>}
        </>
      )}
      {error && <p className="form-error" role="alert" style={{ padding: "0 16px 10px" }}>{error}</p>}
      <div className="audit" style={{ flexDirection: "column", alignItems: "flex-start" }}>
        <span>Every decision and field edit is written to the audit log with reviewer and time.</span>
        {audit.slice(0, 5).map((e) => (
          <span key={e.id} className="tabular">{reviewTime(e.created_at)} · {e.user_name} · {e.action.replace("_", " ")}{e.note ? ` — ${e.note}` : ""}</span>
        ))}
      </div>
    </>
  );
}
