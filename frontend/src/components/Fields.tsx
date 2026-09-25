import { useState } from "react";
import { api, type CaseDetail, type DocumentDetail, type Field } from "../api";
import { docLabel, fieldDisplay } from "../format";

function FieldInput({ doc, field, flagged, onSaved, onCancel }: {
  doc: DocumentDetail; field: Field; flagged: boolean; onSaved: (d: CaseDetail) => void; onCancel?: () => void;
}) {
  const [value, setValue] = useState(fieldDisplay(field.key, field.value));
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function save() {
    if (busy) return;
    const raw = value.replace(/[$,\s]/g, "");
    const parsed = typeof field.value === "number" ? Number(raw) : value;
    if (typeof parsed === "number" && (raw === "" || !Number.isFinite(parsed))) {
      setError("Enter a valid number, then confirm the value.");
      return;
    }
    setBusy(true);
    try {
      onSaved(await api.editField(doc.id, field.key, parsed));
      setError("");
    } catch (err) {
      setError(`${err instanceof Error ? err.message : "Could not save"}. Check the value and try again.`);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="field-edit" style={{ flexDirection: "column", alignItems: "flex-end", gap: 4 }}>
      <input className={`field-input tabular${flagged ? "" : " field-input-plain"}`} id={`field-${field.key}`}
        aria-label={flagged ? `${field.label}, flagged for review` : field.label} autoFocus={!flagged}
        value={value} onChange={(e) => setValue(e.target.value)} disabled={busy}
        onKeyDown={(e) => {
          if (e.key === "Enter") { e.preventDefault(); void save(); }
          if (e.key === "Escape" && onCancel) { e.preventDefault(); onCancel(); }
        }} />
      <button className="btn btn-ghost" type="button" disabled={busy} onClick={() => void save()} style={{ padding: "7px 9px", fontSize: 11 }}>{busy ? "Confirming…" : "Confirm value"}</button>
      {error && <span className="edit-hint" role="alert">{error}</span>}
    </div>
  );
}

export default function Fields({ doc, locked, onSaved }: { doc?: DocumentDetail; locked: boolean; onSaved: (d: CaseDetail) => void }) {
  const [editing, setEditing] = useState("");
  return (
    <section className="card">
      <div className="card-head">
        <h2>Extracted fields</h2>
        {doc && <span style={{ fontSize: 12, fontWeight: 500, color: "var(--ink-mute)" }}>{docLabel(doc.doc_type, doc.file_name)}</span>}
      </div>
      <div className="card-body">
        {(!doc || doc.fields.length === 0) && <p className="empty">Fields appear here once the document is processed.</p>}
        {doc?.fields.map((f) => {
          const flagged = f.flagged && !f.edited && !locked;
          const editing_row = !locked && editing === f.key;
          const warn = f.key === "bnpl_hits" && Number(f.value) > 0;
          const warn_style = warn ? { color: "var(--warn)" } : undefined;
          const display = `${fieldDisplay(f.key, f.value)}${warn ? " detected" : ""}`;
          return (
            <div key={f.key} className={`field-row${flagged ? " field-flagged" : ""}`}>
              <span className="field-label">
                {flagged ? "⚠ " : ""}{f.label}
                {flagged && <span style={{ display: "block", fontSize: 11, marginTop: 2 }}>{f.flag_reason}</span>}
                {f.edited && <span style={{ display: "block", fontSize: 11, marginTop: 2, color: "var(--ink-mute)" }}>Verified by reviewer</span>}
              </span>
              {flagged || editing_row
                ? <FieldInput doc={doc} field={f} flagged={flagged}
                    onSaved={(d) => { setEditing(""); onSaved(d); }} onCancel={flagged ? undefined : () => setEditing("")} />
                : locked
                  ? <span className="field-value tabular" style={warn_style}>{display}</span>
                  : <button type="button" className="field-value field-value-btn tabular" aria-label={`Edit ${f.label}`}
                      style={warn_style} onClick={() => setEditing(f.key)}>
                      {display}
                    </button>}
            </div>
          );
        })}
      </div>
    </section>
  );
}
