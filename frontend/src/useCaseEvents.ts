import { useEffect, useRef } from "react";
import type { StageEvent } from "./api";

export function useCaseEvents(caseId: string | undefined, onEvent: (e: StageEvent) => void) {
  const handler = useRef(onEvent);
  handler.current = onEvent;
  useEffect(() => {
    if (!caseId) return;
    const source = new EventSource(`/api/cases/${caseId}/events`);
    source.addEventListener("stage", (msg) => handler.current(JSON.parse((msg as MessageEvent).data)));
    return () => source.close();
  }, [caseId]);
}
