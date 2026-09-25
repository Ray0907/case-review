export type User = { id: string; email: string; name: string; title: string };
export type CaseStatus = "processing" | "needs_review" | "ready" | "approved" | "rejected" | "sent_back";
export type DocStatus = "pending" | "parsing" | "classifying" | "extracting" | "judging" | "done" | "failed" | "unsupported" | "superseded";
export type Recommendation = "" | "eligible" | "ineligible" | "needs_review";

export type CaseRecord = {
  id: string; borrower_name: string; loan_number: string; loan_product: string;
  requested_amount: number; status: CaseStatus; created_at: number;
};
export type CaseSummary = CaseRecord & { recommendation: Recommendation; doc_count: number; blocker: string };
export type Field = { key: string; label: string; value: string | number; flagged: boolean; flag_reason: string; edited: boolean };
export type Judgment = { name: string; score: number; low: boolean; reason: string };
export type DocumentDetail = {
  id: string; case_id: string; file_name: string; doc_type: string; status: DocStatus;
  failure_reason: string; uploaded_at: number; fields: Field[]; judgments: Judgment[];
};
export type Assessment = {
  monthly_income: number | null; monthly_debt: number | null; dti: number | null;
  recommendation: Exclude<Recommendation, "">; reasons: string[];
};
export type CaseDetail = { case: CaseRecord; documents: DocumentDetail[]; case_judgments: Judgment[]; assessment: Assessment | null };
export type AuditEntry = { id: number; action: string; note: string; user_name: string; created_at: number };
export type StageEvent = { case_id: string; document_id?: string; stage: string; status: string; detail?: string };
export type NewCase = { borrower_name: string; loan_number: string; loan_product: string; requested_amount: number };

export class ApiError extends Error {
  status: number;
  body: Record<string, unknown>;
  constructor(status: number, body: Record<string, unknown>) {
    super(typeof body.error === "string" ? body.error : `Request failed (${status})`);
    this.status = status;
    this.body = body;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = init.body instanceof FormData ? undefined : { "Content-Type": "application/json" };
  const res = await fetch(path, { credentials: "same-origin", headers, ...init });
  if (res.status === 204) return undefined as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, body);
  return body as T;
}

const json = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });

export const api = {
  login: (email: string, password: string) => request<User>("/api/auth/login", json("POST", { email, password })),
  logout: () => request<void>("/api/auth/logout", json("POST")),
  me: () => request<User>("/api/auth/me"),
  listCases: () => request<CaseSummary[]>("/api/cases"),
  createCase: (input: NewCase) => request<CaseRecord>("/api/cases", json("POST", input)),
  getCase: (id: string) => request<CaseDetail>(`/api/cases/${id}`),
  uploadDocument: (caseId: string, file: File) => {
    const form = new FormData();
    form.append("file", file);
    return request<DocumentDetail>(`/api/cases/${caseId}/documents`, { method: "POST", body: form });
  },
  retryDocument: (id: string) => request<void>(`/api/documents/${id}/retry`, json("POST")),
  editField: (docId: string, key: string, value: string | number) =>
    request<CaseDetail>(`/api/documents/${docId}/fields/${key}`, json("PATCH", { value })),
  decide: (caseId: string, action: "approve" | "reject" | "send_back", note: string) =>
    request<CaseDetail>(`/api/cases/${caseId}/decision`, json("POST", { action, note })),
  audit: (caseId: string) => request<AuditEntry[]>(`/api/cases/${caseId}/audit`),
};
