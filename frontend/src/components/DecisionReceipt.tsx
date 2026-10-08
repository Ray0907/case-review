import { useEffect, useState, type RefObject } from "react";
import { api, type AuditEntry, type CaseDetail, type CaseSummary, type DocumentDetail } from "../api";
import { pct, requiredTypes, reviewTime } from "../format";

const titles: Record<string, string> = { approved: "Approved", rejected: "Rejected", sent_back: "Sent back for documents" };

export default function DecisionReceipt({ detail, docs, next, panel, onNext, onQueue }: {
  detail: CaseDetail; docs: DocumentDetail[]; next?: CaseSummary; panel: RefObject<HTMLElement | null>;
  onNext: (id: string) => void; onQueue: () => void;
}) {
  const [entry, setEntry] = useState<AuditEntry | undefined>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setFailed(false);
    api.audit(detail.case.id).then((list) => setEntry(list[0])).catch(() => setFailed(true));
  }, [detail]);
  const verified = docs.flatMap((d) => d.fields).filter((f) => f.edited).length;
  const received = docs.filter((d) => d.status === "done").length;
  return (
    <section ref={panel} className="receipt" tabIndex={-1} role="region" aria-label="Decision recorded">
      <span className="receipt-mark" aria-hidden="true">
        <svg viewBox="0 0 16 16" fill="none"><path d="M3.5 8.5l3 3 6-7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </span>
      <div className="receipt-body">
        <h2>{titles[detail.case.status]}</h2>
        <p className="receipt-by tabular">
          {failed ? "Could not load who recorded this decision. Refresh to try again."
            : entry ? `Recorded by ${entry.user_name} · ${reviewTime(entry.created_at)}` : "Recording…"}
        </p>
        {entry?.note && <p className="receipt-note">“{entry.note}”</p>}
        <dl className="receipt-facts">
          <div><dt>DTI</dt><dd className="tabular">{pct(detail.assessment?.dti)}</dd></div>
          <div><dt>Fields verified by reviewer</dt><dd className="tabular">{verified}</dd></div>
          <div><dt>Documents</dt><dd className="tabular">{received} of {requiredTypes.length}</dd></div>
        </dl>
      </div>
      <div className="receipt-actions">
        {next
          ? <>
              <button type="button" className="btn btn-primary" onClick={() => onNext(next.id)}>Review next case</button>
              <span className="receipt-next">{next.borrower_name} · {next.blocker}</span>
            </>
          : <span className="receipt-next">No other cases are waiting for review.</span>}
        <button type="button" className="btn btn-ghost" onClick={onQueue}>Back to queue</button>
      </div>
    </section>
  );
}
