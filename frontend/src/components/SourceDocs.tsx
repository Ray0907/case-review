import { useEffect, useRef, useState } from "react";
import { api, type DocumentDetail } from "../api";
import { docLabel, requiredLabel, requiredTypes, stageLabel } from "../format";

const busy = new Set(["pending", "parsing", "classifying", "extracting", "judging"]);

export default function SourceDocs({ caseId, docs, selected, onSelect, locked, onChanged }: {
  caseId: string; docs: DocumentDetail[]; selected?: DocumentDetail; onSelect: (id: string) => void; locked: boolean; onChanged: () => void;
}) {
  const [uploadErrors, setUploadErrors] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [retryError, setRetryError] = useState("");
  const fileInput = useRef<HTMLInputElement>(null);
  const [previewFailed, setPreviewFailed] = useState(false);
  useEffect(() => { setPreviewFailed(false); }, [selected?.id]);
  const received = docs.filter((d) => d.status === "done").length;
  const missing = requiredTypes.filter((type) => !docs.some((d) => d.doc_type === type));

  async function upload(files: FileList | null) {
    if (!files) return;
    setUploading(true);
    const errors: string[] = [];
    for (const f of Array.from(files)) {
      try {
        await api.uploadDocument(caseId, f);
      } catch (err) {
        errors.push(`${f.name}: ${err instanceof Error ? err.message : "upload failed"}. Check the file is a PDF, PNG or JPEG under 20 MB, then try again.`);
      }
    }
    setUploadErrors(errors);
    setUploading(false);
    onChanged();
  }

  const isImage = selected && /\.(png|jpe?g)$/i.test(selected.file_name);
  return (
    <section className="card">
      <div className="card-head">
        <h2>Source documents</h2>
        <span className="pill pill-info">{received} of 5 processed</span>
      </div>
      <div className="doc-strip" aria-label="Documents">
        {docs.map((d) => (
          <button key={d.id} aria-pressed={d.id === selected?.id} type="button"
            className={`doc-thumb${d.id === selected?.id ? " active" : ""}`} onClick={() => onSelect(d.id)}>
            <span className="doc-icon" />
            {d.doc_type ? `${docLabel(d.doc_type)}${d.status === "failed" ? " (failed)" : ""}` : d.status === "failed" ? `${d.file_name.slice(0, 18)} (failed)` : "Processing…"}
          </button>
        ))}
        {missing.map((type) => <span key={type} className="doc-thumb doc-missing" aria-label={`${requiredLabel(type)} not uploaded`}>{requiredLabel(type)}</span>)}
        {!locked && (
          <>
            <button className="doc-thumb" type="button" aria-label="Add documents" disabled={uploading} onClick={() => fileInput.current?.click()}>
              <span aria-hidden="true" style={{ fontSize: 18 }}>+</span>{uploading ? "Uploading…" : "Add"}
            </button>
            <input ref={fileInput} type="file" multiple accept=".pdf,.png,.jpg,.jpeg" hidden onChange={(e) => { void upload(e.target.files); e.target.value = ""; }} data-testid="file-input" aria-label="Add PDF, PNG or JPEG documents" />
          </>
        )}
      </div>
      {uploadErrors.map((e) => <p key={e} className="form-error" style={{ padding: "8px 16px 0" }}>{e}</p>)}
      {!selected && <p className="empty">Add the borrower's W‑2, 1040, Form 1003, pay stub and bank statement.</p>}
      {selected && (
        <div style={{ padding: 12, display: "grid", gap: 10 }}>
          {busy.has(selected.status) && <div className="stage-row" aria-live="polite"><span>{stageLabel(selected.status)}</span><span className="pill pill-info">Live</span></div>}
          {selected.status === "failed" && (
            <div className="scan-flag" style={{ marginTop: 0, justifyContent: "space-between" }} role="alert">
              <span>{selected.doc_type ? "This document couldn't be read. Retry to process it again." : "This document couldn't be read, so its type is unknown. Retry to process it again."}<small className="failure-details">Details: {selected.failure_reason}</small></span>
              <button className="btn btn-ghost" onClick={() => api.retryDocument(selected.id).then(() => { setRetryError(""); onChanged(); }).catch((e) => setRetryError(`${e instanceof Error ? e.message : "Retry failed"}. Refresh the case and try again.`))}>Retry</button>
            </div>
          )}
          {retryError && <p className="form-error" role="alert">{retryError}</p>}
          {selected.status === "unsupported" && <div className="scan-flag" style={{ marginTop: 0 }}>Not a supported document type. Upload a W‑2, 1040, Form 1003, pay stub or bank statement.</div>}
          {previewFailed && <p className="form-error" role="alert">Preview unavailable.</p>}
          {!previewFailed && (isImage
            ? <img className="doc-frame" style={{ objectFit: "contain" }} src={`/api/documents/${selected.id}/file`} onError={() => setPreviewFailed(true)} alt={`${docLabel(selected.doc_type, "Document")} source scan`} />
            : <iframe className="doc-frame" src={`/api/documents/${selected.id}/file`} title={`${docLabel(selected.doc_type, "Document")} source`} />)}
          <p className="scan-caption" style={{ padding: 0 }}>{selected.file_name} · <a href={`/api/documents/${selected.id}/file`} target="_blank" rel="noopener noreferrer">Open the original file</a></p>
        </div>
      )}
    </section>
  );
}
