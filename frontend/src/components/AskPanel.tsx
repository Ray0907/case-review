import { useEffect, useMemo, useRef, useState } from "react";
import { useChat } from "@ai-sdk/react";
import Markdown from "react-markdown";
import { DefaultChatTransport, type UIMessage } from "ai";
import type { CaseDetail, Citation, DocumentDetail } from "../api";
import { loadHistory, saveHistory } from "../askHistory";
import { docLabel, fieldDisplay, money, pct } from "../format";

type AskMessage = UIMessage<unknown, { citations: Citation[] }>;

const suggestions = [
  "What was the debt-to-income ratio?",
  "Which fields did the reviewer confirm or correct?",
  "Where does the monthly income come from?",
  "Summarize this case",
];

// The server answers with {"error": "..."} before a stream starts; show that text, not the raw response.
async function friendlyFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const res = await fetch(input, init);
  if (res.ok) return res;
  const body = await res.json().catch(() => ({}));
  throw new Error(typeof body.error === "string" ? body.error : `Request failed (${res.status})`);
}

function lastQuestion(messages: AskMessage[]): string {
  const last = [...messages].reverse().find((m) => m.role === "user");
  return (last?.parts ?? []).map((p) => (p.type === "text" ? p.text : "")).join("");
}

function resolve(detail: CaseDetail, docs: DocumentDetail[], c: Citation): { label: string; value: string } | null {
  if (c.document === "assessment") {
    const a = detail.assessment;
    if (!a) return null;
    if (c.key === "dti") return { label: "Debt-to-income", value: pct(a.dti) };
    if (c.key === "monthly_income") return { label: "Monthly income", value: money(a.monthly_income) };
    if (c.key === "monthly_debt") return { label: "Monthly debt", value: money(a.monthly_debt) };
    if (c.key === "recommendation") return { label: "Recommendation", value: a.recommendation.replace("_", " ") };
    return null;
  }
  const doc = docs.find((d) => d.doc_type === c.document);
  const field = doc?.fields.find((f) => f.key === c.key);
  return doc && field ? { label: `${docLabel(doc.doc_type)} · ${field.label}`, value: fieldDisplay(field.key, field.value) } : null;
}

function messageText(m: AskMessage): string {
  return m.parts.map((p) => (p.type === "text" ? p.text : "")).join("");
}

function messageCitations(m: AskMessage): Citation[] {
  return m.parts.flatMap((p) => (p.type === "data-citations" ? p.data : []));
}

