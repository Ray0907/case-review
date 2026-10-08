import type { Assessment } from "../api";
import { money, pct } from "../format";

export default function Figures({ a, requested }: { a: Assessment | null; requested: number }) {
  const dti = a?.dti ?? null;
  const tone = dti == null ? "var(--on-dark-ink)" : dti > 0.43 ? "var(--on-dark-bad)" : dti >= 0.41 ? "var(--on-dark-warn)" : "var(--on-dark-ink)";
  const missingReason = a?.monthly_income == null || a.monthly_income <= 0 ? "Monthly income missing: upload a pay stub or W‑2." : a?.monthly_debt == null ? "Monthly debt missing: upload a bank statement." : "Check the extracted income and debt values before deciding.";
  return (
    <section className="figures" aria-label="Case figures">
      <dl className="figure-list">
        <div className="figure figure-lead">
          <dt>Debt‑to‑income</dt>
          <dd className="tabular" style={{ color: tone }}>{!a ? "—" : dti == null ? "Not computed" : pct(dti)}</dd>
          <div className="dti-bar" aria-hidden="true">
            <div className="dti-fill" style={{ width: `${Math.min((dti ?? 0) * 100, 100)}%`, background: tone }} />
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
      {dti != null && dti > 0.43 && <p className="dti-flag">Above the 43% limit</p>}
      {dti != null && dti >= 0.41 && dti <= 0.43 && <p className="dti-flag dti-flag-warn">Within 2 points of the 43% limit</p>}
    </section>
  );
}
