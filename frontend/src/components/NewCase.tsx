import { useState, type FormEvent } from "react";
import { api, type CaseRecord } from "../api";

export default function NewCase({ onCreated, onCancel }: { onCreated: (c: CaseRecord) => void; onCancel: () => void }) {
  const [form, setForm] = useState({ borrower_name: "", loan_number: "", loan_product: "Conventional 30yr fixed", requested_amount: "" });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value });

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      onCreated(await api.createCase({ ...form, requested_amount: Number(form.requested_amount) }));
    } catch (err) {
      setError(`${err instanceof Error ? err.message : "Could not create case"}. Check the details and try again.`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card form-card" onSubmit={submit}>
      <h2 className="form-title">New case</h2>
      <section className="form-group" aria-labelledby="group-borrower">
        <div className="form-group-head"><h3 id="group-borrower">Applicant</h3><p>The person named on the application.</p></div>
        <div className="form-fields">
          <label className="form-label">Borrower<input id="borrower" className="input" value={form.borrower_name} onChange={set("borrower_name")} required /></label>
        </div>
      </section>
      <section className="form-group" aria-labelledby="group-loan">
        <div className="form-group-head"><h3 id="group-loan">Loan</h3><p>Used to find the case in the review queue and to compute debt-to-income.</p></div>
        <div className="form-fields">
          <label className="form-label">Loan number<input id="loan-number" className="input input-md" value={form.loan_number} onChange={set("loan_number")} required /></label>
          <label className="form-label">Loan product<input id="loan-product" className="input" value={form.loan_product} onChange={set("loan_product")} required /></label>
          <label className="form-label">Requested amount (USD)<input id="amount" className="input input-md tabular" type="number" min="0.01" step="0.01" value={form.requested_amount} onChange={set("requested_amount")} required /></label>
        </div>
      </section>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="form-actions">
        <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? "Creating…" : "Create case"}</button>
        <button className="btn btn-ghost" type="button" onClick={onCancel}>Cancel</button>
      </div>
    </form>
  );
}
