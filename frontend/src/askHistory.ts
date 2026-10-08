import type { UIMessage } from "ai";

// One conversation per reviewer and case, kept in this browser only.
const prefix = "case-review:ask:";
const keep_last = 40;

const key = (user_id: string, case_id: string) => `${prefix}${user_id}:${case_id}`;

export function loadHistory(user_id: string, case_id: string): UIMessage[] {
  try {
    const raw = window.localStorage.getItem(key(user_id, case_id));
    const list: unknown = raw ? JSON.parse(raw) : [];
    if (!Array.isArray(list)) return [];
    return list.filter((m): m is UIMessage => !!m && typeof m.id === "string" && (m.role === "user" || m.role === "assistant") && Array.isArray(m.parts));
  } catch {
    return [];
  }
}

export function saveHistory(user_id: string, case_id: string, messages: UIMessage[]) {
  try {
    if (messages.length === 0) window.localStorage.removeItem(key(user_id, case_id));
    else window.localStorage.setItem(key(user_id, case_id), JSON.stringify(messages.slice(-keep_last)));
  } catch {
    // Storage can be full or blocked; the conversation still works for this visit.
  }
}

// Called on sign-out so the next person on this browser does not see earlier conversations.
export function clearAllHistory() {
  try {
    Object.keys(window.localStorage).filter((k) => k.startsWith(prefix)).forEach((k) => window.localStorage.removeItem(k));
  } catch {
    // Nothing to clear when storage is blocked.
  }
}
