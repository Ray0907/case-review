import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type CaseDetail, type CaseSummary, type User } from "../api";
import { moneyWhole } from "../format";
import { useCaseEvents } from "../useCaseEvents";
import Queue from "../components/Queue";
import NewCase from "../components/NewCase";
import SourceDocs from "../components/SourceDocs";
import Fields from "../components/Fields";
import Dti from "../components/Dti";
import Confidence from "../components/Confidence";
import Decision from "../components/Decision";

const statusPill: Record<string, [string, string]> = {
  processing: ["pill-info", "Processing"], needs_review: ["pill-warn", "Needs review"], ready: ["pill-good", "Ready for decision"],
  approved: ["pill-good", "Approved"], rejected: ["pill-bad", "Rejected"], sent_back: ["pill-warn", "Sent back"],
};

export default function Workspace({ user, onSignedOut }: { user: User; onSignedOut: () => void }) {
  const { caseId } = useParams();
  const navigate = useNavigate();
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [detail, setDetail] = useState<CaseDetail | null>(null);
  const [docId, setDocId] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [focusAfterSave, setFocusAfterSave] = useState<string>();
  const [queueLoading, setQueueLoading] = useState(true);
  const [queueError, setQueueError] = useState(false);
  const [detailError, setDetailError] = useState(false);
  const [signOutError, setSignOutError] = useState(false);
  const detailRequest = useRef(0);
  const activeCaseId = useRef(caseId);
  activeCaseId.current = caseId;

  const loadCases = useCallback(async () => {
    setQueueLoading(true);
    setQueueError(false);
    try { setCases(await api.listCases()); }
    catch { setQueueError(true); }
    finally { setQueueLoading(false); }
  }, []);
  const loadDetail = useCallback(async () => {
    const request = ++detailRequest.current;
    if (!caseId) return;
    setDetailError(false);
    try {
      const result = await api.getCase(caseId);
      if (request === detailRequest.current) setDetail(result);
    } catch {
      if (request === detailRequest.current) setDetailError(true);
    }
  }, [caseId]);

  useEffect(() => { void loadCases(); }, [loadCases]);
  useEffect(() => {
    setDetail(null); setDocId(undefined); setFocusAfterSave(undefined);
    void loadDetail();
    return () => { detailRequest.current++; };
  }, [loadDetail]);
  useCaseEvents(caseId, () => { loadDetail(); loadCases(); });
  useEffect(() => {
    if (detail?.case.status !== "processing") return;
    const t = setInterval(loadDetail, 5000);
    return () => clearInterval(t);
  }, [detail?.case.status, loadDetail]);

  const docs = useMemo(() => (detail?.documents ?? []).filter((d) => d.status !== "superseded"), [detail]);
  const selected = docs.find((d) => d.id === docId) ?? docs.find((d) => d.fields.some((f) => f.flagged && !f.edited)) ?? docs[0];
  const unresolved = docs.reduce((n, d) => n + d.fields.filter((f) => f.flagged && !f.edited).length, 0);
  const locked = !!detail && ["approved", "rejected", "sent_back"].includes(detail.case.status);
  const blocker = cases.find((c) => c.id === detail?.case.id)?.blocker;
  const reviewCount = cases.filter((c) => c.status === "needs_review").length;

  useEffect(() => {
    if (!focusAfterSave) return;
    const target = document.getElementById(focusAfterSave);
    const focusable = target instanceof HTMLButtonElement && target.disabled ? document.getElementById("approve-blocker") : target;
    if (focusable) { focusable.focus(); setFocusAfterSave(undefined); }
  }, [detail, docId, focusAfterSave]);

  function applyDetail(d: CaseDetail) {
    if (d.case.id !== activeCaseId.current) return false;
    setDetail(d); void loadCases();
    return true;
  }
  function savedField(d: CaseDetail) {
    if (!applyDetail(d)) return;
    const next = d.documents.filter((doc) => doc.status !== "superseded")
      .flatMap((doc) => doc.fields.filter((field) => field.flagged && !field.edited).map((field) => ({ doc, field })))[0];
    setDocId(next?.doc.id ?? selected?.id);
    setFocusAfterSave(next ? `field-${next.field.key}` : "approve-button");
  }

  return (
    <>
      {caseId && !creating && <a className="skip-case" href="#case-content">Skip to case</a>}
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none"><path d="M3 12c3-4 6-4 9 0s6 4 9 0" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" /></svg>
          </span>
          Case Review
        </div>
        <div className="topbar-meta">
          <span className="pill pill-info">{reviewCount} {reviewCount === 1 ? "needs" : "need"} review</span>
          <div className="reviewer">
            <span className="reviewer-avatar">{user.name.split(" ").map((p) => p[0]).join("")}</span>
            <span className="reviewer-name">{user.name}, {user.title}</span>
          </div>
          <button className="btn btn-ghost" onClick={() => api.logout().then(onSignedOut).catch(() => setSignOutError(true))}>Sign out</button>
          {signOutError && <span className="form-error" role="alert">Could not sign out. Try again.</span>}
        </div>
      </header>
      <div className={`app${caseId && !creating ? " case-open" : ""}`}>
        <Queue cases={cases} selected={caseId} onSelect={(id) => { setCreating(false); navigate(`/cases/${id}`); }} onNew={() => setCreating(true)} />
        <main className="main" id="case-content" tabIndex={-1}>
          {caseId && !creating && <Link className="all-cases" to="/cases">← All cases</Link>}
          {queueLoading && cases.length === 0 && <p className="empty" role="status">Loading cases…</p>}
          {queueError && <div className="load-error" role="alert"><p>Could not load the review queue. Try again.</p><button className="btn btn-ghost" onClick={() => void loadCases()}>Retry queue</button></div>}
          {!creating && caseId && detailError && <div className="load-error" role="alert"><p>Could not load this case. Try again.</p><button className="btn btn-ghost" onClick={() => void loadDetail()}>Retry case</button></div>}
          {!creating && caseId && !detail && !detailError && <p className="empty" role="status">Loading case…</p>}
          {creating && <NewCase onCancel={() => setCreating(false)} onCreated={(c) => { setCreating(false); loadCases(); navigate(`/cases/${c.id}`); }} />}
          {!creating && !caseId && <p className="empty">Select a case from the queue, or create a new one.</p>}
          {!creating && detail && (
            <>
              <div className="case-head">
                <div>
                  <h1>{detail.case.borrower_name}</h1>
                  <div className="case-status">
                    <span className={`pill ${statusPill[detail.case.status][0]}`}>{statusPill[detail.case.status][1]}</span>
                    {blocker && blocker !== statusPill[detail.case.status][1] && <span>{blocker}</span>}
                  </div>
                  <div className="case-sub">
                    <span><span className="m-label">Loan #</span> <span className="m-val">{detail.case.loan_number}</span></span>
                    <span className="m-val">{detail.case.loan_product}</span>
                    <span><span className="m-label">Requested</span> <span className="m-val tabular">{moneyWhole(detail.case.requested_amount)}</span></span>
                  </div>
                </div>
              </div>
              <div className="columns">
                <SourceDocs key={detail.case.id} caseId={detail.case.id} docs={docs} selected={selected} onSelect={setDocId} locked={locked} onChanged={loadDetail} />
                <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
                  <Fields key={selected?.id} doc={selected} locked={locked} onSaved={savedField} />
                  <Dti a={detail.assessment} />
                  <section className="card">
                    <Confidence documentJudgments={selected?.judgments ?? []} documentType={selected?.doc_type} caseJudgments={detail.case_judgments} a={detail.assessment} />
                    <Decision key={detail.case.id} detail={detail} docs={docs} unresolved={unresolved} onDecided={applyDetail} />
                  </section>
                </div>
              </div>
            </>
          )}
        </main>
      </div>
    </>
  );
}
