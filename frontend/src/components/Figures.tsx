import type { Assessment } from "../api";
import { money, pct } from "../format";

const DTI_LIMIT = 0.43;
const DTI_WARN = 0.41;

export default function Figures({ a, requested }: { a: Assessment | null; requested: number }) {
  const dti = a?.dti ?? null;
  // The band picks the tone in CSS (data-band) and the note below, so the thresholds live here once.
  const band = dti == null ? "none" : dti > DTI_LIMIT ? "bad" : dti >= DTI_WARN ? "warn" : "ok";
  const missingReason = a?.monthly_income == null || a.monthly_income <= 0 ? "Monthly income missing: upload a pay stub or W‑2." : a?.monthly_debt == null ? "Monthly debt missing: upload a bank statement." : "Check the extracted income and debt values before deciding.";
  return (
    <section className="figures" data-band={band} aria-label="Case figures">
      <dl className="figure-list">
        <div className="figure figure-lead">
          <dt>Debt‑to‑income</dt>
          <dd className="tabular">{!a ? "—" : dti == null ? "Not computed" : pct(dti)}</dd>
          <div className="dti-bar" aria-hidden="true">
            <div className="dti-fill" style={{ width: `${Math.min((dti ?? 0) * 100, 100)}%` }} />
            <div className="dti-threshold" />
          </div>
          <span className="figure-note">QM limit 43%</span>
        </div>
        <div className="figure"><dt>Monthly income</dt><dd className="tabular">{money(a?.monthly_income)}</dd></div>
        <div className="figure"><dt>Monthly debt</dt><dd className="tabular">{money(a?.monthly_debt)}</dd></div>
        <div className="figure"><dt>Requested</dt><dd className="tabular">{money(requested)}</dd></div>
      </dl>
      {!a && <p className="dti-of">Figures are calculated once every document is processed.</p>}
      {a && dti == null && <p className="dti-of" role="status">{missingReason}</p>}
      {band === "bad" && <p className="dti-flag">Above the 43% limit</p>}
      {band === "warn" && <p className="dti-flag">Within 2 points of the 43% limit</p>}
    </section>
  );
}
