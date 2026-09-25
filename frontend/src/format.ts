const usd = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 2 });
const whole = new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", maximumFractionDigits: 0 });

export const money = (n: number | null | undefined) => (n == null ? "—" : usd.format(n));
export const moneyWhole = (n: number | null | undefined) => (n == null ? "—" : whole.format(n));
export const pct = (n: number | null | undefined, digits = 1) => (n == null ? "—" : `${(n * 100).toFixed(digits)}%`);
const dateTime = new Intl.DateTimeFormat("en-US", { month: "short", day: "numeric", year: "numeric", hour: "numeric", minute: "2-digit" });
export const reviewTime = (seconds: number) => dateTime.format(new Date(seconds * 1000));

const labels: Record<string, string> = {
  w2: "W‑2", form_1040: "Form 1040", form_1003: "Form 1003", pay_stub: "Pay stub", bank_statement: "Bank statement", other: "Unsupported",
};
export const docLabel = (t: string, fallback = "Classifying…") => labels[t] ?? fallback;
export const requiredTypes = ["w2", "form_1040", "form_1003", "pay_stub", "bank_statement"];
export const requiredLabel = (t: string) => docLabel(t).replace("‑", "-");
export const sentenceLabel = (t: string) => t === "pay_stub" || t === "bank_statement" ? requiredLabel(t).toLowerCase() : requiredLabel(t);
export const listLabels = (items: string[]) => items.length < 2 ? items.join("") : `${items.slice(0, -1).join(", ")} and ${items.at(-1)}`;

const stages: Record<string, string> = {
  pending: "Queued", parsing: "Reading document", classifying: "Identifying type", extracting: "Extracting fields",
  judging: "Scoring confidence", done: "Processed", failed: "Failed", unsupported: "Unsupported", superseded: "Replaced",
};
export const stageLabel = (s: string) => stages[s] ?? s;

export const judgmentLabel = (n: string) =>
  ({ field_completeness: "Field completeness", document_authenticity: "Document authenticity", ocr_quality: "OCR extraction quality", income_consistency: "Income consistency" } as Record<string, string>)[n] ?? n;

export const moneyKeys = new Set(["box1_wages", "box2_fed_tax", "adjusted_gross_income", "taxable_income", "total_tax", "loan_amount",
  "stated_monthly_income", "gross_pay", "ytd_gross", "monthly_income", "beginning_balance", "ending_balance", "total_deposits", "total_withdrawals", "monthly_debt"]);
export const fieldDisplay = (key: string, v: string | number) => (typeof v === "number" && moneyKeys.has(key) ? money(v) : String(v));
