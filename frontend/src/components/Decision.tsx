import { useEffect, useState } from "react";
import { api, ApiError, type AuditEntry, type CaseDetail, type DocumentDetail } from "../api";
import { decisionLabels, listLabels, sentenceLabel, requiredTypes, processingStates } from "../format";
import When from "./When";

export default function Decision({ detail, docs, unresolved, onDecided }: { detail: CaseDetail; docs: DocumentDetail[]; unresolved: number; onDecided: (d: CaseDetail) => void }) {
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const status = detail.case.status;
  const decided = status in decisionLabels;

  // The audit entry only feeds the decided summary line, so skip the request while the case is open.
  useEffect(() => {
    if (!decided) return;
    api.audit(detail.case.id).then(setAudit).catch(() => setError("Could not load the audit trail. Refresh this case to try again."));
  }, [detail, decided]);

  async function decide(action: "approve" | "reject" | "send_back") {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      onDecided(await api.decide(detail.case.id, action, note));
      setError("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not record decision. Try again.");
    } finally {
      setBusy(false);
    }
  }

  const processing = status === "processing" || docs.some((d) => processingStates.includes(d.status));
  const missing = requiredTypes.filter((type) => !docs.some((d) => d.doc_type === type && d.status === "done"));
  const blockedReason = processing ? "Wait for document processing to finish." : docs.some((d) => d.status === "failed") ? "Retry the failed document before approving." : missing.length ? `Upload the ${listLabels(missing.map(sentenceLabel))} before approving.` : unresolved > 0 ? `Verify ${unresolved} flagged ${unresolved === 1 ? "field" : "fields"} against the source document before approving.` : "";
  const approveBlocked = !!blockedReason;
  return (
    <>
      <div className="decision-bar">
      {decided ? (
        <div className="actions"><strong>{decisionLabels[status]}</strong><span className="queue-loan">by {audit[0]?.user_name} · {audit[0] && <When seconds={audit[0].created_at} />}</span></div>
      ) : (
        <>
          <div className="actions" aria-busy={busy}>
            <div className="note-wrap">
              <label className="note-label" htmlFor="decision-note">Note for the file</label>
              <input className="note" id="decision-note" aria-describedby="decision-note-help" placeholder="Add context for the reviewer" value={note} disabled={busy} onChange={(e) => setNote(e.target.value)} />
              <span id="decision-note-help">Required to reject or send back.</span>
            </div>
            <div className="action-buttons">
              <button className="btn btn-link" disabled={busy || !note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("send_back")}>Send back for documents</button>
              <button className="btn btn-ghost" disabled={busy || !note.trim()} title={!note.trim() ? "Add a note first" : undefined} onClick={() => decide("reject")}>Reject</button>
              <button id="approve-button" className="btn btn-primary" disabled={busy || approveBlocked} aria-describedby={blockedReason ? "approve-blocker" : undefined} title={blockedReason || undefined} onClick={() => decide("approve")}>Approve</button>
            </div>
          </div>
          {busy && <p className="action-blocker" role="status">Recording decision…</p>}
          {(blockedReason || (!approveBlocked && detail.assessment?.recommendation === "needs_review" && detail.assessment.reasons.length > 0)) &&
            <p id={blockedReason ? "approve-blocker" : undefined} className="action-blocker" role="status" tabIndex={blockedReason ? -1 : undefined}>{blockedReason || `Check before approving: ${detail.assessment?.reasons.join("; ")}.`}</p>}
        </>
      )}
      {error && <p className="form-error" role="alert" style={{ padding: "0 16px 10px" }}>{error}</p>}
      </div>
    </>
  );
}
