import type { Assessment } from "../api";
import { money, pct } from "../format";

export default function Dti({ a }: { a: Assessment | null }) {
  const dti = a?.dti ?? null;
  const tone = dti == null ? "var(--ink)" : dti > 0.43 ? "var(--bad)" : dti >= 0.41 ? "var(--warn)" : "var(--good)";
  const missingReason = a?.monthly_income == null || a.monthly_income <= 0 ? "Monthly income missing: upload a pay stub or W‑2." : a?.monthly_debt == null ? "Monthly debt missing: upload a bank statement." : "Check the extracted income and debt values before deciding.";
  return (
    <section className="card">
      <div className="card-head"><h2>Debt‑to‑income</h2></div>
      <div className="card-body">
        {!a && <p className="empty">Calculated once every document is processed.</p>}
        {a && (
          <>
            <div className="dti-numbers">
              <span className="dti-big tabular" style={{ color: tone }}>{dti == null ? "Not computed" : pct(dti)}</span>
              <span className="dti-of tabular">{money(a.monthly_debt)} monthly debt ÷ {money(a.monthly_income)} monthly income</span>
            </div>
            {dti == null && <p className="dti-of" role="status">{missingReason}</p>}
            <div className="dti-bar" aria-hidden="true">
              <div className="dti-fill" style={{ width: `${Math.min((dti ?? 0) * 100, 100)}%`, background: tone }} />
              <div className="dti-threshold" />
            </div>
            <div className="dti-legend"><span>0%</span><span>QM threshold 43%</span><span>100%</span></div>
          </>
        )}
      </div>
    </section>
  );
}
