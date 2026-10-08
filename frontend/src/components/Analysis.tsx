import { useState } from "react";
import type { Assessment, DocumentDetail } from "../api";
import { money, pct } from "../format";

type Bar = { label: string; value: number; sign?: string; tone: "ctx" | "lead" | "flag"; note?: string; hint: string };
const bar_share = 0.7;

function figure(docs: DocumentDetail[], type: string, key: string): number | null {
  const doc = docs.find((d) => d.doc_type === type && d.status === "done");
  const value = doc?.fields.find((f) => f.key === key)?.value;
  return typeof value === "number" ? value : null;
}

function Chart({ id, title, takeaway, bars, foot, children }: {
  id: string; title: string; takeaway: string; bars: Bar[]; foot?: string; children?: React.ReactNode;
}) {
  const [table, setTable] = useState(false);
  const top = Math.max(...bars.map((b) => b.value), 1);
  return (
    <section className="chart" data-chart={id} aria-label={title}>
      <div className="chart-head">
        <h3>{title}</h3>
        <button type="button" className="chart-toggle" aria-pressed={table} onClick={() => setTable(!table)}>
          {table ? "View as chart" : "View as table"}
        </button>
      </div>
      <p className="chart-takeaway">{takeaway}</p>
      {table
        ? <table className="chart-table">
            <thead><tr><th scope="col">Item</th><th scope="col">Amount</th></tr></thead>
            <tbody>{bars.map((b) => <tr key={b.label}><th scope="row">{b.label}</th><td className="tabular">{b.sign}{money(b.value)}</td></tr>)}</tbody>
          </table>
        : <ul className="bar-list">
            {bars.map((b) => {
              const ratio = (b.value / top) * bar_share * 100;
              return (
                <li key={b.label} className="bar-row" title={b.hint}>
                  <span className="bar-label">
                    {b.label}
                    {b.note && <small className={`bar-note bar-note-${b.tone}`}>{b.note}</small>}
                  </span>
                  <span className="bar-track">
                    <span className={`bar bar-${b.tone}`} style={{ width: `${ratio}%` }} />
                  </span>
                  <span className="bar-value tabular">{b.sign}{money(b.value)}</span>
                </li>
              );
            })}
          </ul>}
      {children}
      {foot && <p className="chart-foot">{foot}</p>}
    </section>
  );
}

export default function Analysis({ docs, assessment }: { docs: DocumentDetail[]; assessment: Assessment | null }) {
  const begin = figure(docs, "bank_statement", "beginning_balance");
  const deposits = figure(docs, "bank_statement", "total_deposits");
  const withdrawals = figure(docs, "bank_statement", "total_withdrawals");
  const printed = figure(docs, "bank_statement", "ending_balance");
  const debt = figure(docs, "bank_statement", "monthly_debt");
  const nsf = figure(docs, "bank_statement", "nsf_count");
  const bnpl = figure(docs, "bank_statement", "bnpl_hits");

  let balance: React.ReactNode = null;
  if (begin != null && deposits != null && withdrawals != null && printed != null) {
    const expected = begin + deposits - withdrawals;
    const diff = printed - expected;
    const mismatch = Math.abs(diff) > 0.005;
    const bars: Bar[] = [
      { label: "Beginning balance", value: begin, tone: "ctx", hint: "Balance at the start of the statement period" },
      { label: "Deposits", value: deposits, sign: "+", tone: "ctx", hint: "Total deposits in the period" },
      { label: "Withdrawals", value: withdrawals, sign: "−", tone: "ctx", hint: "Total withdrawals in the period" },
      { label: "Computed ending", value: expected, tone: "lead", hint: `${money(begin)} + ${money(deposits)} − ${money(withdrawals)}` },
      { label: "Printed ending", value: printed, tone: mismatch ? "flag" : "lead", note: mismatch ? "Does not match" : undefined,
        hint: "Ending balance printed on the statement" },
    ];
    balance = (
      <Chart id="balance" title="Asset analysis: balance check" bars={bars}
        takeaway={mismatch
          ? `Printed ending balance is ${money(Math.abs(diff))} ${diff < 0 ? "below" : "above"} what the transactions imply.`
          : "Printed ending balance matches beginning balance plus deposits minus withdrawals."}>
        <dl className="chart-stats">
          {debt != null && <div><dt>Recurring monthly debt</dt><dd className="tabular">{money(debt)}</dd></div>}
          {bnpl != null && <div><dt>BNPL installments</dt><dd className="tabular">{bnpl}{bnpl > 0 && <small className="chart-stat-flag">flagged</small>}</dd></div>}
          {nsf != null && <div><dt>NSF / overdraft events</dt><dd className="tabular">{nsf}{nsf > 0 && <small className="chart-stat-flag">flagged</small>}</dd></div>}
        </dl>
      </Chart>
    );
  }

  const used = assessment?.monthly_income ?? null;
  const sources: [string, number | null][] = [
    ["Pay stub (monthly)", figure(docs, "pay_stub", "monthly_income")],
    ["W‑2 (Box 1 ÷ 12)", (() => { const v = figure(docs, "w2", "box1_wages"); return v == null ? null : v / 12; })()],
    ["Form 1040 (AGI ÷ 12)", (() => { const v = figure(docs, "form_1040", "adjusted_gross_income"); return v == null ? null : v / 12; })()],
  ];
  const present = sources.filter((s): s is [string, number] => s[1] != null);
  let income: React.ReactNode = null;
  if (present.length >= 2) {
    const values = present.map((s) => s[1]);
    const spread = (Math.max(...values) - Math.min(...values)) / Math.max(...values);
    // When several sources tie, the first one (pay stub) is the one the DTI uses.
    const used_label = used == null ? undefined : present.find(([, value]) => Math.abs(value - used) < 0.5)?.[0];
    const bars: Bar[] = present.map(([label, value]) => {
      const is_used = label === used_label;
      return { label, value, tone: is_used ? "lead" : "ctx", note: is_used ? "Used for DTI" : undefined, hint: `${money(value)} a month` };
    });
    income = (
      <Chart id="income" title="Eligible income by source" bars={bars} foot="Annual figures are divided by 12."
        takeaway={spread < 0.02 ? `All sources agree within ${pct(spread)}.` : `Sources differ by up to ${pct(spread)} (${money(Math.max(...values) - Math.min(...values))} a month).`} />
    );
  }

  if (!balance && !income) return null;
  return (
    <section className="card card-quiet analysis">
      <div className="card-head"><h2>Cross-checks</h2><span>From extracted fields</span></div>
      <div className="card-body">{balance}{income}</div>
    </section>
  );
}
