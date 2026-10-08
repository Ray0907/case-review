import { useEffect, useRef, useState } from "react";
import type { PDFDocumentLoadingTask, PDFDocumentProxy, PDFPageProxy, RenderTask } from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import "pdfjs-dist/web/pdf_viewer.css";
import type { SourceBox } from "../api";

type Rect = { x: number; y: number; w: number; h: number };
const rect_keys = ["x", "y", "w", "h"] as const;
const rect_zero: Rect = { x: 0, y: 0, w: 0, h: 0 };
const spring_response = 0.35;
const spring_stiffness = (2 * Math.PI / spring_response) ** 2;
const spring_damping = 2 * Math.sqrt(spring_stiffness);
const render_scale = 2;

// Critically damped spring: a new target re-aims from the live position and velocity,
// so selecting another field mid-flight never jumps. Reduced motion jumps straight there.
function useSpringRect(target: Rect): Rect {
  const [rect, setRect] = useState(target);
  const live = useRef({ pos: target, vel: rect_zero });
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      live.current = { pos: target, vel: rect_zero };
      setRect(target);
      return;
    }
    let frame = 0;
    let time_last = performance.now();
    function step(now: number) {
      const dt = Math.min((now - time_last) / 1000, 1 / 30);
      time_last = now;
      const { pos, vel } = live.current;
      const pos_next = { ...pos };
      const vel_next = { ...vel };
      let moving = false;
      for (const key of rect_keys) {
        vel_next[key] += (spring_stiffness * (target[key] - pos[key]) - spring_damping * vel[key]) * dt;
        pos_next[key] += vel_next[key] * dt;
        if (Math.abs(target[key] - pos_next[key]) > 0.01 || Math.abs(vel_next[key]) > 0.05) moving = true;
      }
      live.current = moving ? { pos: pos_next, vel: vel_next } : { pos: target, vel: rect_zero };
      setRect(live.current.pos);
      if (moving) frame = requestAnimationFrame(step);
    }
    frame = requestAnimationFrame(step);
    return () => cancelAnimationFrame(frame);
  }, [target.x, target.y, target.w, target.h]);
  return rect;
}

function Highlight({ box }: { box: SourceBox }) {
  const rect = useSpringRect({
    x: 100 * box.x / box.page_width, y: 100 * box.y / box.page_height,
    w: 100 * box.w / box.page_width, h: 100 * box.h / box.page_height,
  });
  return <div className="source-highlight" data-testid="source-highlight" aria-hidden="true" style={{
    left: `${rect.x}%`, top: `${rect.y}%`, width: `${rect.w}%`, height: `${rect.h}%`,
  }} />;
}