export default function AskPanel({ userId, detail, docs, onCite }: {
  userId: string; detail: CaseDetail; docs: DocumentDetail[]; onCite: (document: string, key: string) => void;
}) {
  const case_id = detail.case.id;
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const launcher = useRef<HTMLButtonElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const log = useRef<HTMLDivElement>(null);
  const transport = useMemo(() => new DefaultChatTransport<AskMessage>({
    api: `/api/cases/${case_id}/ask`,
    credentials: "same-origin",
    headers: { Accept: "text/event-stream" },
    fetch: friendlyFetch,
    prepareSendMessagesRequest: ({ messages }) => ({ body: { question: lastQuestion(messages) } }),
  }), [case_id]);
  const chat = useChat<AskMessage>({ id: `${userId}:${case_id}`, messages: loadHistory(userId, case_id) as AskMessage[], transport });
  const busy = chat.status === "submitted" || chat.status === "streaming";

  useEffect(() => { if (open) input.current?.focus(); }, [open]);
  useEffect(() => { log.current?.scrollTo({ top: log.current.scrollHeight }); }, [chat.messages, chat.status]);
  useEffect(() => {
    if (chat.status === "ready" || chat.status === "error") saveHistory(userId, case_id, chat.messages);
  }, [chat.messages, chat.status, userId, case_id]);

  function close() { setOpen(false); launcher.current?.focus(); }
  function send(question: string) {
    const text = question.trim();
    if (!text || busy) return;
    setDraft("");
    void chat.sendMessage({ text });
  }
  function reset() {
    chat.stop();
    chat.setMessages([]);
    chat.clearError();
    saveHistory(userId, case_id, []);
    input.current?.focus();
  }

  return (
    <>
      <button ref={launcher} type="button" className="ask-launch" aria-label="Ask about this case" aria-expanded={open}
        aria-controls="ask-window" onClick={() => (open ? close() : setOpen(true))}>
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
          {open
            ? <path d="M6 9l6 6 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
            : <path d="M5 6.5A2.5 2.5 0 017.5 4h9A2.5 2.5 0 0119 6.5v6a2.5 2.5 0 01-2.5 2.5H11l-4 3.5V15h-.5A2.5 2.5 0 014 12.5v-6z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" />}
        </svg>
      </button>
      {open && (
        <section id="ask-window" className="ask-window" role="dialog" aria-modal="false" aria-label="Ask about this case"
          onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); close(); } }}>
          <header className="ask-head">
            <span className="ask-avatar" aria-hidden="true">AI</span>
            <div className="ask-title">
              <h2>Case assistant</h2>
              <p>Answers come from the recorded file only</p>
            </div>
            {chat.messages.length > 0 && <button type="button" className="ask-new" onClick={reset}>New chat</button>}
            <button type="button" className="ask-close" aria-label="Close case assistant" onClick={close}>
              <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
            </button>
          </header>
          <div ref={log} className="ask-log" role="log" aria-live="polite" aria-label="Conversation">
            {chat.messages.length === 0 && (
              <div className="ask-intro">
                <p className="ask-mono">Closed case · {detail.case.status.replace("_", " ")}</p>
                <p className="ask-lead">Ask about the figures, edits and documents in this file.</p>
                <ul className="ask-suggestions">
                  {suggestions.map((s) => <li key={s}><button type="button" className="ask-chip" onClick={() => send(s)}>{s}</button></li>)}
                </ul>
              </div>
            )}
            {chat.messages.map((m, i) => {
              const text = messageText(m);
              const streaming = chat.status === "streaming" && i === chat.messages.length - 1 && m.role === "assistant";
              const cites = messageCitations(m);
              if (m.role === "assistant" && !text && cites.length === 0) return null;
              return (
                <div key={m.id} className={`ask-message ask-message-${m.role}`}>
                  <div className={`ask-bubble ${m.role === "user" ? "ask-bubble-user" : "ask-bubble-ai"}${streaming ? " ask-streaming" : ""}`}>
                    {m.role === "user"
                      ? <p>{text}</p>
                      : <Markdown disallowedElements={["a", "img", "h1", "h2", "h3", "h4", "h5", "h6"]} unwrapDisallowed>{text}</Markdown>}
                  </div>
                  {cites.length > 0 && (
                    <div className="ask-sources">
                      <span className="ask-mono">Sources</span>
                      <ul>
                        {cites.map((c) => {
                          const r = resolve(detail, docs, c);
                          return r && <li key={`${c.document}:${c.key}`}>
                            <button type="button" className="ask-source" onClick={() => onCite(c.document, c.key)}>
                              <span>{r.label}</span><b className="tabular">{r.value}</b>
                            </button>
                          </li>;
                        })}
                      </ul>
                    </div>
                  )}
                </div>
              );
            })}
            {chat.status === "submitted" && <div className="ask-message ask-message-assistant"><div className="ask-bubble ask-bubble-ai" role="status">Reading the case file…</div></div>}
            {chat.error && (
              <div className="ask-message ask-message-assistant">
                <div className="ask-bubble ask-bubble-ai ask-bubble-error" role="alert">
                  <p>{chat.error.message}</p>
                  <button type="button" className="ask-retry" onClick={() => { chat.clearError(); void chat.regenerate(); }}>Try again</button>
                </div>
              </div>
            )}
          </div>
          <form className="ask-form" onSubmit={(e) => { e.preventDefault(); send(draft); }}>
            <label className="sr-only" htmlFor="ask-input">Question about this case</label>
            <input ref={input} id="ask-input" className="ask-input" maxLength={500} autoComplete="off" placeholder="Ask a question"
              value={draft} onChange={(e) => setDraft(e.target.value)} />
            {busy
              ? <button type="button" className="ask-send ask-stop" aria-label="Stop answering" onClick={() => void chat.stop()}>
                  <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><rect x="7" y="7" width="10" height="10" rx="2" /></svg>
                </button>
              : <button type="submit" className="ask-send" aria-label="Send question" disabled={!draft.trim()}>
                  <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="M12 19V6M6.5 11.5L12 6l5.5 5.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" /></svg>
                </button>}
          </form>
          <p className="ask-note">AI can be wrong. Answers are not credit decisions.</p>
        </section>
      )}
    </>
  );
}