export default function SourceViewer({ url, image, box, label, selection }: {
  url: string; image: boolean; box?: SourceBox; label: string; selection?: { text: string; onClear: () => void };
}) {
  const scroller = useRef<HTMLDivElement>(null);
  const canvases = useRef<(HTMLCanvasElement | null)[]>([]);
  const text_layers = useRef<(HTMLDivElement | null)[]>([]);
  const [state, setState] = useState<"loading" | "ready" | "failed">("loading");
  const [page_count, setPageCount] = useState(image ? 1 : 0);
  const [pages_done, setPagesDone] = useState(image ? 1 : 0);
  const [page_current, setPageCurrent] = useState(1);

  useEffect(() => {
    if (image) return;
    let cancelled = false;
    let load_task: PDFDocumentLoadingTask | undefined;
    let render_task: RenderTask | undefined;
    const observers: ResizeObserver[] = [];
    async function render() {
      try {
        const pdfjs = await import("pdfjs-dist");
        if (cancelled) return;
        pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;
        load_task = pdfjs.getDocument({ url });
        const pdf: PDFDocumentProxy = await load_task.promise;
        if (cancelled) return;
        setPageCount(pdf.numPages);
        await new Promise((resolve) => requestAnimationFrame(resolve));
        for (let n = 1; n <= pdf.numPages; n++) {
          const page = await pdf.getPage(n);
          const node = canvases.current[n - 1];
          if (cancelled || !node) return;
          const viewport = page.getViewport({ scale: render_scale });
          node.width = viewport.width;
          node.height = viewport.height;
          render_task = page.render({ canvas: node, viewport });
          await render_task.promise;
          if (cancelled) return;
          await renderText(pdfjs, page, text_layers.current[n - 1], observers);
          if (cancelled) return;
          setPagesDone(n);
        }
        setState("ready");
      } catch {
        if (!cancelled) setState("failed");
      }
    }
    void render();
    return () => {
      cancelled = true;
      render_task?.cancel();
      observers.forEach((o) => o.disconnect());
      void load_task?.destroy();
    };
  }, [url, image]);

  const page_target = box?.page ?? 0;
  useEffect(() => {
    if (state !== "ready" || !page_target) return;
    goToPage(page_target);
  }, [page_target, state]);

  function goToPage(n: number) {
    const box_scroll = scroller.current;
    const node = box_scroll?.querySelector<HTMLElement>(`[data-page="${n}"]`);
    if (!box_scroll || !node) return;
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    box_scroll.scrollTo({ top: node.offsetTop - 12, behavior: reduce ? "auto" : "smooth" });
  }

  function trackPage() {
    const box_scroll = scroller.current;
    if (!box_scroll) return;
    const pages = [...box_scroll.querySelectorAll<HTMLElement>("[data-page]")];
    const mid = box_scroll.scrollTop + box_scroll.clientHeight / 2;
    const hit = pages.findIndex((p) => p.offsetTop + p.offsetHeight > mid);
    setPageCurrent(hit === -1 ? pages.length : hit + 1);
  }

  async function renderText(pdfjs: typeof import("pdfjs-dist"), page: PDFPageProxy, node: HTMLDivElement | null, observers: ResizeObserver[]) {
    const holder = node?.parentElement;
    if (!node || !holder) return;
    const unit = page.getViewport({ scale: 1 });
    node.replaceChildren();
    await new pdfjs.TextLayer({ textContentSource: page.streamTextContent(), container: node, viewport: unit }).render();
    // The text layer is laid out at scale 1; scale it with the displayed page width.
    const fit = () => {
      holder.style.setProperty("--total-scale-factor", String(holder.clientWidth / unit.width));
      holder.style.setProperty("--scale-round-x", "1px");
      holder.style.setProperty("--scale-round-y", "1px");
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(holder);
    observers.push(observer);
  }

  const total = image ? 1 : page_count;
  return (
    <div className="pdf-viewer" role="group" aria-label={label} data-state={state} data-url={url}>
      {(total > 1 || selection) && (
        <div className="pdf-bar">
          <span className="pdf-bar-text">
            {selection && <span role="status">{selection.text}</span>}
            {total > 1 && <span className="tabular">Page {page_current} of {total}</span>}
          </span>
          <span className="pdf-bar-actions">
            {selection && <button type="button" className="btn btn-ghost" onClick={selection.onClear}>Clear highlight</button>}
            {total > 1 && <>
              <button type="button" className="btn btn-ghost" disabled={page_current <= 1} onClick={() => goToPage(page_current - 1)}>Previous page</button>
              <button type="button" className="btn btn-ghost" disabled={page_current >= total} onClick={() => goToPage(page_current + 1)}>Next page</button>
            </>}
          </span>
        </div>
      )}
      {state === "loading" && !image && <p role="status" className="scan-caption">Loading preview…</p>}
      {state === "failed" && <p role="alert" className="form-error">Preview unavailable. Open the original file below.</p>}
      <div ref={scroller} className="pdf-scroll" tabIndex={0} role="region" aria-label={`${label}, scrollable`}
        style={{ display: state === "failed" ? "none" : undefined }} onScroll={trackPage}>
        {image
          ? <div className="source-page" data-page="1">
              <img src={url} alt={label} onLoad={() => setState("ready")} onError={() => setState("failed")} />
              {state === "ready" && box && <Highlight key={box.page} box={box} />}
            </div>
          : Array.from({ length: page_count }, (_, i) => (
              <div className="source-page" data-page={i + 1} key={i}>
                <canvas ref={(node) => { canvases.current[i] = node; }} aria-hidden="true" />
                <div className="textLayer" ref={(node) => { text_layers.current[i] = node; }} />
                {pages_done >= i + 1 && box?.page === i + 1 && <Highlight key={box.page} box={box} />}
              </div>
            ))}
      </div>
    </div>
  );
}
